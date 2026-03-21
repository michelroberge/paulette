package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/ai-app-factory/backend/internal/agent"
	"github.com/michelroberge/ai-app-factory/backend/internal/model"
	"github.com/michelroberge/ai-app-factory/backend/internal/pipeline"
	"github.com/michelroberge/ai-app-factory/backend/internal/repository"
)

type ChatHandler struct {
	registry     repository.RegistryRepo
	chatRepo     repository.ChatRepo
	artifactRepo repository.ArtifactRepo
}

func NewChatHandler(registry repository.RegistryRepo, chatRepo repository.ChatRepo, artifactRepo repository.ArtifactRepo) *ChatHandler {
	return &ChatHandler{
		registry:     registry,
		chatRepo:     chatRepo,
		artifactRepo: artifactRepo,
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
		content, _ := h.artifactRepo.Read(project.HostDir, s)
		if content != "" {
			previousArtifacts[s] = content
		}
	}

	systemPrompt := agent.GetSystemPrompt(stage, previousArtifacts)

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

	// Start Claude CLI subprocess
	events, err := agent.Chat(r.Context(), systemPrompt, history, req.Message)
	if err != nil {
		http.Error(w, "failed to start agent: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Set up SSE
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for event := range events {
		if event.Type == "done" {
			// Extract and save artifact if present
			if artifact, found := agent.ExtractArtifact(event.Content); found {
				if err := h.artifactRepo.Write(project.HostDir, stage, artifact); err != nil {
					sseWrite(w, flusher, agent.StreamEvent{Type: "error", Content: "failed to save artifact"})
				} else {
					sseWrite(w, flusher, agent.StreamEvent{Type: "artifact", Content: string(stage) + "/" + string(stage) + ".md"})
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

			sseWrite(w, flusher, agent.StreamEvent{Type: "done", Content: chatContent})
		} else {
			sseWrite(w, flusher, event)
		}
	}
}

func sseWrite(w http.ResponseWriter, flusher http.Flusher, event agent.StreamEvent) {
	data, _ := json.Marshal(event)
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}
