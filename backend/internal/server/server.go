package server

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/cors"

	"github.com/michelroberge/claudette/backend/internal/config"
	"github.com/michelroberge/claudette/backend/internal/git"
	"github.com/michelroberge/claudette/backend/internal/handler"
	"github.com/michelroberge/claudette/backend/internal/repository"
	"github.com/michelroberge/claudette/backend/internal/stream"
)

type Server struct {
	cfg          *config.Config
	registry     repository.RegistryRepo
	projectRepo  repository.ProjectRepo
	artifactRepo repository.ArtifactRepo
	chatRepo     repository.ChatRepo
	runs         *stream.Manager
	staticFS     fs.FS
}

func New(
	cfg *config.Config,
	registry repository.RegistryRepo,
	projectRepo repository.ProjectRepo,
	artifactRepo repository.ArtifactRepo,
	chatRepo repository.ChatRepo,
	staticFS fs.FS,
) *Server {
	return &Server{
		cfg:          cfg,
		registry:     registry,
		projectRepo:  projectRepo,
		artifactRepo: artifactRepo,
		chatRepo:     chatRepo,
		runs:         stream.NewManager(),
		staticFS:     staticFS,
	}
}

// Runs returns the stream manager so callers can cancel active runs on shutdown.
func (s *Server) Runs() *stream.Manager {
	return s.runs
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173", "http://localhost:8080"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
	})
	r.Use(c.Handler)

	gitSvc := git.NewService()

	ph := handler.NewProjectHandler(s.registry, s.projectRepo, s.artifactRepo, gitSvc)
	plh := handler.NewPipelineHandler(s.registry, s.projectRepo, s.artifactRepo, s.runs, gitSvc)
	ah := handler.NewArtifactHandler(s.registry, s.artifactRepo)
	ch := handler.NewChatHandler(s.registry, s.chatRepo, s.artifactRepo, s.runs)
	mh := handler.NewMockHandler(s.registry, s.artifactRepo, s.runs)
	bh := handler.NewBeadHandler(s.registry, s.artifactRepo, s.runs)
	rh := handler.NewResetHandler(s.registry, s.projectRepo)
	eh := handler.NewEnhanceHandler(s.registry, s.projectRepo, s.artifactRepo, gitSvc)
	acth := handler.NewActivityHandler(s.runs)
	gh := handler.NewGitHandler(s.registry, s.projectRepo, gitSvc)
	ih := handler.NewImportHandler(s.registry, s.projectRepo, s.artifactRepo, s.runs, gitSvc)

	r.Route("/api/projects", func(r chi.Router) {
		r.Post("/", ph.Create)
		r.Post("/import", ih.Import)
		r.Get("/", ph.List)
		r.Get("/{id}", ph.Get)
		r.Patch("/{id}", ph.Patch)
		r.Delete("/{id}", ph.Delete)

		r.Get("/{id}/pipeline", plh.GetPipeline)
		r.Post("/{id}/pipeline/approve", plh.Approve)
		r.Get("/{id}/pipeline/summary", plh.GetSummary)
		r.Get("/{id}/pipeline/summary/watch", plh.WatchSummary)
		r.Post("/{id}/pipeline/summary", plh.RegenerateSummary)
		r.Post("/{id}/pipeline/enhance", eh.Enhance)

		r.Get("/{id}/stages/{stage}/artifact", ah.Get)

		r.Get("/{id}/stages/{stage}/chat", ch.GetHistory)
		r.Post("/{id}/stages/{stage}/chat", ch.Send)

		r.Get("/{id}/stages/ux/mock", mh.Get)
		r.Post("/{id}/stages/ux/mock", mh.Generate)
		r.Get("/{id}/stages/ux/framework", mh.GetFramework)
		r.Put("/{id}/stages/ux/framework", mh.SetFramework)

		r.Get("/{id}/stages/build/beads", bh.GetGraph)
		r.Get("/{id}/stages/build/beads/watch", bh.Watch)
		r.Post("/{id}/stages/build/beads/generate", bh.Generate)
		r.Post("/{id}/stages/build/beads/execute", bh.Execute)

		r.Post("/{id}/stages/{stage}/reset", rh.Reset)

		r.Get("/{id}/import/watch", ih.WatchImport)

		r.Get("/{id}/activity", acth.List)
		r.Get("/{id}/activity/{runId}/stream", acth.Stream)

		r.Get("/{id}/git/log", gh.Log)
		r.Get("/{id}/git/status", gh.Status)
		r.Post("/{id}/git/reset", gh.Reset)
		r.Post("/{id}/git/discard", gh.Discard)
		r.Put("/{id}/git/remote", gh.SetRemote)
		r.Delete("/{id}/git/remote", gh.RemoveRemote)
		r.Post("/{id}/git/push", gh.Push)
		r.Post("/{id}/git/pull", gh.Pull)
	})

	// Serve embedded frontend static files with SPA fallback.
	if s.staticFS != nil {
		r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
			path := strings.TrimPrefix(r.URL.Path, "/")
			if path == "" {
				path = "index.html"
			}

			// Try to open the requested file.
			f, err := s.staticFS.Open(path)
			if err != nil {
				// File not found — serve index.html for SPA routing.
				indexFile, _ := fs.ReadFile(s.staticFS, "index.html")
				w.Header().Set("Content-Type", "text/html")
				w.Write(indexFile)
				return
			}
			f.Close()

			http.FileServer(http.FS(s.staticFS)).ServeHTTP(w, r)
		})
	}

	return r
}
