package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/cors"

	"github.com/michelroberge/ai-app-factory/backend/internal/config"
	"github.com/michelroberge/ai-app-factory/backend/internal/handler"
	"github.com/michelroberge/ai-app-factory/backend/internal/repository"
)

type Server struct {
	cfg          *config.Config
	registry     repository.RegistryRepo
	projectRepo  repository.ProjectRepo
	artifactRepo repository.ArtifactRepo
	chatRepo     repository.ChatRepo
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
	}
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
	ch := handler.NewChatHandler(s.registry, s.chatRepo, s.artifactRepo)
	mh := handler.NewMockHandler(s.registry, s.artifactRepo)

	r.Route("/api/projects", func(r chi.Router) {
		r.Post("/", ph.Create)
		r.Get("/", ph.List)
		r.Get("/{id}", ph.Get)
		r.Delete("/{id}", ph.Delete)

		r.Get("/{id}/pipeline", plh.GetPipeline)
		r.Post("/{id}/pipeline/approve", plh.Approve)

		r.Get("/{id}/stages/{stage}/artifact", ah.Get)

		r.Get("/{id}/stages/{stage}/chat", ch.GetHistory)
		r.Post("/{id}/stages/{stage}/chat", ch.Send)

		r.Get("/{id}/stages/ux/mock", mh.Get)
		r.Post("/{id}/stages/ux/mock", mh.Generate)
	})

	return r
}
