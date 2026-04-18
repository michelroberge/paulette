package handler

import (
	"encoding/json"
	"net/http"

	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/promptfiles"
)

// jsonError writes a JSON-encoded error response with the given HTTP status code.
// All API error responses follow the shape { "error": "message" }.
// This function is shared across all handlers in the package.
func jsonError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// writeJSON serialises v as JSON and writes it to w with a 200 OK status.
// The Content-Type header is set to application/json.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// promptStoreForProject returns a *promptfiles.PromptStore for the project when
// its AIMode is "files" and the prompts directory exists, or nil otherwise.
func promptStoreForProject(project *model.Project) *promptfiles.PromptStore {
	if project.AIMode != model.AIModeFiles && project.AIMode != "" {
		return nil
	}
	store := promptfiles.New(project.HostDir)
	if !store.Exists() {
		return nil
	}
	return store
}

// promptStoreWithConnection builds a project store with a connection-level
// fallback. Resolution: project file → connection file → hardcoded default.
// registryPath is needed to locate ~/.paulette/connection-prompts/{connID}/.
func promptStoreWithConnection(project *model.Project, connectionID, registryPath string) *promptfiles.PromptStore {
	projectStore := promptStoreForProject(project)

	var connStore *promptfiles.PromptStore
	if connectionID != "" && registryPath != "" {
		cs := promptfiles.NewForConnection(registryPath, connectionID)
		if cs.Exists() {
			connStore = cs
		}
	}

	if projectStore != nil && connStore != nil {
		return projectStore.WithFallback(connStore)
	}
	if projectStore != nil {
		return projectStore
	}
	if connStore != nil {
		return connStore
	}
	return nil
}

// setSSEHeaders writes the standard headers for Server-Sent Events responses,
// including X-Accel-Buffering: no to disable nginx proxy buffering.
func setSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
}
