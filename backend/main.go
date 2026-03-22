package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/michelroberge/claudette/backend/internal/config"
	"github.com/michelroberge/claudette/backend/internal/repository/fs"
	"github.com/michelroberge/claudette/backend/internal/server"
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
	httpServer := &http.Server{
		Addr:    addr,
		Handler: srv.Router(),
	}

	// Start server in a goroutine
	go func() {
		log.Printf("AI App Factory listening on %s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Wait for SIGINT or SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("received %s, shutting down...", sig)

	// Cancel all active Claude processes
	srv.Runs().CancelAll()
	log.Println("cancelled all active agent runs")

	// Gracefully shut down the HTTP server (5s deadline for in-flight requests)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}

	log.Println("server stopped")
}
