package handler

import (
	"encoding/json"
	"net/http"
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

// setSSEHeaders writes the standard headers for Server-Sent Events responses,
// including X-Accel-Buffering: no to disable nginx proxy buffering.
func setSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
}
