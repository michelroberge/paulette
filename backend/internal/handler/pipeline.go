package handler

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/paulette/backend/internal/agent"
	"github.com/michelroberge/paulette/backend/internal/git"
	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/pipeline"
	"github.com/michelroberge/paulette/backend/internal/provider"
	"github.com/michelroberge/paulette/backend/internal/repository"
	fsrepo "github.com/michelroberge/paulette/backend/internal/repository/fs"
	"github.com/michelroberge/paulette/backend/internal/stream"
)

type PipelineHandler struct {
	registry         repository.RegistryRepo
	projectRepo      repository.ProjectRepo
	artifactRepo     repository.ArtifactRepo
	activityRepo     repository.ActivityRepo
	chatRepo         repository.ChatRepo
	runs             *stream.Manager
	git              *git.Service
	providerRegistry *provider.Registry
	stageConfig      *provider.StageConfigStore
}

func NewPipelineHandler(registry repository.RegistryRepo, projectRepo repository.ProjectRepo, artifactRepo repository.ArtifactRepo, activityRepo repository.ActivityRepo, chatRepo repository.ChatRepo, runs *stream.Manager, gitSvc *git.Service, providerRegistry *provider.Registry, stageConfig *provider.StageConfigStore) *PipelineHandler {
	return &PipelineHandler{
		registry:         registry,
		projectRepo:      projectRepo,
		artifactRepo:     artifactRepo,
		activityRepo:     activityRepo,
		chatRepo:         chatRepo,
		runs:             runs,
		git:              gitSvc,
		providerRegistry: providerRegistry,
		stageConfig:      stageConfig,
	}
}

// resolveProvider returns the Provider and model ID to use for the given stage,
// falling back to ClaudeCLI when no registry is configured.
func (h *PipelineHandler) resolveProvider(projectID, hostDir string, stage model.StageName) (provider.Provider, string, error) {
	if h.providerRegistry != nil {
		return h.providerRegistry.ResolveForStage(projectID, stage, h.stageConfig, hostDir)
	}
	return provider.NewClaudeCLIProvider(), provider.FallbackModel(stage), nil
}

func (h *PipelineHandler) GetPipeline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	state := h.buildState(project)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

// buildState assembles pipeline state enriched with per-stage activity.
func (h *PipelineHandler) buildState(project *model.Project) model.PipelineState {
	liveRuns := h.runs.ActiveForProject(project.ID)
	persisted, _ := h.activityRepo.ReadActivity(project.HostDir)
	activities := mergeActivities(liveRuns, persisted)
	return pipeline.BuildPipelineState(project.CurrentStage, activities, project.SummaryApproved)
}

// WatchPipeline is an SSE endpoint that pushes updated PipelineState whenever runs change.
func (h *PipelineHandler) WatchPipeline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	setSSEHeaders(w)

	emit := func() {
		p, e := h.registry.Get(id)
		if e != nil {
			return
		}
		state := h.buildState(p)
		data, _ := json.Marshal(state)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	emit() // send initial state immediately

	ch, subID := h.runs.Watch(id)
	defer h.runs.Unwatch(id, subID)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ch:
			emit()
		case <-ticker.C:
			emit()
		case <-r.Context().Done():
			return
		}
	}

	_ = project // suppress unused warning; project used only for 404 check above
}

type approveResponse struct {
	PreviousStage string `json:"previousStage"`
	CurrentStage  string `json:"currentStage"`
}

func (h *PipelineHandler) Approve(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	previousStage := project.CurrentStage

	if err := h.ApproveInternal(id); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "failed to") {
			status = http.StatusInternalServerError
		}
		http.Error(w, err.Error(), status)
		return
	}

	project, _ = h.registry.Get(id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(approveResponse{
		PreviousStage: string(previousStage),
		CurrentStage:  string(project.CurrentStage),
	})
}

