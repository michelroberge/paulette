package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/paulette/backend/internal/promptfiles"
	"github.com/michelroberge/paulette/backend/internal/provider"
)

// ConnectionPromptHandler handles CRUD for per-connection prompt template files.
type ConnectionPromptHandler struct {
	connStore    *provider.ConnectionStore
	registryPath string
}

// NewConnectionPromptHandler constructs a ConnectionPromptHandler.
func NewConnectionPromptHandler(connStore *provider.ConnectionStore, registryPath string) *ConnectionPromptHandler {
	return &ConnectionPromptHandler{connStore: connStore, registryPath: registryPath}
}

func (h *ConnectionPromptHandler) store(connID string) *promptfiles.PromptStore {
	return promptfiles.NewForConnection(h.registryPath, connID)
}

func (h *ConnectionPromptHandler) validateConn(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := chi.URLParam(r, "id")
	if _, err := h.connStore.Get(id); err != nil {
		jsonError(w, http.StatusNotFound, "connection not found")
		return "", false
	}
	return id, true
}

// Init creates the connection prompts directory and writes all defaults.
// POST /api/connections/{id}/prompts/init
func (h *ConnectionPromptHandler) Init(w http.ResponseWriter, r *http.Request) {
	id, ok := h.validateConn(w, r)
	if !ok {
		return
	}
	if err := h.store(id).Init(); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to init prompts: "+err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// List returns metadata for all known prompt templates.
// GET /api/connections/{id}/prompts
func (h *ConnectionPromptHandler) List(w http.ResponseWriter, r *http.Request) {
	id, ok := h.validateConn(w, r)
	if !ok {
		return
	}
	writeJSON(w, h.store(id).List())
}

// Get returns the content and metadata of a single prompt template.
// GET /api/connections/{id}/prompts/{name}
func (h *ConnectionPromptHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := h.validateConn(w, r)
	if !ok {
		return
	}
	name := chi.URLParam(r, "name")
	store := h.store(id)
	content, err := store.Load(name)
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}

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
// PUT /api/connections/{id}/prompts/{name}
func (h *ConnectionPromptHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := h.validateConn(w, r)
	if !ok {
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
	if err := h.store(id).Save(name, body.Content); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// Reset restores a single prompt template to its built-in default.
// POST /api/connections/{id}/prompts/reset/{name}
func (h *ConnectionPromptHandler) Reset(w http.ResponseWriter, r *http.Request) {
	id, ok := h.validateConn(w, r)
	if !ok {
		return
	}
	name := chi.URLParam(r, "name")
	if err := h.store(id).Reset(name); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}
