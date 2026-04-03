package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/paulette/backend/internal/agent"
	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/pipeline"
	"github.com/michelroberge/paulette/backend/internal/provider"
	"github.com/michelroberge/paulette/backend/internal/repository"
	fsrepo "github.com/michelroberge/paulette/backend/internal/repository/fs"
	"github.com/michelroberge/paulette/backend/internal/stream"
)

// connectionErrorPayload is the structured JSON payload embedded in a
// StreamEvent{Type: "error"} Content field when the error originates from a
// provider connection failure. It matches the frontend ConnectionError type in
// types/provider.ts so the frontend can render the actionable ConnectionErrorBanner.
type connectionErrorPayload struct {
	IsConnectionError bool   `json:"isConnectionError"`
	ConnectionID      string `json:"connectionId"`
	ConnectionName    string `json:"connectionName"`
	Reason            string `json:"reason"`
}

type ChatHandler struct {
	registry         repository.RegistryRepo
	chatRepo         repository.ChatRepo
	artifactRepo     repository.ArtifactRepo
	activityRepo     repository.ActivityRepo
	runs             *stream.Manager
	providerRegistry *provider.Registry
	stageConfig      *provider.StageConfigStore
	connStore        *provider.ConnectionStore
}

// NewChatHandler creates a ChatHandler. providerRegistry, stageConfig, and
// connStore may be nil — in that case every stage falls back to the Claude CLI
// provider, preserving v0.1.0 behaviour exactly.
func NewChatHandler(
	registry repository.RegistryRepo,
	chatRepo repository.ChatRepo,
	artifactRepo repository.ArtifactRepo,
	activityRepo repository.ActivityRepo,
	runs *stream.Manager,
	providerRegistry *provider.Registry,
	stageConfig *provider.StageConfigStore,
	connStore *provider.ConnectionStore,
) *ChatHandler {
	return &ChatHandler{
		registry:         registry,
		chatRepo:         chatRepo,
		artifactRepo:     artifactRepo,
		activityRepo:     activityRepo,
		runs:             runs,
		providerRegistry: providerRegistry,
		stageConfig:      stageConfig,
		connStore:        connStore,
	}
}

// buildProviderErrorEvent builds a StreamEvent{Type: "error"} whose Content is a
// JSON-encoded connectionErrorPayload. It attempts to look up the connection name
// for the given stage; on any lookup failure it falls back to safe defaults so
// the event is always emittable.
func (h *ChatHandler) buildProviderErrorEvent(hostDir string, stage model.StageName, err error) agent.StreamEvent {
	payload := connectionErrorPayload{
		IsConnectionError: true,
		ConnectionName:    "configured connection",
		Reason:            err.Error(),
	}

	// Best-effort: enrich the payload with the connection ID and human-readable name.
	if h.stageConfig != nil && h.connStore != nil {
		if assignment := h.stageConfig.ResolveStage(hostDir, stage); assignment != nil {
			payload.ConnectionID = assignment.ConnectionID
			if conn, lookupErr := h.connStore.Get(assignment.ConnectionID); lookupErr == nil {
				payload.ConnectionName = conn.Name
			}
		}
	}

	content, _ := json.Marshal(payload)
	return agent.StreamEvent{Type: "error", Content: string(content)}
}

// resolveProvider returns the Provider, model ID, and optional stage settings to
// use for a stage. It delegates to the Registry when one is available; otherwise
// it falls back to the Claude CLI provider using the hardcoded stage-model map
// (v0.1.0 behaviour). The returned *StageAssignment may be nil for the fallback.
func (h *ChatHandler) resolveProvider(projectID, hostDir string, stage model.StageName) (provider.Provider, string, *provider.StageAssignment, error) {
	if h.providerRegistry != nil {
		return h.providerRegistry.ResolveForStageWithSettings(projectID, stage, h.stageConfig, hostDir)
	}
	// Nil registry — construct a bare Claude CLI provider as a safe fallback.
	return provider.NewClaudeCLIProvider(), provider.FallbackModel(stage), nil, nil
}

