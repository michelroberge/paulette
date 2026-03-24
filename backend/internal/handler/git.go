package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/paulette/backend/internal/git"
	"github.com/michelroberge/paulette/backend/internal/pipeline"
	"github.com/michelroberge/paulette/backend/internal/repository"
)

type GitHandler struct {
	registry    repository.RegistryRepo
	projectRepo repository.ProjectRepo
	git         *git.Service
}

func NewGitHandler(registry repository.RegistryRepo, projectRepo repository.ProjectRepo, gitSvc *git.Service) *GitHandler {
	return &GitHandler{
		registry:    registry,
		projectRepo: projectRepo,
		git:         gitSvc,
	}
}

func (h *GitHandler) projectDir(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	id := chi.URLParam(r, "id")
	p, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return "", "", false
	}
	return id, p.HostDir, true
}

// Log returns the git log for the project.
func (h *GitHandler) Log(w http.ResponseWriter, r *http.Request) {
	_, dir, ok := h.projectDir(w, r)
	if !ok {
		return
	}
	limit := 50
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			limit = n
		}
	}
	entries, err := h.git.Log(dir, limit)
	if err != nil {
		http.Error(w, "git log failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if entries == nil {
		entries = []git.CommitEntry{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

// Status returns git status and remote info.
func (h *GitHandler) Status(w http.ResponseWriter, r *http.Request) {
	_, dir, ok := h.projectDir(w, r)
	if !ok {
		return
	}
	status, err := h.git.Status(dir)
	if err != nil {
		http.Error(w, "git status failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

type resetRequest struct {
	Ref string `json:"ref"`
}

type resetResponse struct {
	Project  interface{} `json:"project"`
	Pipeline interface{} `json:"pipeline"`
}

// Reset resets the project to a given commit, then reloads project state.
func (h *GitHandler) Reset(w http.ResponseWriter, r *http.Request) {
	id, dir, ok := h.projectDir(w, r)
	if !ok {
		return
	}
	var req resetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Ref == "" {
		http.Error(w, "ref is required", http.StatusBadRequest)
		return
	}

	if err := h.git.ResetHard(dir, req.Ref); err != nil {
		http.Error(w, "git reset failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Re-read project state from the restored files
	project, err := h.projectRepo.Load(dir)
	if err != nil {
		http.Error(w, "failed to reload project after reset: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Preserve registry ID (project.json may have same ID, but just in case)
	project.ID = id

	// Update the central registry to match
	if err := h.registry.Update(project); err != nil {
		log.Printf("failed to update registry after git reset: %v", err)
	}

	state := pipeline.BuildPipelineState(project.CurrentStage, nil)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resetResponse{
		Project:  project,
		Pipeline: state,
	})
}

// Discard discards all uncommitted changes.
func (h *GitHandler) Discard(w http.ResponseWriter, r *http.Request) {
	_, dir, ok := h.projectDir(w, r)
	if !ok {
		return
	}
	if err := h.git.DiscardChanges(dir); err != nil {
		http.Error(w, "discard failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type remoteRequest struct {
	URL string `json:"url"`
}

// GetRemote returns the current remote URL.
func (h *GitHandler) GetRemote(w http.ResponseWriter, r *http.Request) {
	_, dir, ok := h.projectDir(w, r)
	if !ok {
		return
	}
	url := h.git.RemoteGet(dir)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": url})
}

// SetRemote sets the origin remote URL.
func (h *GitHandler) SetRemote(w http.ResponseWriter, r *http.Request) {
	_, dir, ok := h.projectDir(w, r)
	if !ok {
		return
	}
	var req remoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}
	if err := h.git.RemoteSet(dir, req.URL); err != nil {
		http.Error(w, "set remote failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoveRemote removes the origin remote.
func (h *GitHandler) RemoveRemote(w http.ResponseWriter, r *http.Request) {
	_, dir, ok := h.projectDir(w, r)
	if !ok {
		return
	}
	if err := h.git.RemoteRemove(dir); err != nil {
		http.Error(w, "remove remote failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Push pushes to origin with tags.
func (h *GitHandler) Push(w http.ResponseWriter, r *http.Request) {
	_, dir, ok := h.projectDir(w, r)
	if !ok {
		return
	}
	if err := h.git.Push(dir); err != nil {
		http.Error(w, "push failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Pull pulls from origin.
func (h *GitHandler) Pull(w http.ResponseWriter, r *http.Request) {
	id, dir, ok := h.projectDir(w, r)
	if !ok {
		return
	}
	if err := h.git.Pull(dir); err != nil {
		http.Error(w, "pull failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Re-read project state after pull (remote may have advanced)
	project, err := h.projectRepo.Load(dir)
	if err != nil {
		log.Printf("failed to reload project after pull: %v", err)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	project.ID = id
	if err := h.registry.Update(project); err != nil {
		log.Printf("failed to update registry after pull: %v", err)
	}

	state := pipeline.BuildPipelineState(project.CurrentStage, nil)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resetResponse{
		Project:  project,
		Pipeline: state,
	})
}
