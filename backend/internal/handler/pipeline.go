package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/Claudine/backend/internal/agent"
	"github.com/michelroberge/Claudine/backend/internal/git"
	"github.com/michelroberge/Claudine/backend/internal/model"
	"github.com/michelroberge/Claudine/backend/internal/pipeline"
	"github.com/michelroberge/Claudine/backend/internal/repository"
	fsrepo "github.com/michelroberge/Claudine/backend/internal/repository/fs"
	"github.com/michelroberge/Claudine/backend/internal/stream"
)

type PipelineHandler struct {
	registry     repository.RegistryRepo
	projectRepo  repository.ProjectRepo
	artifactRepo repository.ArtifactRepo
	runs         *stream.Manager
	git          *git.Service
}

func NewPipelineHandler(registry repository.RegistryRepo, projectRepo repository.ProjectRepo, artifactRepo repository.ArtifactRepo, runs *stream.Manager, gitSvc *git.Service) *PipelineHandler {
	return &PipelineHandler{
		registry:     registry,
		projectRepo:  projectRepo,
		artifactRepo: artifactRepo,
		runs:         runs,
		git:          gitSvc,
	}
}

func (h *PipelineHandler) GetPipeline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	state := pipeline.BuildPipelineState(project.CurrentStage)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
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

	// Check artifact exists for current stage (check both .claudine and docs/)
	artifactContent, err := h.artifactRepo.ReadWithFallback(project.HostDir, project.Version, project.CurrentStage)
	if err != nil {
		http.Error(w, "failed to check artifact: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if strings.TrimSpace(artifactContent) == "" {
		http.Error(w, "cannot approve: no artifact for current stage", http.StatusBadRequest)
		return
	}

	// Extract validation commands when approving architecture
	if project.CurrentStage == model.StageArchitecture {
		dev, build, run := pipeline.ParseValidationCommands(artifactContent)
		project.DevCommands = dev
		project.BuildCommands = build
		project.RunCommands = run
	}

	// Advance to next stage
	previousStage := project.CurrentStage
	nextStage, err := pipeline.NextStage(project.CurrentStage)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	project.CurrentStage = nextStage
	project.UpdatedAt = time.Now()
	if nextStage == model.StageComplete {
		project.SummaryReady = false
	}

	// Update both registry and project file
	if err := h.registry.Update(project); err != nil {
		http.Error(w, "failed to update registry: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.projectRepo.Save(project.HostDir, project); err != nil {
		http.Error(w, "failed to save project: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Promote artifact from .claudine to docs and remove from .claudine
	if artifact, readErr := h.artifactRepo.Read(project.HostDir, previousStage); readErr == nil && artifact != "" {
		if docErr := fsrepo.WriteStageDoc(project.HostDir, project.Version, previousStage, artifact); docErr != nil {
			log.Printf("docs promotion failed for %s: %v", previousStage, docErr)
		} else {
			// Remove the artifact from .claudine now that it lives in docs
			aiFactoryPath := filepath.Join(project.HostDir, ".claudine", string(previousStage), string(previousStage)+".md")
			if rmErr := os.Remove(aiFactoryPath); rmErr != nil {
				log.Printf("failed to remove .claudine artifact %s: %v", aiFactoryPath, rmErr)
			}
		}
	}

	// Git commit the promoted artifact and the removal from .claudine
	commitMsg := fmt.Sprintf("approve(%s): promote artifact to docs", previousStage)
	if err := h.git.AddAllAndCommit(project.HostDir, commitMsg); err != nil {
		log.Printf("git commit failed for %s: %v", previousStage, err)
	}

	// Generate summary when transitioning to complete
	if nextStage == model.StageComplete {
		h.startSummaryRun(project)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(approveResponse{
		PreviousStage: string(previousStage),
		CurrentStage:  string(nextStage),
	})
}

// startSummaryRun creates a managed run for summary generation and kicks it off in a goroutine.
func (h *PipelineHandler) startSummaryRun(project *model.Project) {
	run := h.runs.Start(project.ID, "complete", "summary")
	if run == nil {
		return // already running
	}

	go func() {
		defer run.Finish(h.runs)

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

		events, err := agent.StreamSummary(run.Context(), artifacts, project.Name, project.Version)
		if err != nil {
			run.Emit(agent.StreamEvent{Type: "error", Content: err.Error()})
			return
		}

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

		summary := strings.TrimSpace(fullText.String())
		if summary == "" {
			run.Emit(agent.StreamEvent{Type: "error", Content: "summary generation produced no output"})
			return
		}

		summaryPath := filepath.Join(project.HostDir, ".claudine", "summary.md")
		if err := os.WriteFile(summaryPath, []byte(summary), 0644); err != nil {
			log.Printf("failed to write summary: %v", err)
			run.Emit(agent.StreamEvent{Type: "error", Content: "failed to write summary"})
			return
		}

		project.SummaryReady = true
		project.SummaryTokens = tokens
		project.AddStageTokens(model.StageComplete, tokens)
		project.UpdatedAt = time.Now()
		if err := h.registry.Update(project); err != nil {
			log.Printf("failed to update registry after summary: %v", err)
		}
		if err := h.projectRepo.Save(project.HostDir, project); err != nil {
			log.Printf("failed to save project after summary: %v", err)
		}

		commitMsg := fmt.Sprintf("complete(v%s): iteration summary", project.Version)
		if err := h.git.AddAndCommit(project.HostDir, []string{".claudine/summary.md"}, commitMsg); err != nil {
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
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
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
	summaryPath := filepath.Join(project.HostDir, ".claudine", "summary.md")
	b, err := os.ReadFile(summaryPath)
	if err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"content": "", "exists": false})
			return
		}
		http.Error(w, "failed to read summary: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"content": string(b), "exists": true})
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
