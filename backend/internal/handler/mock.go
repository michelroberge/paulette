package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/paulette/backend/internal/agent"
	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/repository"
	fsrepo "github.com/michelroberge/paulette/backend/internal/repository/fs"
	"github.com/michelroberge/paulette/backend/internal/stream"
)

const mockRelPath = ".paulette/ux/mock.html"

type MockHandler struct {
	registry     repository.RegistryRepo
	artifactRepo repository.ArtifactRepo
	runs         *stream.Manager
}

func NewMockHandler(registry repository.RegistryRepo, artifactRepo repository.ArtifactRepo, runs *stream.Manager) *MockHandler {
	return &MockHandler{registry: registry, artifactRepo: artifactRepo, runs: runs}
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

	// Check for existing active run — reconnect to it
	if existing := h.runs.Active(id, "ux", "mock"); existing != nil {
		existing.StreamTo(w, r, 0)
		return
	}

	var req generateMockRequest
	json.NewDecoder(r.Body).Decode(&req) // optional body — ignore decode errors

	// Use UX artifact if available, fall back to vision artifact
	uxContent, _ := h.artifactRepo.ReadWithFallback(project.HostDir, project.Version, model.StageUX)
	if uxContent == "" {
		uxContent, _ = h.artifactRepo.ReadWithFallback(project.HostDir, project.Version, model.StageVision)
	}
	if uxContent == "" {
		http.Error(w, "no artifact available — chat with the agent to generate content first", http.StatusBadRequest)
		return
	}

	frameworkCfg, _ := fsrepo.ReadFramework(project.HostDir)

	// Start a managed run
	run := h.runs.Start(id, "ux", "mock")
	if run == nil {
		http.Error(w, "mock generation is already running", http.StatusConflict)
		return
	}

	events, err := agent.GenerateMock(run.Context(), uxContent, req.Refinement, frameworkCfg)
	if err != nil {
		run.Finish(h.runs)
		http.Error(w, "failed to start mock generation: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Background goroutine: process events, save result
	go func() {
		defer run.Finish(h.runs)

		var stageTokensAccum int
		defer func() {
			if stageTokensAccum > 0 {
				project.AddStageTokens(model.StageUX, stageTokensAccum)
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
				html := agent.ExtractHTML(event.Content)
				// Save to disk
				p := filepath.Join(project.HostDir, mockRelPath)
				if err := os.MkdirAll(filepath.Dir(p), 0755); err == nil {
					os.WriteFile(p, []byte(html), 0644)
				}
				run.Emit(agent.StreamEvent{Type: "done", Content: html})
			} else {
				run.Emit(event)
			}
		}
	}()

	run.StreamTo(w, r, 0)
}

// --- Framework endpoints ---

type frameworkResponse struct {
	Framework  string `json:"framework"`
	CustomName string `json:"customName,omitempty"`
}

type frameworkSetRequest struct {
	Framework  string `json:"framework"`
	CustomName string `json:"customName,omitempty"`
}

// GetFramework returns the stored framework config for a project.
func (h *MockHandler) GetFramework(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	cfg, err := fsrepo.ReadFramework(project.HostDir)
	if err != nil {
		http.Error(w, "failed to read framework: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(frameworkResponse{
		Framework:  string(cfg.Framework),
		CustomName: cfg.CustomName,
	})
}

// SetFramework stores the framework selection for a project.
func (h *MockHandler) SetFramework(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	var req frameworkSetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	validFrameworks := map[string]bool{
		"tailwind": true, "bootstrap": true, "mui": true,
		"shadcn": true, "vanilla": true, "other": true,
	}
	if !validFrameworks[req.Framework] {
		http.Error(w, "invalid framework: "+req.Framework, http.StatusBadRequest)
		return
	}
	if req.Framework == "other" && req.CustomName == "" {
		http.Error(w, "customName is required when framework is 'other'", http.StatusBadRequest)
		return
	}

	cfg := &model.FrameworkConfig{
		Framework:  model.UXFramework(req.Framework),
		CustomName: req.CustomName,
	}
	if err := fsrepo.WriteFramework(project.HostDir, cfg); err != nil {
		http.Error(w, "failed to save framework: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(frameworkResponse{
		Framework:  req.Framework,
		CustomName: req.CustomName,
	})
}
