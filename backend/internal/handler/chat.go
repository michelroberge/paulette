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

	"github.com/michelroberge/Claudine/backend/internal/agent"
	"github.com/michelroberge/Claudine/backend/internal/model"
	"github.com/michelroberge/Claudine/backend/internal/pipeline"
	"github.com/michelroberge/Claudine/backend/internal/repository"
	fsrepo "github.com/michelroberge/Claudine/backend/internal/repository/fs"
	"github.com/michelroberge/Claudine/backend/internal/stream"
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
	runs         *stream.Manager
}

func NewChatHandler(registry repository.RegistryRepo, chatRepo repository.ChatRepo, artifactRepo repository.ArtifactRepo, runs *stream.Manager) *ChatHandler {
	return &ChatHandler{
		registry:     registry,
		chatRepo:     chatRepo,
		artifactRepo: artifactRepo,
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

	// Save user message
	userMsg := model.Message{
		Role:      model.RoleUser,
		Content:   req.Message,
		Timestamp: time.Now(),
	}
	if err := h.chatRepo.AppendMessage(project.HostDir, stage, userMsg); err != nil {
		http.Error(w, "failed to save message: "+err.Error(), http.StatusInternalServerError)
		return
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
		// Load summary from current .claudine (it was archived but also kept)
		summaryPath := filepath.Join(project.HostDir, ".claudine", "summary.md")
		if data, err := os.ReadFile(summaryPath); err == nil {
			enhCtx.Summary = string(data)
		}
		// Load prior iteration's artifact for this stage
		prevVersion := findPriorIterationVersion(project.HostDir)
		if prevVersion != "" {
			priorArtifactPath := filepath.Join(project.HostDir, ".claudine", "iterations", "v"+prevVersion, string(stage), string(stage)+".md")
			if data, err := os.ReadFile(priorArtifactPath); err == nil {
				enhCtx.PriorArtifact = string(data)
			}
		}
	}

	systemPrompt := agent.GetSystemPrompt(stage, project.Version, previousArtifacts, frameworkCfg, enhCtx)

	// Get chat history for context
	history, err := h.chatRepo.GetHistory(project.HostDir, stage)
	if err != nil {
		http.Error(w, "failed to get chat history: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Remove the last message (the one we just appended) since we pass it separately
	if len(history) > 0 {
		history = history[:len(history)-1]
	}

	// Start a managed run
	run := h.runs.Start(id, string(stage), "chat")
	if run == nil {
		http.Error(w, "an agent is already running for this stage", http.StatusConflict)
		return
	}

	// Start Claude CLI subprocess using the run's context (survives client disconnect)
	events, err := agent.Chat(run.Context(), stageModels[stage], systemPrompt, history, req.Message, project.HostDir)
	if err != nil {
		run.Finish(h.runs)
		http.Error(w, "failed to start agent: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Background goroutine: process agent events, emit through run
	go func() {
		defer run.Finish(h.runs)

		var stageTokensAccum int
		defer func() {
			if stageTokensAccum > 0 {
				project.AddStageTokens(stage, stageTokensAccum)
				h.registry.Update(project)
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

	// Stream to this client (blocks until run finishes or client disconnects)
	run.StreamTo(w, r, 0)
}

func sseWrite(w http.ResponseWriter, flusher http.Flusher, event agent.StreamEvent) {
	data, _ := json.Marshal(event)
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

// findPriorIterationVersion finds the most recent archived iteration version.
func findPriorIterationVersion(hostDir string) string {
	iterDir := filepath.Join(hostDir, ".claudine", "iterations")
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