func (h *ChatHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	stage := model.StageName(chi.URLParam(r, "stage"))

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	messages, err := h.chatRepo.GetHistory(project.HostDir, stage)
	if err != nil {
		http.Error(w, "failed to get chat history: "+err.Error(), http.StatusInternalServerError)
		return
	}

	nextTurn := "agent"
	if len(messages) > 0 && messages[len(messages)-1].Role == model.RoleAssistant {
		nextTurn = "user"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(model.ChatHistory{Messages: messages, NextTurn: nextTurn})
}

type sendMessageRequest struct {
	Message string `json:"message"`
}

// Send handles chat messages via Claude CLI with SSE streaming.
func (h *ChatHandler) Send(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	stage := model.StageName(chi.URLParam(r, "stage"))

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	// Check for existing active run — if so, reconnect to it
	if existing := h.runs.Active(id, string(stage), "chat"); existing != nil {
		existing.StreamTo(w, r, 0)
		return
	}

	var req sendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	run, err := h.StartChatRun(project, stage, req.Message)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if run == nil {
		http.Error(w, "an agent is already running for this stage", http.StatusConflict)
		return
	}

	// Stream to this client (blocks until run finishes or client disconnects)
	run.StreamTo(w, r, 0)
}

// Resume re-invokes the agent for a stage where the last message is from the user but
// no agent response was produced (e.g. due to a server restart mid-generation).
func (h *ChatHandler) Resume(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	stage := model.StageName(chi.URLParam(r, "stage"))

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	if existing := h.runs.Active(id, string(stage), "chat"); existing != nil {
		existing.StreamTo(w, r, 0)
		return
	}

	history, err := h.chatRepo.GetHistory(project.HostDir, stage)
	if err != nil {
		http.Error(w, "failed to get chat history: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if len(history) == 0 || history[len(history)-1].Role != model.RoleUser {
		http.Error(w, "nothing to resume: last message is not from user", http.StatusBadRequest)
		return
	}

	lastUserMsg := history[len(history)-1].Content
	priorHistory := history[:len(history)-1]

	run, err := h.resumeChatRun(project, stage, priorHistory, lastUserMsg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if run == nil {
		http.Error(w, "an agent is already running for this stage", http.StatusConflict)
		return
	}

	run.StreamTo(w, r, 0)
}

// resumeChatRun starts a chat run using existing history and a prior user message,
// without appending a new user message to the stored history.
func (h *ChatHandler) resumeChatRun(project *model.Project, stage model.StageName, history []model.Message, message string) (*stream.Run, error) {
	previousArtifacts := make(map[model.StageName]string)
	for _, s := range pipeline.StageOrder {
		if s == stage {
			break
		}
		content, _ := h.artifactRepo.ReadWithFallback(project.HostDir, project.Version, s)
		if content != "" {
			previousArtifacts[s] = content
		}
	}

	var frameworkCfg *model.FrameworkConfig
	if stage == model.StageUX {
		frameworkCfg, _ = fsrepo.ReadFramework(project.HostDir)
	}

	var enhCtx *agent.EnhancementContext
	if project.EnhancementVision != "" {
		enhCtx = &agent.EnhancementContext{Vision: project.EnhancementVision}
		summaryPath := filepath.Join(project.HostDir, ".paulette", "summary.md")
		if data, err := os.ReadFile(summaryPath); err == nil {
			enhCtx.Summary = string(data)
		}
		prevVersion := findPriorIterationVersion(project.HostDir)
		if prevVersion != "" {
			priorArtifactPath := filepath.Join(project.HostDir, ".paulette", "iterations", "v"+prevVersion, string(stage), string(stage)+".md")
			if data, err := os.ReadFile(priorArtifactPath); err == nil {
				enhCtx.PriorArtifact = string(data)
			}
		}
	}

	systemPrompt := agent.GetSystemPrompt(stage, project.Version, previousArtifacts, frameworkCfg, enhCtx)

	run := h.runs.Start(project.ID, string(stage), "chat")
	if run == nil {
		return nil, nil // race
	}
	writeActivity(h.activityRepo, project.HostDir, stage, "chat")

	// Resolve the LLM provider for this stage (project override → global default → Claude CLI).
	prov, modelID, sa, provErr := h.resolveProvider(project.ID, project.HostDir, stage)
	if provErr != nil {
		run.Emit(h.buildProviderErrorEvent(project.HostDir, stage, provErr))
		clearActivity(h.activityRepo, project.HostDir, stage)
		run.Finish(h.runs)
		return run, nil
	}

	saTemp, saNumCtx, saStream := sa.Fields()
	events, chatErr := prov.Chat(run.Context(), provider.ChatRequest{
		Model:        modelID,
		SystemPrompt: systemPrompt,
		History:      history,
		UserMessage:  message,
		ProjectDir:   project.HostDir,
		Stage:        string(stage),
		Temperature:  saTemp,
		NumCtx:       saNumCtx,
		Stream:       saStream,
	})
	if chatErr != nil {
		run.Emit(h.buildProviderErrorEvent(project.HostDir, stage, chatErr))
		clearActivity(h.activityRepo, project.HostDir, stage)
		run.Finish(h.runs)
		return run, nil
	}

	go func() {
		defer run.Finish(h.runs)
		defer clearActivity(h.activityRepo, project.HostDir, stage)

		var stageTokensAccum int
		var accumulated strings.Builder
		runStart := time.Now()
		defer func() {
			if stageTokensAccum > 0 {
				project.AddStageTokens(stage, stageTokensAccum)
				h.registry.Update(project)
				recordSession(project.HostDir, stage, model.SessionChat, project.Iteration, runStart, stageTokensAccum)
			}
		}()

		for event := range events {
			if event.Type == "tokens" {
				var n int
				fmt.Sscanf(event.Content, "%d", &n)
				stageTokensAccum += n
			}
			if event.Type == "done" {
				fullContent := event.Content
				if fullContent == "" {
					fullContent = accumulated.String()
				}
				if artifact, found := agent.ExtractArtifact(fullContent); found {
					if err := h.artifactRepo.Write(project.HostDir, stage, artifact); err != nil {
						run.Emit(agent.StreamEvent{Type: "error", Content: "failed to save artifact"})
					} else {
						run.Emit(agent.StreamEvent{Type: "artifact", Content: string(stage) + "/" + string(stage) + ".md"})
					}
				}

				chatContent := agent.StripArtifact(fullContent)
				assistantMsg := model.Message{
					Role:      model.RoleAssistant,
					Content:   chatContent,
					Timestamp: time.Now(),
				}
				h.chatRepo.AppendMessage(project.HostDir, stage, assistantMsg)

				run.Emit(agent.StreamEvent{Type: "done", Content: chatContent})
			} else {
				if event.Type == "chunk" {
					accumulated.WriteString(event.Content)
				}
				run.Emit(event)
			}
		}
	}()

	return run, nil
}

// StartChatRun starts a chat run without HTTP plumbing. The caller is responsible for
// checking whether a run is already active before calling this. Returns (nil, nil) on race.
func (h *ChatHandler) StartChatRun(project *model.Project, stage model.StageName, message string) (*stream.Run, error) {
	// Save user message
	userMsg := model.Message{
		Role:      model.RoleUser,
		Content:   message,
		Timestamp: time.Now(),
	}
	if err := h.chatRepo.AppendMessage(project.HostDir, stage, userMsg); err != nil {
		return nil, fmt.Errorf("failed to save message: %w", err)
	}

	// Load previous artifacts for system prompt
	previousArtifacts := make(map[model.StageName]string)
	for _, s := range pipeline.StageOrder {
		if s == stage {
			break
		}
		content, _ := h.artifactRepo.ReadWithFallback(project.HostDir, project.Version, s)
		if content != "" {
			previousArtifacts[s] = content
		}
	}

	var frameworkCfg *model.FrameworkConfig
	if stage == model.StageUX {
		frameworkCfg, _ = fsrepo.ReadFramework(project.HostDir)
	}

	// Build enhancement context if this is an enhancement iteration
	var enhCtx *agent.EnhancementContext
	if project.EnhancementVision != "" {
		enhCtx = &agent.EnhancementContext{Vision: project.EnhancementVision}
		summaryPath := filepath.Join(project.HostDir, ".paulette", "summary.md")
		if data, err := os.ReadFile(summaryPath); err == nil {
			enhCtx.Summary = string(data)
		}
		prevVersion := findPriorIterationVersion(project.HostDir)
		if prevVersion != "" {
			priorArtifactPath := filepath.Join(project.HostDir, ".paulette", "iterations", "v"+prevVersion, string(stage), string(stage)+".md")
			if data, err := os.ReadFile(priorArtifactPath); err == nil {
				enhCtx.PriorArtifact = string(data)
			}
		}
	}

	systemPrompt := agent.GetSystemPrompt(stage, project.Version, previousArtifacts, frameworkCfg, enhCtx)

	// Get chat history for context
	history, err := h.chatRepo.GetHistory(project.HostDir, stage)
	if err != nil {
		return nil, fmt.Errorf("failed to get chat history: %w", err)
	}
	// Remove the last message (the one we just appended) since we pass it separately
	if len(history) > 0 {
		history = history[:len(history)-1]
	}

	// Start a managed run
	run := h.runs.Start(project.ID, string(stage), "chat")
	if run == nil {
		return nil, nil // race: already started
	}
	writeActivity(h.activityRepo, project.HostDir, stage, "chat")

	// Resolve the LLM provider for this stage (project override → global default → Claude CLI).
	prov, modelID, sa, provErr := h.resolveProvider(project.ID, project.HostDir, stage)
	if provErr != nil {
		run.Emit(h.buildProviderErrorEvent(project.HostDir, stage, provErr))
		clearActivity(h.activityRepo, project.HostDir, stage)
		run.Finish(h.runs)
		return run, nil
	}

	// Start LLM streaming via the resolved provider (survives client disconnect).
	saTemp, saNumCtx, saStream := sa.Fields()
	events, chatErr := prov.Chat(run.Context(), provider.ChatRequest{
		Model:        modelID,
		SystemPrompt: systemPrompt,
		History:      history,
		UserMessage:  message,
		ProjectDir:   project.HostDir,
		Stage:        string(stage),
		Temperature:  saTemp,
		NumCtx:       saNumCtx,
		Stream:       saStream,
	})
	if chatErr != nil {
		run.Emit(h.buildProviderErrorEvent(project.HostDir, stage, chatErr))
		clearActivity(h.activityRepo, project.HostDir, stage)
		run.Finish(h.runs)
		return run, nil
	}

	// Background goroutine: process agent events, emit through run.
	// LIFO defer: clearActivity runs first, then run.Finish — watcher sees clean state.
	go func() {
		defer run.Finish(h.runs)
		defer clearActivity(h.activityRepo, project.HostDir, stage)

		var stageTokensAccum int
		var accumulated strings.Builder
		runStart := time.Now()
		defer func() {
			if stageTokensAccum > 0 {
				project.AddStageTokens(stage, stageTokensAccum)
				h.registry.Update(project)
				recordSession(project.HostDir, stage, model.SessionChat, project.Iteration, runStart, stageTokensAccum)
			}
		}()

		for event := range events {
			if event.Type == "tokens" {
				var n int
				fmt.Sscanf(event.Content, "%d", &n)
				stageTokensAccum += n
			}
			if event.Type == "done" {
				fullContent := event.Content
				if fullContent == "" {
					fullContent = accumulated.String()
				}
				// Extract and save artifact if present
				if artifact, found := agent.ExtractArtifact(fullContent); found {
					if err := h.artifactRepo.Write(project.HostDir, stage, artifact); err != nil {
						run.Emit(agent.StreamEvent{Type: "error", Content: "failed to save artifact"})
					} else {
						run.Emit(agent.StreamEvent{Type: "artifact", Content: string(stage) + "/" + string(stage) + ".md"})
					}
				}

				// Save assistant response without artifact block
				chatContent := agent.StripArtifact(fullContent)
				assistantMsg := model.Message{
					Role:      model.RoleAssistant,
					Content:   chatContent,
					Timestamp: time.Now(),
				}
				h.chatRepo.AppendMessage(project.HostDir, stage, assistantMsg)

				run.Emit(agent.StreamEvent{Type: "done", Content: chatContent})
			} else {
				if event.Type == "chunk" {
					accumulated.WriteString(event.Content)
				}
				run.Emit(event)
			}
		}
	}()

	return run, nil
}

func sseWrite(w http.ResponseWriter, flusher http.Flusher, event agent.StreamEvent) {
	data, _ := json.Marshal(event)
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

// findPriorIterationVersion finds the most recent archived iteration version.
func findPriorIterationVersion(hostDir string) string {
	iterDir := filepath.Join(hostDir, ".paulette", "iterations")
	entries, err := os.ReadDir(iterDir)
	if err != nil {
		return ""
	}
	var versions []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "v") {
			versions = append(versions, e.Name()[1:]) // strip "v" prefix
		}
	}
	if len(versions) == 0 {
		return ""
	}
	sort.Strings(versions)
	return versions[len(versions)-1]
}
