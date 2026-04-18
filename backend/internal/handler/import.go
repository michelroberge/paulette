package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/michelroberge/paulette/backend/internal/agent"
	"github.com/michelroberge/paulette/backend/internal/git"
	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/repository"
	"github.com/michelroberge/paulette/backend/internal/stream"
)

type ImportHandler struct {
	registry     repository.RegistryRepo
	projectRepo  repository.ProjectRepo
	artifactRepo repository.ArtifactRepo
	runs         *stream.Manager
	git          *git.Service
	gitPath      string
	dataPath     string
	gitIdentity  *git.GlobalIdentityStore
}

func NewImportHandler(registry repository.RegistryRepo, projectRepo repository.ProjectRepo, artifactRepo repository.ArtifactRepo, runs *stream.Manager, gitSvc *git.Service, gitPath, dataPath string, gitIdentity *git.GlobalIdentityStore) *ImportHandler {
	return &ImportHandler{
		registry:     registry,
		projectRepo:  projectRepo,
		artifactRepo: artifactRepo,
		runs:         runs,
		git:          gitSvc,
		gitPath:      gitPath,
		dataPath:     dataPath,
		gitIdentity:  gitIdentity,
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

	if err := h.resolveRepo(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Apply global git identity to the repo so commits work out of the box.
	if id := h.gitIdentity.Get(); id.Name != "" && id.Email != "" {
		_ = h.git.SetIdentity(req.HostDir, id.Name, id.Email)
	}

	project := h.buildProject(&req)

	// Initialize working-state directory
	if err := h.projectRepo.Init(project.DataDir); err != nil {
		http.Error(w, "failed to init project directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Save project.json and register
	if err := h.projectRepo.Save(project.DataDir, project); err != nil {
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

// resolveRepo validates and prepares the repository for import (clone or local).
func (h *ImportHandler) resolveRepo(req *importRequest) error {
	if req.RepoURL != "" {
		return h.resolveCloneImport(req)
	}
	return h.resolveLocalImport(req)
}

func (h *ImportHandler) resolveCloneImport(req *importRequest) error {
	if req.HostDir == "" {
		base := strings.TrimSuffix(filepath.Base(req.RepoURL), ".git")
		req.HostDir = filepath.Join(h.gitPath, base)
	}
	if _, err := os.Stat(req.HostDir); err == nil {
		return fmt.Errorf("hostDir already exists; for local import, omit repoUrl")
	}
	return h.git.Clone(req.RepoURL, req.HostDir)
}

func (h *ImportHandler) resolveLocalImport(req *importRequest) error {
	if req.HostDir == "" {
		return fmt.Errorf("hostDir is required for local imports")
	}
	if _, err := os.Stat(req.HostDir); os.IsNotExist(err) {
		return fmt.Errorf("hostDir does not exist")
	}
	if _, err := os.Stat(filepath.Join(req.HostDir, ".git")); os.IsNotExist(err) {
		return fmt.Errorf("hostDir is not a git repository")
	}
	return nil
}

func (h *ImportHandler) buildProject(req *importRequest) *model.Project {
	name := req.Name
	if name == "" {
		name = filepath.Base(req.HostDir)
	}
	version := req.Version
	if version == "" {
		version = "1.0.0"
	}
	now := time.Now()
	projectID := uuid.New().String()
	dataDir := model.ComputeDataDir(h.dataPath, projectID, version)
	return &model.Project{
		ID:           projectID,
		Name:         name,
		Author:       req.Author,
		Version:      version,
		HostDir:      req.HostDir,
		DataDir:      dataDir,
		CurrentStage: model.StageVision,
		Iteration:    1,
		Imported:     true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
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
			if err := h.artifactRepo.Write(project.DataDir, stage, content); err != nil {
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
		setSSEHeaders(w)
		return
	}
	run.StreamTo(w, r, 0)
}
