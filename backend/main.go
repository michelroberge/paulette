package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/michelroberge/ai-app-factory/backend/internal/config"
	"github.com/michelroberge/ai-app-factory/backend/internal/repository/fs"
	"github.com/michelroberge/ai-app-factory/backend/internal/server"
)

func main() {
	cfg := config.Load()

	registry, err := fs.NewRegistryRepo(cfg.RegistryPath)
	if err != nil {
		log.Fatalf("failed to initialize registry: %v", err)
	}

	projectRepo := fs.NewProjectRepo()
	artifactRepo := fs.NewArtifactRepo()
	chatRepo := fs.NewChatRepo()

	srv := server.New(cfg, registry, projectRepo, artifactRepo, chatRepo)

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("AI App Factory listening on %s", addr)
	if err := http.ListenAndServe(addr, srv.Router()); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