// ApproveInternal advances the pipeline without HTTP plumbing.
func (h *PipelineHandler) ApproveInternal(projectID string) error {
	project, err := h.registry.Get(projectID)
	if err != nil {
		return fmt.Errorf("project not found: %w", err)
	}

	// Check artifact exists for current stage (check both .paulette and docs/)
	artifactContent, err := h.artifactRepo.ReadWithFallback(project.HostDir, project.Version, project.CurrentStage)
	if err != nil {
		return fmt.Errorf("failed to check artifact: %w", err)
	}
	if strings.TrimSpace(artifactContent) == "" {
		return fmt.Errorf("cannot approve: no artifact for current stage")
	}

	// Require all beads closed before approving build
	if project.CurrentStage == model.StageBuild {
		graph, err := fsrepo.ReadBdBeadGraph(context.Background(), project.HostDir)
		if err != nil {
			return fmt.Errorf("cannot approve: failed to read bead graph: %w", err)
		}
		for _, b := range graph.Beads {
			if b.Status != model.BeadStatusClosed {
				return fmt.Errorf("cannot approve: bead %s (%s) is not closed", b.ID, b.Title)
			}
		}
	}

	// Extract validation commands when approving architecture
	if project.CurrentStage == model.StageArchitecture {
		dev, build, run := pipeline.ParseValidationCommands(artifactContent)
		project.DevCommands = dev
		project.BuildCommands = build
		project.RunCommands = run
	}

	previousStage := project.CurrentStage
	nextStage, err := pipeline.NextStage(project.CurrentStage)
	if err != nil {
		return err
	}

	project.CurrentStage = nextStage
	project.UpdatedAt = time.Now()
	if nextStage == model.StageComplete {
		project.SummaryReady = false
	}

	if err := h.registry.Update(project); err != nil {
		return fmt.Errorf("failed to update registry: %w", err)
	}
	if err := h.projectRepo.Save(project.HostDir, project); err != nil {
		return fmt.Errorf("failed to save project: %w", err)
	}

	// Promote artifact from .paulette to docs
	if artifact, readErr := h.artifactRepo.Read(project.HostDir, previousStage); readErr == nil && artifact != "" {
		if docErr := fsrepo.WriteStageDoc(project.HostDir, project.Version, previousStage, artifact); docErr != nil {
			log.Printf("docs promotion failed for %s: %v", previousStage, docErr)
		} else {
			aiFactoryPath := filepath.Join(project.HostDir, ".paulette", string(previousStage), string(previousStage)+".md")
			if rmErr := os.Remove(aiFactoryPath); rmErr != nil {
				log.Printf("failed to remove .paulette artifact %s: %v", aiFactoryPath, rmErr)
			}
		}
	}

	// Promote UX mock.html to docs
	if previousStage == model.StageUX {
		mockPath := filepath.Join(project.HostDir, ".paulette", "ux", "mock.html")
		if mockBytes, readErr := os.ReadFile(mockPath); readErr == nil && len(mockBytes) > 0 {
			if docErr := fsrepo.WriteMockDoc(project.HostDir, project.Version, mockBytes); docErr != nil {
				log.Printf("mock.html promotion failed: %v", docErr)
			}
		}
	}

	commitMsg := fmt.Sprintf("approve(%s): promote artifact to docs", previousStage)
	if err := h.git.AddAllAndCommit(project.HostDir, commitMsg); err != nil {
		log.Printf("git commit failed for %s: %v", previousStage, err)
	}

	if nextStage == model.StageComplete {
		h.startSummaryRun(project)
	}

	return nil
}

