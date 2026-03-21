package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/ai-app-factory/backend/internal/agent"
	"github.com/michelroberge/ai-app-factory/backend/internal/model"
	"github.com/michelroberge/ai-app-factory/backend/internal/pipeline"
	"github.com/michelroberge/ai-app-factory/backend/internal/repository"
)

type PipelineHandler struct {
	registry     repository.RegistryRepo
	projectRepo  repository.ProjectRepo
	artifactRepo repository.ArtifactRepo
}

func NewPipelineHandler(registry repository.RegistryRepo, projectRepo repository.ProjectRepo, artifactRepo repository.ArtifactRepo) *PipelineHandler {
	return &PipelineHandler{
		registry:     registry,
		projectRepo:  projectRepo,
		artifactRepo: artifactRepo,
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

	// Check artifact exists for current stage
	exists, err := h.artifactRepo.Exists(project.HostDir, project.CurrentStage)
	if err != nil {
		http.Error(w, "failed to check artifact: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "cannot approve: no artifact for current stage", http.StatusBadRequest)
		return
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

	// Update both registry and project file
	if err := h.registry.Update(project); err != nil {
		http.Error(w, "failed to update registry: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.projectRepo.Save(project.HostDir, project); err != nil {
		http.Error(w, "failed to save project: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Git commit the approved artifact
	if artifactPath, ok := pipeline.ArtifactPaths[previousStage]; ok {
		gitAdd := exec.Command("git", "add", artifactPath)
		gitAdd.Dir = project.HostDir
		if err := gitAdd.Run(); err != nil {
			log.Printf("git add failed for %s: %v", artifactPath, err)
		} else {
			commitMsg := fmt.Sprintf("approve(%s): artifact", previousStage)
			gitCommit := exec.Command("git", "commit", "-m", commitMsg)
			gitCommit.Dir = project.HostDir
			if err := gitCommit.Run(); err != nil {
				log.Printf("git commit failed for %s: %v", artifactPath, err)
			}
		}
	}

	// Generate summary when transitioning to complete
	if nextStage == model.StageComplete {
		go func() {
			artifacts := make(map[model.StageName]string)
			for _, s := range pipeline.StageOrder {
				if s == model.StageComplete {
					break
				}
				content, _ := h.artifactRepo.Read(project.HostDir, s)
				if content != "" {
					artifacts[s] = content
				}
			}
			summary, err := agent.GenerateSummary(r.Context(), artifacts, project.Name, project.Version)
			if err != nil {
				log.Printf("summary generation failed: %v", err)
				return
			}
			summaryPath := filepath.Join(project.HostDir, ".ai-factory", "summary.md")
			if err := os.WriteFile(summaryPath, []byte(summary), 0644); err != nil {
				log.Printf("failed to write summary: %v", err)
				return
			}
			// Git commit the summary
			gitAdd := exec.Command("git", "add", ".ai-factory/summary.md")
			gitAdd.Dir = project.HostDir
			if err := gitAdd.Run(); err != nil {
				log.Printf("git add summary failed: %v", err)
			} else {
				gitCommit := exec.Command("git", "commit", "-m", fmt.Sprintf("complete(v%s): iteration summary", project.Version))
				gitCommit.Dir = project.HostDir
				if err := gitCommit.Run(); err != nil {
					log.Printf("git commit summary failed: %v", err)
				}
			}
		}()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(approveResponse{
		PreviousStage: string(previousStage),
		CurrentStage:  string(nextStage),
	})
}
