package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/paulette/backend/internal/agent"
	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/provider"
	"github.com/michelroberge/paulette/backend/internal/repository"
	"github.com/michelroberge/paulette/backend/internal/stream"
)

type RefineHandler struct {
	registry         repository.RegistryRepo
	artifactRepo     repository.ArtifactRepo
	chatRepo         repository.ChatRepo
	runs             *stream.Manager
	providerRegistry *provider.Registry
	stageConfig      *provider.StageConfigStore
	connStore        *provider.ConnectionStore
	logBase          string
}

func NewRefineHandler(
	registry repository.RegistryRepo,
	artifactRepo repository.ArtifactRepo,
	chatRepo repository.ChatRepo,
	runs *stream.Manager,
	providerRegistry *provider.Registry,
	stageConfig *provider.StageConfigStore,
	connStore *provider.ConnectionStore,
	logBase string,
) *RefineHandler {
	return &RefineHandler{
		registry:         registry,
		artifactRepo:     artifactRepo,
		chatRepo:         chatRepo,
		runs:             runs,
		providerRegistry: providerRegistry,
		stageConfig:      stageConfig,
		connStore:        connStore,
		logBase:          logBase,
	}
}

type refineRequest struct {
	Selection   string `json:"selection"`
	Instruction string `json:"instruction"`
	// ConversationHistory contains previous refine instructions and responses for this selection
	ConversationHistory []model.Message `json:"conversationHistory,omitempty"`
}

type manualEditRequest struct {
	OldText string `json:"oldText"`
	NewText string `json:"newText"`
}

// Refine handles AI-assisted refinement of a selected artifact section.
// It streams the rewritten selection text back via SSE.
func (h *RefineHandler) Refine(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	stage := model.StageName(chi.URLParam(r, "stage"))

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	var req refineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Selection == "" || req.Instruction == "" {
		http.Error(w, "selection and instruction are required", http.StatusBadRequest)
		return
	}

	// Read current artifact
	artifact, err := h.artifactRepo.ReadWithFallback(project.DataDir, project.HostDir, project.Version, stage)
	if err != nil || artifact == "" {
		http.Error(w, "artifact not found", http.StatusNotFound)
		return
	}

	// Build the system prompt for refinement
	systemPrompt := fmt.Sprintf(`You are an AI assistant helping refine a section of a document artifact.

The user has selected a specific portion of the document and wants you to rewrite ONLY that section based on their instructions.

IMPORTANT RULES:
- Output ONLY the rewritten text for the selected section
- Do NOT include any explanation, preamble, or surrounding text
- Do NOT wrap the output in markdown code blocks
- Maintain the same markdown formatting style as the original
- The rewritten section must fit seamlessly into the surrounding document context

FULL DOCUMENT (for context only - do not rewrite the whole thing):
---
%s
---

SELECTED SECTION TO REWRITE:
---
%s
---`, artifact, req.Selection)

	// Build conversation history for the refine session
	var history []model.Message
	for _, msg := range req.ConversationHistory {
		history = append(history, msg)
	}

	userMessage := req.Instruction

	// Resolve provider
	prov, modelID, sa, provErr := h.resolveProvider(project.ID, project.HostDir, stage)
	if provErr != nil {
		http.Error(w, "provider error: "+provErr.Error(), http.StatusInternalServerError)
		return
	}

	runLogID := startRunLog(h.logBase, project, stage, "refine")

	saTemp, saNumCtx, saStream := sa.Fields()
	events, chatErr := prov.Chat(r.Context(), provider.ChatRequest{
		Model:        modelID,
		SystemPrompt: systemPrompt,
		History:      history,
		UserMessage:  userMessage,
		ProjectDir:   project.HostDir,
		Stage:        string(stage),
		Temperature:  saTemp,
		NumCtx:       saNumCtx,
		Stream:       saStream,
	})
	if chatErr != nil {
		failRunLog(h.logBase, project.Name, runLogID, chatErr.Error())
		http.Error(w, "chat error: "+chatErr.Error(), http.StatusInternalServerError)
		return
	}

	// SSE streaming response
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		failRunLog(h.logBase, project.Name, runLogID, "streaming not supported")
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	var accumulated strings.Builder
	var totalTokens int
	runStart := time.Now()

	for event := range events {
		switch event.Type {
		case "chunk":
			accumulated.WriteString(event.Content)
			sseWrite(w, flusher, event)
		case "tokens":
			var n int
			fmt.Sscanf(event.Content, "%d", &n)
			totalTokens += n
			sseWrite(w, flusher, event)
		case "done":
			content := event.Content
			if content == "" {
				content = accumulated.String()
			}
			// Record full prompt snapshot for tuning/inspection.
			writePromptSnapshot(h.logBase, project.Name, runLogID, model.PromptSnapshot{
				Stage:        string(stage),
				Timestamp:    time.Now(),
				Model:        modelID,
				SystemPrompt: systemPrompt,
				History:      history,
				UserMessage:  userMessage,
				RawResponse:  accumulated.String(),
			})
			sseWrite(w, flusher, agent.StreamEvent{Type: "done", Content: content})
			successRunLog(h.logBase, project.Name, runLogID, nil, totalTokens, fmt.Sprintf("refine selection (%d chars)", len(req.Selection)))
		case "error":
			sseWrite(w, flusher, event)
			failRunLog(h.logBase, project.Name, runLogID, event.Content)
		}
	}

	// Track tokens
	if totalTokens > 0 {
		project.AddStageTokens(stage, totalTokens)
		h.registry.Update(project)
		recordSession(project.DataDir, stage, model.SessionChat, project.Iteration, runStart, totalTokens)
	}
}

