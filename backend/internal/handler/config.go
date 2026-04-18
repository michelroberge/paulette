package handler

import (
	"encoding/json"
	"net/http"

	"github.com/michelroberge/paulette/backend/internal/config"
)

type ConfigHandler struct {
	cfg *config.Config
}

func NewConfigHandler(cfg *config.Config) *ConfigHandler {
	return &ConfigHandler{cfg: cfg}
}

func (h *ConfigHandler) GetInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
		"reposPath":   h.cfg.GitPath,
		"version":     h.cfg.Version,
		"author":      h.cfg.Author,
		"oidcEnabled": h.cfg.OIDC.Enabled,
		"ragEnabled":  h.cfg.RAG.Enabled,
	})
}
