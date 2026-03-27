package handler

import (
	"encoding/json"
	"net/http"

	"github.com/michelroberge/paulette/backend/internal/git"
)

// GitGlobalHandler exposes git operations that do not require a project context.
type GitGlobalHandler struct {
	git           *git.Service
	identityStore *git.GlobalIdentityStore
}

func NewGitGlobalHandler(gitSvc *git.Service, identityStore *git.GlobalIdentityStore) *GitGlobalHandler {
	return &GitGlobalHandler{git: gitSvc, identityStore: identityStore}
}

// SSHKey returns the global SSH public key (auto-generated if missing).
// GET /api/git/ssh-key
func (h *GitGlobalHandler) SSHKey(w http.ResponseWriter, r *http.Request) {
	key, err := h.git.SSHPublicKey()
	if err != nil {
		http.Error(w, "failed to get SSH key: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set(headerContentType, contentTypeJSON)
	json.NewEncoder(w).Encode(map[string]string{"publicKey": key})
}

// GetIdentity returns the global git identity.
// GET /api/git/identity
func (h *GitGlobalHandler) GetIdentity(w http.ResponseWriter, r *http.Request) {
	id := h.identityStore.Get()
	w.Header().Set(headerContentType, contentTypeJSON)
	json.NewEncoder(w).Encode(id)
}

// SetIdentity saves the global git identity.
// POST /api/git/identity  body: { "name": "...", "email": "..." }
func (h *GitGlobalHandler) SetIdentity(w http.ResponseWriter, r *http.Request) {
	var req git.GlobalIdentity
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Email == "" {
		http.Error(w, "name and email are required", http.StatusBadRequest)
		return
	}
	if err := h.identityStore.Set(req.Name, req.Email); err != nil {
		http.Error(w, "failed to save identity: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