// ManualEdit replaces a section of the artifact with user-provided text.
func (h *RefineHandler) ManualEdit(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	stage := model.StageName(chi.URLParam(r, "stage"))

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	var req manualEditRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	artifact, err := h.artifactRepo.ReadWithFallback(project.DataDir, project.HostDir, project.Version, stage)
	if err != nil || artifact == "" {
		http.Error(w, "artifact not found", http.StatusNotFound)
		return
	}

	// Replace the old text with the new text
	updated := strings.Replace(artifact, req.OldText, req.NewText, 1)
	if updated == artifact {
		http.Error(w, "selected text not found in artifact", http.StatusBadRequest)
		return
	}

	if err := h.artifactRepo.Write(project.DataDir, stage, updated); err != nil {
		http.Error(w, "failed to save artifact", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "content": updated})
}

// ApplyRefine applies a refined selection to the artifact.
func (h *RefineHandler) ApplyRefine(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	stage := model.StageName(chi.URLParam(r, "stage"))

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	var req struct {
		OldText     string `json:"oldText"`
		NewText     string `json:"newText"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	artifact, err := h.artifactRepo.ReadWithFallback(project.DataDir, project.HostDir, project.Version, stage)
	if err != nil || artifact == "" {
		http.Error(w, "artifact not found", http.StatusNotFound)
		return
	}

	updated := strings.Replace(artifact, req.OldText, req.NewText, 1)
	if updated == artifact {
		http.Error(w, "selected text not found in artifact", http.StatusBadRequest)
		return
	}

	if err := h.artifactRepo.Write(project.DataDir, stage, updated); err != nil {
		http.Error(w, "failed to save artifact", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *RefineHandler) resolveProvider(projectID, hostDir string, stage model.StageName) (provider.Provider, string, *provider.StageAssignment, error) {
	if h.providerRegistry != nil {
		return h.providerRegistry.ResolveForStageWithSettings(projectID, stage, h.stageConfig, hostDir)
	}
	return provider.NewClaudeCLIProvider(), provider.FallbackModel(stage), nil, nil
}
