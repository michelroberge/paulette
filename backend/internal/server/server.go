package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/cors"

	"github.com/michelroberge/ai-app-factory/backend/internal/config"
	"github.com/michelroberge/ai-app-factory/backend/internal/handler"
	"github.com/michelroberge/ai-app-factory/backend/internal/repository"
	"github.com/michelroberge/ai-app-factory/backend/internal/stream"
)

type Server struct {
	cfg          *config.Config
	registry     repository.RegistryRepo
	projectRepo  repository.ProjectRepo
	artifactRepo repository.ArtifactRepo
	chatRepo     repository.ChatRepo
	runs         *stream.Manager
}

func New(
	cfg *config.Config,
	registry repository.RegistryRepo,
	projectRepo repository.ProjectRepo,
	artifactRepo repository.ArtifactRepo,
	chatRepo repository.ChatRepo,
) *Server {
	return &Server{
		cfg:          cfg,
		registry:     registry,
		projectRepo:  projectRepo,
		artifactRepo: artifactRepo,
		chatRepo:     chatRepo,
		runs:         stream.NewManager(),
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
		AllowedOrigins:   []string{"http://localhost:5173"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
	})
	r.Use(c.Handler)

	ph := handler.NewProjectHandler(s.registry, s.projectRepo, s.artifactRepo)
	plh := handler.NewPipelineHandler(s.registry, s.projectRepo, s.artifactRepo)
	ah := handler.NewArtifactHandler(s.registry, s.artifactRepo)
	ch := handler.NewChatHandler(s.registry, s.chatRepo, s.artifactRepo, s.runs)
	mh := handler.NewMockHandler(s.registry, s.artifactRepo, s.runs)
	bh := handler.NewBeadHandler(s.registry, s.artifactRepo, s.runs)
	rh := handler.NewResetHandler(s.registry, s.projectRepo)
	eh := handler.NewEnhanceHandler(s.registry, s.projectRepo, s.artifactRepo)
	acth := handler.NewActivityHandler(s.runs)

	r.Route("/api/projects", func(r chi.Router) {
		r.Post("/", ph.Create)
		r.Get("/", ph.List)
		r.Get("/{id}", ph.Get)
		r.Delete("/{id}", ph.Delete)

		r.Get("/{id}/pipeline", plh.GetPipeline)
		r.Post("/{id}/pipeline/approve", plh.Approve)
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

		r.Get("/{id}/activity", acth.List)
		r.Get("/{id}/activity/{runId}/stream", acth.Stream)
	})

	return r
}
