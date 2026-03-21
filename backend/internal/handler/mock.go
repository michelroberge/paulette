package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/ai-app-factory/backend/internal/agent"
	"github.com/michelroberge/ai-app-factory/backend/internal/model"
	"github.com/michelroberge/ai-app-factory/backend/internal/repository"
)

const mockRelPath = ".ai-factory/ux/mock.html"

type MockHandler struct {
	registry     repository.RegistryRepo
	artifactRepo repository.ArtifactRepo
}

func NewMockHandler(registry repository.RegistryRepo, artifactRepo repository.ArtifactRepo) *MockHandler {
	return &MockHandler{registry: registry, artifactRepo: artifactRepo}
}

type mockGetResponse struct {
	HTML   string `json:"html"`
	Exists bool   `json:"exists"`
}

func (h *MockHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	p := filepath.Join(project.HostDir, mockRelPath)
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockGetResponse{Exists: false})
			return
		}
		http.Error(w, "failed to read mock: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mockGetResponse{HTML: string(b), Exists: true})
}

type generateMockRequest struct {
	Refinement string `json:"refinement"`
}

// Generate streams mock HTML generation via SSE.
func (h *MockHandler) Generate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	var req generateMockRequest
	json.NewDecoder(r.Body).Decode(&req) // optional body — ignore decode errors

	// Use UX artifact if available, fall back to vision artifact
	uxContent, _ := h.artifactRepo.Read(project.HostDir, model.StageUX)
	if uxContent == "" {
		uxContent, _ = h.artifactRepo.Read(project.HostDir, model.StageVision)
	}
	if uxContent == "" {
		http.Error(w, "no artifact available — chat with the agent to generate content first", http.StatusBadRequest)
		return
	}

	events, err := agent.GenerateMock(r.Context(), uxContent, req.Refinement)
	if err != nil {
		http.Error(w, "failed to start mock generation: "+err.Error(), http.StatusInternalServerError)
		return
	}

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
			html := agent.ExtractHTML(event.Content)
			// Save to disk
			p := filepath.Join(project.HostDir, mockRelPath)
			if err := os.MkdirAll(filepath.Dir(p), 0755); err == nil {
				os.WriteFile(p, []byte(html), 0644)
			}
			sseWrite(w, flusher, agent.StreamEvent{Type: "done", Content: html})
		} else {
			sseWrite(w, flusher, event)
		}
	}

	fmt.Fprintf(w, "\n")
}
