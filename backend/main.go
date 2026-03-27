package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/michelroberge/paulette/backend/internal/agent"
	"github.com/michelroberge/paulette/backend/internal/cli"
	"github.com/michelroberge/paulette/backend/internal/config"
	"github.com/michelroberge/paulette/backend/internal/provider"
	fsrepo "github.com/michelroberge/paulette/backend/internal/repository/fs"
	"github.com/michelroberge/paulette/backend/internal/server"
)

//go:embed all:static
var staticFiles embed.FS

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			os.Exit(cli.RunInit())
		default:
			fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
			os.Exit(1)
		}
	}

	cfg := config.Load()
	agent.SetClaudePath(cfg.ClaudePath)

	registry, err := fsrepo.NewRegistryRepo(cfg.RegistryPath)
	if err != nil {
		log.Fatalf("failed to initialize registry: %v", err)
	}

	projectRepo := fsrepo.NewProjectRepo()
	artifactRepo := fsrepo.NewArtifactRepo()
	chatRepo := fsrepo.NewChatRepo()

	// Initialise the pluggable provider layer (v0.2.0).
	// ConnectionStore and StageConfigStore load lazily — missing files are treated
	// as empty config, so existing Claude CLI projects continue to work unchanged.
	connStore := provider.NewConnectionStore(cfg.RegistryPath)
	providerRegistry := provider.NewRegistry(connStore)
	stageConfig := provider.NewStageConfigStore(cfg.RegistryPath)

	// Strip the "static" prefix so files are served from "/".
	staticSub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("failed to load embedded static files: %v", err)
	}

	srv := server.New(cfg, registry, projectRepo, artifactRepo, chatRepo, staticSub, connStore, providerRegistry, stageConfig)

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("paulette listening on %s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Resume autonomous pipelines for any projects that were running before restart
	go srv.Orchestrator().StartAll()

	// Wait for SIGINT or SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("received %s, shutting down...", sig)

	// Cancel autonomous orchestrators, then active Claude processes
	srv.Orchestrator().CancelAll()
	srv.Runs().CancelAll()
	log.Println("cancelled all active agent runs")

	// Gracefully shut down the HTTP server (90s deadline for in-flight requests)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}

	log.Println("server stopped")
}