// startSummaryRun creates a managed run for summary generation and kicks it off in a goroutine.
func (h *PipelineHandler) startSummaryRun(project *model.Project) {
	run := h.runs.Start(project.ID, "complete", "summary")
	if run == nil {
		return // already running
	}
	writeActivity(h.activityRepo, project.HostDir, model.StageComplete, "summary")

	// Collect artifacts before entering the goroutine.
	artifacts := make(map[model.StageName]string)
	for _, s := range pipeline.StageOrder {
		if s == model.StageComplete {
			break
		}
		content, _ := h.artifactRepo.ReadWithFallback(project.HostDir, project.Version, s)
		if content != "" {
			artifacts[s] = content
		}
	}

	// Resolve provider before entering the goroutine.
	prov, modelID, provErr := h.resolveProvider(project.ID, project.HostDir, model.StageComplete)
	if provErr != nil {
		failActivity(h.activityRepo, project.HostDir, model.StageComplete, "summary", provErr.Error())
		run.Emit(agent.StreamEvent{Type: "error", Content: provErr.Error()})
		clearActivity(h.activityRepo, project.HostDir, model.StageComplete)
		run.Finish(h.runs)
		return
	}

	// Pass 0 for maxContextChars (no truncation) — the provider's num_ctx
	// setting handles context limits. Callers can pass a budget for constrained models.
	systemPrompt, userMsg := agent.BuildStreamSummaryRequest(artifacts, project.Name, project.Version, 0)

	go func() {
		// LIFO defer: clearActivity runs first, then run.Finish (so watcher sees clean state)
		defer run.Finish(h.runs)
		defer clearActivity(h.activityRepo, project.HostDir, model.StageComplete)

		events, err := prov.Chat(run.Context(), provider.ChatRequest{
			Model:        modelID,
			SystemPrompt: systemPrompt,
			UserMessage:  userMsg,
			ProjectDir:   project.HostDir,
		})
		if err != nil {
			failActivity(h.activityRepo, project.HostDir, model.StageComplete, "summary", err.Error())
			run.Emit(agent.StreamEvent{Type: "error", Content: err.Error()})
			return
		}

		runStart := time.Now()
		var fullText strings.Builder
		var tokens int
		for ev := range events {
			if ev.Type == "chunk" {
				fullText.WriteString(ev.Content)
			} else if ev.Type == "tokens" {
				fmt.Sscanf(ev.Content, "%d", &tokens)
			}
			run.Emit(ev)
		}

		parsed := agent.ParseResponse(fullText.String()).Discussion
		if parsed == "" {
			parsed = strings.TrimSpace(fullText.String())
		}
		summary := parsed
		if summary == "" {
			failActivity(h.activityRepo, project.HostDir, model.StageComplete, "summary", "summary generation produced no output")
			run.Emit(agent.StreamEvent{Type: "error", Content: "summary generation produced no output"})
			return
		}

		summaryPath := filepath.Join(project.HostDir, ".paulette", "summary.md")
		if err := os.WriteFile(summaryPath, []byte(summary), 0644); err != nil {
			log.Printf("failed to write summary: %v", err)
			run.Emit(agent.StreamEvent{Type: "error", Content: "failed to write summary"})
			return
		}

		project.SummaryReady = true
		project.SummaryTokens = tokens
		project.AddStageTokens(model.StageComplete, tokens)
		recordSession(project.HostDir, model.StageComplete, model.SessionSummaryKind, project.Iteration, runStart, tokens)
		project.UpdatedAt = time.Now()
		if err := h.registry.Update(project); err != nil {
			log.Printf("failed to update registry after summary: %v", err)
		}
		if err := h.projectRepo.Save(project.HostDir, project); err != nil {
			log.Printf("failed to save project after summary: %v", err)
		}

		commitMsg := fmt.Sprintf("complete(v%s): iteration summary", project.Version)
		if err := h.git.AddAndCommit(project.HostDir, []string{".paulette/summary.md"}, commitMsg); err != nil {
			log.Printf("git commit summary failed: %v", err)
		}
		// Tag the completed version
		tagName := "v" + project.Version
		if err := h.git.CreateTag(project.HostDir, tagName, fmt.Sprintf("Iteration %d complete", project.Iteration)); err != nil {
			log.Printf("git tag %s failed: %v", tagName, err)
		}

		// Signal completion to streaming clients
		run.Emit(agent.StreamEvent{Type: "done", Content: summary})
	}()
}

