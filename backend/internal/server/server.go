package server

import (
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/cors"

	"github.com/michelroberge/paulette/backend/internal/autopilot"
	"github.com/michelroberge/paulette/backend/internal/config"
	"github.com/michelroberge/paulette/backend/internal/git"
	"github.com/michelroberge/paulette/backend/internal/handler"
	authmw "github.com/michelroberge/paulette/backend/internal/middleware"
	"github.com/michelroberge/paulette/backend/internal/provider"
	"github.com/michelroberge/paulette/backend/internal/rag"
	"github.com/michelroberge/paulette/backend/internal/repository"
	fsrepo "github.com/michelroberge/paulette/backend/internal/repository/fs"
	"github.com/michelroberge/paulette/backend/internal/stream"
)

type Server struct {
	cfg              *config.Config
	registry         repository.RegistryRepo
	projectRepo      repository.ProjectRepo
	artifactRepo     repository.ArtifactRepo
	activityRepo     repository.ActivityRepo
	chatRepo         repository.ChatRepo
	runs             *stream.Manager
	staticFS         fs.FS
	orchestrator     *autopilot.Orchestrator
	connStore        *provider.ConnectionStore
	providerRegistry *provider.Registry
	stageConfig      *provider.StageConfigStore
	gitIdentity      *git.GlobalIdentityStore
	ragClient        *rag.Client
	oidcHandler      *handler.OIDCHandler
}

func New(
	cfg *config.Config,
	registry repository.RegistryRepo,
	projectRepo repository.ProjectRepo,
	artifactRepo repository.ArtifactRepo,
	chatRepo repository.ChatRepo,
	staticFS fs.FS,
	connStore *provider.ConnectionStore,
	providerRegistry *provider.Registry,
	stageConfig *provider.StageConfigStore,
	ragClient *rag.Client,
) *Server {
	srv := &Server{
		cfg:              cfg,
		registry:         registry,
		projectRepo:      projectRepo,
		artifactRepo:     artifactRepo,
		activityRepo:     fsrepo.NewActivityRepo(),
		chatRepo:         chatRepo,
		runs:             stream.NewManager(),
		staticFS:         staticFS,
		connStore:        connStore,
		providerRegistry: providerRegistry,
		stageConfig:      stageConfig,
		gitIdentity:      git.NewGlobalIdentityStore(cfg.RegistryPath),
		ragClient:        ragClient,
	}
	if cfg.OIDC.Enabled {
		oidcH, err := handler.NewOIDCHandler(&cfg.OIDC)
		if err != nil {
			log.Printf("WARNING: OIDC enabled but provider init failed: %v — running without OIDC", err)
			// Mark OIDC as disabled so the config endpoint and middleware
			// don't advertise/enforce auth that can't actually work.
			cfg.OIDC.Enabled = false
		} else {
			srv.oidcHandler = oidcH
			log.Println("OIDC authentication enabled")
		}
	}
	return srv
}

// Runs returns the stream manager so callers can cancel active runs on shutdown.
func (s *Server) Runs() *stream.Manager {
	return s.runs
}

// Orchestrator returns the autopilot orchestrator.
func (s *Server) Orchestrator() *autopilot.Orchestrator {
	return s.orchestrator
}

