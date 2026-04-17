package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/paulette/backend/internal/promptfiles"
	"github.com/michelroberge/paulette/backend/internal/repository"
)

// PromptHandler handles CRUD for per-project prompt template files.
type PromptHandler struct {
	registry repository.RegistryRepo
}

// NewPromptHandler constructs a PromptHandler.
func NewPromptHandler(registry repository.RegistryRepo) *PromptHandler {
	return &PromptHandler{registry: registry}
}

// Init creates .paulette/prompts/ in the project's host directory and writes all defaults.
// POST /api/projects/{id}/prompts/init
func (h *PromptHandler) Init(w http.ResponseWriter, r *http.Request) {
	project, err := h.registry.Get(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, http.StatusNotFound, "project not found")
		return
	}
	store := promptfiles.New(project.HostDir)
	if err := store.Init(); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to init prompts: "+err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// List returns metadata for all known prompt templates.
// GET /api/projects/{id}/prompts
func (h *PromptHandler) List(w http.ResponseWriter, r *http.Request) {
	project, err := h.registry.Get(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, http.StatusNotFound, "project not found")
		return
	}
	store := promptfiles.New(project.HostDir)
	writeJSON(w, store.List())
}

// Get returns the content and metadata of a single prompt template.
// GET /api/projects/{id}/prompts/{name}
func (h *PromptHandler) Get(w http.ResponseWriter, r *http.Request) {
	project, err := h.registry.Get(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, http.StatusNotFound, "project not found")
		return
	}
	name := chi.URLParam(r, "name")
	store := promptfiles.New(project.HostDir)
	content, err := store.Load(name)
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}

	// Look up metadata from defaults.
	meta := promptfiles.PromptMeta{Name: name}
	if def, ok := promptfiles.Defaults[name]; ok {
		meta.Category = def.Category
		meta.Description = def.Description
		meta.Variables = def.Variables
	}

	writeJSON(w, map[string]any{
		"name":        meta.Name,
		"category":    meta.Category,
		"description": meta.Description,
		"variables":   meta.Variables,
		"content":     content,
	})
}

// Update writes new content to a prompt template file.
// PUT /api/projects/{id}/prompts/{name}
func (h *PromptHandler) Update(w http.ResponseWriter, r *http.Request) {
	project, err := h.registry.Get(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, http.StatusNotFound, "project not found")
		return
	}

	name := chi.URLParam(r, "name")
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	store := promptfiles.New(project.HostDir)
	if err := store.Save(name, body.Content); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// Reset restores a single prompt template to its built-in default.
// POST /api/projects/{id}/prompts/reset/{name}
func (h *PromptHandler) Reset(w http.ResponseWriter, r *http.Request) {
	project, err := h.registry.Get(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, http.StatusNotFound, "project not found")
		return
	}

	name := chi.URLParam(r, "name")
	store := promptfiles.New(project.HostDir)
	if err := store.Reset(name); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}
