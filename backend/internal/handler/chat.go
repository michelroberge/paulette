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
	"github.com/michelroberge/paulette/backend/internal/repository"
	fsrepo "github.com/michelroberge/paulette/backend/internal/repository/fs"
	"github.com/michelroberge/paulette/backend/internal/stream"
)

// stageModels maps each pipeline stage to the Claude model to use for chat.
var stageModels = map[model.StageName]string{
	model.StageVision:       "claude-sonnet-4-6",
	model.StageUX:           "claude-sonnet-4-6",
	model.StageArchitecture: "claude-opus-4-6",
	model.StageBuild:        "claude-opus-4-6",
}

type ChatHandler struct {
	registry     repository.RegistryRepo
	chatRepo     repository.ChatRepo
	artifactRepo repository.ArtifactRepo
	activityRepo repository.ActivityRepo
	runs         *stream.Manager
}

func NewChatHandler(registry repository.RegistryRepo, chatRepo repository.ChatRepo, artifactRepo repository.ArtifactRepo, activityRepo repository.ActivityRepo, runs *stream.Manager) *ChatHandler {
	return &ChatHandler{
		registry:     registry,
		chatRepo:     chatRepo,
		artifactRepo: artifactRepo,
		activityRepo: activityRepo,
		runs:         runs,
	}
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(model.ChatHistory{Messages: messages})
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

	// Start Claude CLI subprocess using the run's context (survives client disconnect)
	events, err := agent.Chat(run.Context(), stageModels[stage], systemPrompt, history, message, project.HostDir)
	if err != nil {
		clearActivity(h.activityRepo, project.HostDir, stage)
		run.Finish(h.runs)
		return nil, fmt.Errorf("failed to start agent: %w", err)
	}

	// Background goroutine: process agent events, emit through run.
	// LIFO defer: clearActivity runs first, then run.Finish — watcher sees clean state.
	go func() {
		defer run.Finish(h.runs)
		defer clearActivity(h.activityRepo, project.HostDir, stage)

		var stageTokensAccum int
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
				// Extract and save artifact if present
				if artifact, found := agent.ExtractArtifact(event.Content); found {
					if err := h.artifactRepo.Write(project.HostDir, stage, artifact); err != nil {
						run.Emit(agent.StreamEvent{Type: "error", Content: "failed to save artifact"})
					} else {
						run.Emit(agent.StreamEvent{Type: "artifact", Content: string(stage) + "/" + string(stage) + ".md"})
					}
				}

				// Save assistant response without artifact block
				chatContent := agent.StripArtifact(event.Content)
				assistantMsg := model.Message{
					Role:      model.RoleAssistant,
					Content:   chatContent,
					Timestamp: time.Now(),
				}
				h.chatRepo.AppendMessage(project.HostDir, stage, assistantMsg)

				run.Emit(agent.StreamEvent{Type: "done", Content: chatContent})
			} else {
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
