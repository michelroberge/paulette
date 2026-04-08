package handler

import (
	"encoding/json"
	"net/http"

	"github.com/michelroberge/paulette/backend/internal/rag"
)

// RAGHandler exposes RAG integration status to the frontend.
type RAGHandler struct {
	ragClient *rag.Client
}

// NewRAGHandler creates a RAG status handler. ragClient may be nil.
func NewRAGHandler(ragClient *rag.Client) *RAGHandler {
	return &RAGHandler{ragClient: ragClient}
}

// Status returns the current RAG integration status.
// GET /api/rag/status
func (h *RAGHandler) Status(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		Enabled bool   `json:"enabled"`
		Healthy bool   `json:"healthy"`
		BaseURL string `json:"baseUrl"`
	}{
		Enabled: h.ragClient.Enabled(),
		Healthy: h.ragClient.IsHealthy(),
		BaseURL: h.ragClient.BaseURL(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
