package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/michelroberge/claudette/backend/internal/agent"
	"github.com/michelroberge/claudette/backend/internal/git"
	"github.com/michelroberge/claudette/backend/internal/model"
	"github.com/michelroberge/claudette/backend/internal/repository"
	"github.com/michelroberge/claudette/backend/internal/stream"
)

type ImportHandler struct {
	registry     repository.RegistryRepo
	projectRepo  repository.ProjectRepo
	artifactRepo repository.ArtifactRepo
	runs         *stream.Manager
	git          *git.Service
	reposPath    string
}

func NewImportHandler(registry repository.RegistryRepo, projectRepo repository.ProjectRepo, artifactRepo repository.ArtifactRepo, runs *stream.Manager, gitSvc *git.Service, reposPath string) *ImportHandler {
	return &ImportHandler{
		registry:     registry,
		projectRepo:  projectRepo,
		artifactRepo: artifactRepo,
		runs:         runs,
		git:          gitSvc,
		reposPath:    reposPath,
	}
}

type importRequest struct {
	RepoURL string `json:"repoUrl"`
	HostDir string `json:"hostDir"`
	Name    string `json:"name"`
	Author  string `json:"author"`
	Version string `json:"version"`
}

func (h *ImportHandler) Import(w http.ResponseWriter, r *http.Request) {
	var req importRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.RepoURL != "" {
		// Clone import — hostDir is optional, default to repos/<repo-basename>
		if req.HostDir == "" {
			base := filepath.Base(req.RepoURL)
			base = strings.TrimSuffix(base, ".git")
			req.HostDir = filepath.Join(h.reposPath, base)
		}
		// Host dir must not already exist
		if _, err := os.Stat(req.HostDir); err == nil {
			http.Error(w, "hostDir already exists; for local import, omit repoUrl", http.StatusBadRequest)
			return
		}
		if err := h.git.Clone(req.RepoURL, req.HostDir); err != nil {
			http.Error(w, "failed to clone repo: "+err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		// Local import — hostDir is required
		if req.HostDir == "" {
			http.Error(w, "hostDir is required for local imports", http.StatusBadRequest)
			return
		}
		if _, err := os.Stat(req.HostDir); os.IsNotExist(err) {
			http.Error(w, "hostDir does not exist", http.StatusBadRequest)
			return
		}
		if _, err := os.Stat(filepath.Join(req.HostDir, ".git")); os.IsNotExist(err) {
			http.Error(w, "hostDir is not a git repository", http.StatusBadRequest)
			return
		}
	}

	// Defaults
	name := req.Name
	if name == "" {
		name = filepath.Base(req.HostDir)
	}
	version := req.Version
	if version == "" {
		version = "1.0.0"
	}

	// Initialize .ai-factory directory structure
	if err := h.projectRepo.Init(req.HostDir); err != nil {
		http.Error(w, "failed to init project directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	now := time.Now()
	project := &model.Project{
		ID:           uuid.New().String(),
		Name:         name,
		Author:       req.Author,
		Version:      version,
		HostDir:      req.HostDir,
		CurrentStage: model.StageVision,
		Iteration:    1,
		Imported:     true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// Save project.json and register
	if err := h.projectRepo.Save(req.HostDir, project); err != nil {
		http.Error(w, "failed to save project: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.registry.Create(project); err != nil {
		http.Error(w, "failed to register project: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Start background import
	h.startImportRun(project)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(project)
}

func (h *ImportHandler) startImportRun(project *model.Project) {
	run := h.runs.Start(project.ID, "import", "import")
	if run == nil {
		return // already running
	}

	go func() {
		defer run.Finish(h.runs)

		emit := func(ev agent.StreamEvent) {
			run.Emit(ev)
		}

		emit(agent.StreamEvent{Type: "log", Content: "Starting import analysis..."})

		result, err := agent.StreamImport(run.Context(), project.HostDir, project.Version, project.Name, emit)
		if err != nil {
			emit(agent.StreamEvent{Type: "error", Content: err.Error()})
			return
		}

		// Write all generated artifacts
		for stage, content := range result.Artifacts {
			if err := h.artifactRepo.Write(project.HostDir, stage, content); err != nil {
				log.Printf("import: failed to write %s artifact: %v", stage, err)
				emit(agent.StreamEvent{Type: "error", Content: "failed to write " + string(stage) + " artifact"})
				return
			}
		}

		// Git commit all artifacts
		if err := h.git.AddAllAndCommit(project.HostDir, "import: reverse-engineer pipeline artifacts"); err != nil {
			log.Printf("import: git commit failed: %v", err)
		}

		emit(agent.StreamEvent{Type: "done", Content: "Import complete"})
	}()
}

// WatchImport streams the live import progress as SSE.
func (h *ImportHandler) WatchImport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.registry.Get(id); err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	run := h.runs.Active(id, "import", "import")
	if run == nil {
		// No active run — return empty
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		return
	}
	run.StreamTo(w, r, 0)
}