// RAGClient returns the RAG integration client (may be nil).
func (s *Server) RAGClient() *rag.Client {
	return s.ragClient
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	r.Use(func(next http.Handler) http.Handler {
		logger := chimiddleware.Logger(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Log mutating requests immediately on arrival so streaming
			// endpoints (SSE) show up in logs before the response completes.
			if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
				log.Printf("[%s] %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
			}
			if r.URL.Path == "/api/projects/activity" {
				next.ServeHTTP(w, r)
				return
			}
			logger.ServeHTTP(w, r)
		})
	})
	r.Use(chimiddleware.Recoverer)

	allowedOrigins := []string{"http://localhost:5173", "http://localhost:8080"}
	if s.cfg.OIDC.RedirectURI != "" {
		if u, err := url.Parse(s.cfg.OIDC.RedirectURI); err == nil {
			origin := u.Scheme + "://" + u.Host
			allowedOrigins = append(allowedOrigins, origin)
		}
	}
	c := cors.New(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
	})
	r.Use(c.Handler)

	// Apply OIDC session guard to all /api/* routes when OIDC is enabled.
	// The middleware skips /api/auth/* and /api/config paths so the login
	// flow and config discovery are always reachable.
	r.Use(authmw.OIDCAuth(s.cfg.OIDC.Enabled && s.oidcHandler != nil, s.cfg.OIDC.SessionSecret))

	gitSvc := git.NewService()
	skillRepo := fsrepo.NewSkillRepo(s.cfg.RegistryPath)
	logBase := filepath.Join(s.cfg.RegistryPath, "log")

	// Instantiate build pool from persisted config (nil if not configured).
	var buildPool *provider.BuildPool
	if poolCfg := s.stageConfig.GetBuildPool(); poolCfg != nil {
		buildPool = provider.NewBuildPool(poolCfg, s.providerRegistry)
		if buildPool != nil {
			log.Printf("Build pool started with %d slot(s)", len(poolCfg.Slots))
		}
	}

	ph := handler.NewProjectHandler(s.registry, s.projectRepo, s.artifactRepo, gitSvc, s.cfg.ReposPath)
	plh := handler.NewPipelineHandler(s.registry, s.projectRepo, s.artifactRepo, s.activityRepo, s.chatRepo, s.runs, gitSvc, s.providerRegistry, s.stageConfig, s.ragClient, logBase)
	ah := handler.NewArtifactHandler(s.registry, s.artifactRepo)
	ch := handler.NewChatHandler(s.registry, s.chatRepo, s.artifactRepo, s.activityRepo, s.runs, s.providerRegistry, s.stageConfig, s.connStore, s.ragClient, logBase)
	mh := handler.NewMockHandler(s.registry, s.artifactRepo, s.activityRepo, s.runs, s.providerRegistry, s.stageConfig, s.connStore, logBase)
	bh := handler.NewBeadHandler(s.registry, s.projectRepo, s.artifactRepo, s.activityRepo, s.runs, skillRepo, logBase)
	bh.SetProviderRegistry(s.providerRegistry, s.stageConfig)
	bh.SetBuildPool(buildPool)
	bh.SetRAGClient(s.ragClient)
	instructH := handler.NewInstructHandler(s.registry, s.artifactRepo, s.activityRepo, s.runs)
	instructH.SetProviderRegistry(s.providerRegistry, s.stageConfig)
	rh := handler.NewResetHandler(s.registry, s.projectRepo)
	eh := handler.NewEnhanceHandler(s.registry, s.projectRepo, s.artifactRepo, gitSvc)
	acth := handler.NewActivityHandler(s.runs, s.activityRepo, s.registry)
	gh := handler.NewGitHandler(s.registry, s.projectRepo, gitSvc)
	ggh := handler.NewGitGlobalHandler(gitSvc, s.gitIdentity)
	ih := handler.NewImportHandler(s.registry, s.projectRepo, s.artifactRepo, s.runs, gitSvc, s.cfg.ReposPath, s.gitIdentity)
	sh := handler.NewSessionHandler(s.registry)
	vh := handler.NewVersionHandler(s.registry)
	skh := handler.NewSkillHandler(s.registry, s.artifactRepo, s.activityRepo, skillRepo, s.runs, s.providerRegistry, s.stageConfig)
	rfh := handler.NewRefineHandler(s.registry, s.artifactRepo, s.chatRepo, s.runs, s.providerRegistry, s.stageConfig, s.connStore, logBase)

	// Wire orchestrator (created once, reused across Router calls)
	if s.orchestrator == nil {
		s.orchestrator = autopilot.NewOrchestrator(
			s.registry, s.artifactRepo, s.activityRepo, s.runs,
			ch, mh, bh, plh, eh,
		)
	}
	ph.SetOrchestrator(s.orchestrator)

	cfgH := handler.NewConfigHandler(s.cfg)
	r.Get("/api/config", cfgH.GetInfo)

	ragH := handler.NewRAGHandler(s.ragClient)
	r.Get("/api/rag/status", ragH.Status)

	// Connection CRUD, test, and model-discovery endpoints.
	connH := handler.NewConnectionHandler(s.connStore, s.providerRegistry, s.stageConfig, s.registry)
	r.Route("/api/connections", connH.RegisterRoutes)

	// Stage-config global defaults and operation-level overrides
	scfgH := handler.NewStageConfigHandler(s.stageConfig, s.connStore, s.registry)
	scfgH.SetBuildPoolRuntime(buildPool)
	r.Get("/api/config/stages", scfgH.GetGlobalDefaults)
	r.Put("/api/config/stages/{stage}", scfgH.SetGlobalStageDefault)
	r.Put("/api/config/operations/{operation}", scfgH.SetGlobalOperationDefault)
	r.Delete("/api/config/operations/{operation}", scfgH.DeleteGlobalOperationDefault)
	// Build pool config
	r.Get("/api/config/build-pool", scfgH.GetBuildPool)
	r.Put("/api/config/build-pool", scfgH.SetBuildPool)
	r.Delete("/api/config/build-pool", scfgH.DeleteBuildPool)
	r.Get("/api/config/build-pool/status", scfgH.GetBuildPoolStatus)

	authH := handler.NewAuthHandler(s.cfg.ClaudePath)
	r.Get("/api/auth/status", authH.Status)
	r.Get("/api/auth/login", authH.Login)
	r.Post("/api/auth/login/input", authH.LoginInput)
	r.Post("/api/auth/logout", authH.Logout)

	if s.oidcHandler != nil {
		r.Get("/api/auth/oidc/login", s.oidcHandler.Login)
		r.Get("/api/auth/oidc/callback", s.oidcHandler.Callback)
		r.Get("/api/auth/oidc/logout", s.oidcHandler.Logout)
		r.Get("/api/auth/oidc/me", s.oidcHandler.Me)
	}

	r.Get("/api/git/ssh-key", ggh.SSHKey)
	r.Get("/api/git/identity", ggh.GetIdentity)
	r.Post("/api/git/identity", ggh.SetIdentity)

	r.Route("/api/projects", func(r chi.Router) {
		r.Get("/activity", acth.Summary)
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
		r.Post("/{id}/pipeline/summary/approve", plh.ApproveSummary)
		r.Get("/{id}/download", plh.DownloadProject)
		r.Post("/{id}/pipeline/enhance", eh.Enhance)

		r.Get("/{id}/stages/{stage}/artifact", ah.Get)
		r.Post("/{id}/stages/{stage}/artifact/refine", rfh.Refine)
		r.Post("/{id}/stages/{stage}/artifact/manual-edit", rfh.ManualEdit)
		r.Post("/{id}/stages/{stage}/artifact/apply-refine", rfh.ApplyRefine)

		r.Get("/{id}/stages/{stage}/chat", ch.GetHistory)
		r.Post("/{id}/stages/{stage}/chat", ch.Send)
		r.Post("/{id}/stages/{stage}/chat/resume", ch.Resume)

		r.Get("/{id}/stages/ux/mock", mh.Get)
		r.Post("/{id}/stages/ux/mock", mh.Generate)
		r.Get("/{id}/stages/ux/framework", mh.GetFramework)
		r.Put("/{id}/stages/ux/framework", mh.SetFramework)

		r.Get("/{id}/stages/build/beads", bh.GetGraph)
		r.Get("/{id}/stages/build/beads/watch", bh.Watch)
		r.Post("/{id}/stages/build/beads/generate", bh.Generate)
		r.Post("/{id}/stages/build/beads/execute", bh.Execute)
		r.Post("/{id}/stages/build/beads/instruct", instructH.Plan)
		r.Post("/{id}/stages/build/beads/instruct/apply", instructH.Apply)
		r.Get("/{id}/stages/build/beads/{beadId}", bh.GetDetail)
		r.Patch("/{id}/stages/build/beads/{beadId}", bh.UpdateBead)
		r.Post("/{id}/stages/build/beads/{beadId}/control", bh.ControlBead)
		r.Post("/{id}/stages/build/beads/{beadId}/execute", bh.ExecuteSingleBead)
		r.Post("/{id}/stages/build/beads/{beadId}/chat", bh.BeadChat)
		r.Get("/{id}/stages/build/beads/{beadId}/files", bh.GetBeadFiles)
		r.Get("/{id}/stages/build/beads/{beadId}/diff", bh.GetBeadDiff)

		r.Post("/{id}/stages/{stage}/reset", rh.Reset)

		r.Get("/{id}/import/watch", ih.WatchImport)

		r.Get("/{id}/pipeline/watch", plh.WatchPipeline)

		r.Get("/{id}/activity", acth.List)
		r.Post("/{id}/activity/{runId}/btw", acth.SendBtw)
		r.Get("/{id}/activity/{runId}/stream", acth.Stream)

		r.Get("/{id}/git/log", gh.Log)
		r.Get("/{id}/git/status", gh.Status)
		r.Get("/{id}/git/diff", gh.WorkingDiff)
		r.Post("/{id}/git/reset", gh.Reset)
		r.Post("/{id}/git/discard", gh.Discard)
		r.Post("/{id}/git/commit", gh.Commit)
		r.Post("/{id}/git/branch/rename", gh.RenameBranch)
		r.Post("/{id}/git/identity", gh.SetIdentity)
		r.Get("/{id}/git/ssh-key", gh.SSHKey)
		r.Put("/{id}/git/remote", gh.SetRemote)
		r.Delete("/{id}/git/remote", gh.RemoveRemote)
		r.Post("/{id}/git/push", gh.Push)
		r.Post("/{id}/git/pull", gh.Pull)
		r.Get("/{id}/git/branches", gh.ListBranches)

		r.Get("/{id}/sessions", sh.ListSessions)

		r.Post("/{id}/stages/build/skills/analyze", skh.Analyze)
		r.Get("/{id}/stages/build/skills/suggestions", skh.GetSuggestions)
		r.Get("/{id}/stages/build/skills/observed", skh.GetObserved)
		r.Post("/{id}/stages/build/skills/approve", skh.ApproveSuggestions)

		// Per-project stage-config overrides.
		r.Get("/{id}/config/stages", scfgH.GetProjectOverrides)
		r.Put("/{id}/config/stages/{stage}", scfgH.SetProjectStageOverride)
		r.Post("/{id}/config/stages/reset", scfgH.ResetProjectOverrides)
		// Per-project operation overrides.
		r.Put("/{id}/config/operations/{operation}", scfgH.SetProjectOperationOverride)
		r.Delete("/{id}/config/operations/{operation}", scfgH.DeleteProjectOperationOverride)

		// Version history (read-only).
		r.Get("/{id}/versions", vh.ListVersions)
		r.Get("/{id}/versions/{version}", vh.GetVersionSnapshot)
		r.Get("/{id}/versions/{version}/beads/{beadId}", vh.GetVersionBeadExecution)
		r.Get("/{id}/versions/{version}/mock", vh.GetVersionMock)
	})

	rlh := handler.NewRunLogHandler(s.registry, logBase)
	r.Get("/api/run-log", rlh.GetAll)
	r.Get("/api/projects/{id}/run-log", rlh.GetForProject)
	r.Get("/api/projects/{id}/run-log/{runId}/trace/{filename}", rlh.GetTraceFile)
	r.Delete("/api/projects/{id}/run-log/{runId}", rlh.DeleteRun)
	r.Post("/api/projects/{id}/run-log/prune", rlh.PruneRuns)

	r.Route("/api/skills", func(r chi.Router) {
		r.Get("/", skh.ListAll)
		r.Get("/{skillId}", skh.GetSkill)
	})

	// Serve embedded frontend static files with SPA fallback.
	// Never serve index.html for /api/ paths — return 404 instead so
	// missing API routes don't silently serve the SPA.
	if s.staticFS != nil {
		r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}
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