// WatchSummary streams the live summary generation as SSE.
func (h *PipelineHandler) WatchSummary(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.registry.Get(id); err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	run := h.runs.Active(id, "complete", "summary")
	if run == nil {
		// No active run — send a done event so the client doesn't hang
		setSSEHeaders(w)
		fmt.Fprintf(w, "data: {\"type\":\"done\",\"content\":\"\"}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		return
	}
	run.StreamTo(w, r, 0)
}

// GetSummary returns the generated summary markdown for a completed project.
func (h *PipelineHandler) GetSummary(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	approved := project.SummaryApproved
	summaryPath := filepath.Join(project.HostDir, ".paulette", "summary.md")
	b, err := os.ReadFile(summaryPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Fall back to the approved docs copy
			docPath := filepath.Join(project.HostDir, "docs", project.Version, "summary.md")
			b, err = os.ReadFile(docPath)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{"content": "", "exists": false})
				return
			}
			// Only treat as approved if the project record confirms it;
			// a new iteration may have a docs copy from a prior iteration.
		} else {
			http.Error(w, "failed to read summary: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"content": string(b), "exists": true, "approved": approved})
}

// RegenerateSummary allows the client to re-trigger summary generation if it failed.
func (h *PipelineHandler) RegenerateSummary(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	if project.CurrentStage != model.StageComplete {
		http.Error(w, "project must be at complete stage", http.StatusBadRequest)
		return
	}
	// Reset the flag so the frontend knows generation is in progress
	project.SummaryReady = false
	project.UpdatedAt = time.Now()
	if err := h.registry.Update(project); err != nil {
		http.Error(w, "failed to update registry: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.projectRepo.Save(project.HostDir, project); err != nil {
		http.Error(w, "failed to save project: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.startSummaryRun(project)
	w.WriteHeader(http.StatusAccepted)
}

// ApproveSummary promotes the generated summary from .paulette/summary.md to docs/{version}/summary.md.
func (h *PipelineHandler) ApproveSummary(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	if !project.SummaryReady {
		http.Error(w, "summary not ready", http.StatusBadRequest)
		return
	}

	summaryPath := filepath.Join(project.HostDir, ".paulette", "summary.md")
	b, err := os.ReadFile(summaryPath)
	if err != nil {
		http.Error(w, "failed to read summary: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := fsrepo.WriteSummaryDoc(project.HostDir, project.Version, string(b)); err != nil {
		http.Error(w, "failed to write summary doc: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Archive stage chat histories into the versioned docs directory.
	archiveStages := []model.StageName{model.StageVision, model.StageUX, model.StageArchitecture, model.StageBuild}
	for _, stage := range archiveStages {
		msgs, err := h.chatRepo.GetHistory(project.HostDir, stage)
		if err != nil {
			log.Printf("failed to read chat history for stage %s: %v", stage, err)
			continue
		}
		if len(msgs) == 0 {
			continue
		}
		history := model.ChatHistory{Messages: msgs}
		if err := fsrepo.WriteStageChatHistory(project.HostDir, project.Version, stage, history); err != nil {
			log.Printf("failed to archive chat history for stage %s: %v", stage, err)
		}
	}

	// Snapshot the beads graph if present.
	beadGraphBytes, err := os.ReadFile(fsrepo.BeadGraphPath(project.HostDir))
	if err == nil && len(beadGraphBytes) > 0 {
		if err := fsrepo.WriteBeadsGraphSnapshot(project.HostDir, project.Version, beadGraphBytes); err != nil {
			log.Printf("failed to snapshot beads graph: %v", err)
		}
	}

	// Write version-meta.json linking this approval to its git commit.
	commitHash := h.git.CurrentHash(project.HostDir)
	meta := model.VersionMeta{
		Version:    project.Version,
		Iteration:  project.Iteration,
		CommitHash: commitHash,
		TagName:    "v" + project.Version,
		ApprovedAt: time.Now(),
	}
	if err := fsrepo.WriteVersionMeta(project.HostDir, project.Version, meta); err != nil {
		log.Printf("failed to write version meta: %v", err)
	}

	if err := fsrepo.WriteReadme(project); err != nil {
		log.Printf("failed to generate README.md: %v", err)
	}

	// Build the list of paths to commit, including newly archived files.
	commitPaths := []string{
		filepath.Join("docs", project.Version, "summary.md"),
		filepath.Join("docs", project.Version, "version-meta.json"),
		"README.md",
	}
	for _, stage := range archiveStages {
		commitPaths = append(commitPaths, filepath.Join("docs", project.Version, string(stage), "chat-history.json"))
	}
	commitPaths = append(commitPaths, filepath.Join("docs", project.Version, "build", "beads-graph.json"))

	commitMsg := fmt.Sprintf("docs(v%s): approve iteration summary", project.Version)
	if err := h.git.AddAndCommit(project.HostDir, commitPaths, commitMsg); err != nil {
		log.Printf("git commit approved summary failed: %v", err)
	}

	if project.BaseBranch != "" {
		branchName := fmt.Sprintf("pauline/iteration-%d", project.Iteration)
		st, _ := h.git.Status(project.HostDir)
		if st.RemoteURL != "" {
			if err := h.git.Push(project.HostDir, branchName, branchName, false); err != nil {
				log.Printf("push iteration branch failed: %v", err)
			} else {
				prBase := project.BaseBranch
				if project.Iteration > 1 {
					prevBranch := fmt.Sprintf("pauline/iteration-%d", project.Iteration-1)
					if h.git.RemoteBranchExists(project.HostDir, prevBranch) {
						prBase = prevBranch
					}
				}
				prTitle := fmt.Sprintf("Iteration %d (v%s)", project.Iteration, project.Version)
				if err := h.git.CreatePullRequest(project.HostDir, prTitle, prBase); err != nil {
					log.Printf("gh pr create failed: %v", err)
				}
			}
		}
	}

	project.SummaryApproved = true
	project.UpdatedAt = time.Now()
	if err := h.registry.Update(project); err != nil {
		log.Printf("failed to update registry after summary approve: %v", err)
	}
	if err := h.projectRepo.Save(project.HostDir, project); err != nil {
		log.Printf("failed to save project after summary approve: %v", err)
	}

	w.WriteHeader(http.StatusNoContent)
}

// DownloadProject zips the project's HostDir and sends it as a zip file download.
func (h *PipelineHandler) DownloadProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	safeName := strings.ReplaceAll(project.Name, " ", "_")
	filename := safeName + "-v" + project.Version + ".zip"

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")

	zw := zip.NewWriter(w)
	defer zw.Close()

	baseDir := project.HostDir
	err = filepath.Walk(baseDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}
		// Use forward slashes in zip entries
		rel = filepath.ToSlash(rel)

		fw, err := zw.Create(rel)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(fw, f)
		return err
	})
	if err != nil {
		log.Printf("download zip error for project %s: %v", id, err)
	}
}
