package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/paulette/backend/internal/agent"
	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/provider"
	"github.com/michelroberge/paulette/backend/internal/repository"
	fsrepo "github.com/michelroberge/paulette/backend/internal/repository/fs"
	"github.com/michelroberge/paulette/backend/internal/stream"
)

type SkillHandler struct {
	registry         repository.RegistryRepo
	artifactRepo     repository.ArtifactRepo
	activityRepo     repository.ActivityRepo
	skillRepo        *fsrepo.SkillRepo
	runs             *stream.Manager
	providerRegistry *provider.Registry
	stageConfig      *provider.StageConfigStore
}

func NewSkillHandler(
	registry repository.RegistryRepo,
	artifactRepo repository.ArtifactRepo,
	activityRepo repository.ActivityRepo,
	skillRepo *fsrepo.SkillRepo,
	runs *stream.Manager,
	providerRegistry *provider.Registry,
	stageConfig *provider.StageConfigStore,
) *SkillHandler {
	return &SkillHandler{
		registry:         registry,
		artifactRepo:     artifactRepo,
		activityRepo:     activityRepo,
		skillRepo:        skillRepo,
		runs:             runs,
		providerRegistry: providerRegistry,
		stageConfig:      stageConfig,
	}
}

// resolveProvider returns the Provider and model ID to use for the given stage,
// falling back to ClaudeCLI when no registry is configured.
func (h *SkillHandler) resolveProvider(projectID, hostDir string, stage model.StageName) (provider.Provider, string, error) {
	if h.providerRegistry != nil {
		return h.providerRegistry.ResolveForStage(projectID, stage, h.stageConfig, hostDir)
	}
	return provider.NewClaudeCLIProvider(), provider.FallbackModel(stage), nil
}

// Analyze triggers Claude-based skill analysis of the build plan (pre-phase).
func (h *SkillHandler) Analyze(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	// Check for existing active run — reconnect
	if existing := h.runs.Active(id, "build", "skills-analyze"); existing != nil {
		existing.StreamTo(w, r, 0)
		return
	}

	buildContent, _ := h.artifactRepo.ReadWithFallback(project.HostDir, project.Version, model.StageBuild)
	if buildContent == "" {
		http.Error(w, "no build artifact available", http.StatusBadRequest)
		return
	}
	archContent, _ := h.artifactRepo.ReadWithFallback(project.HostDir, project.Version, model.StageArchitecture)

	existingSkills, _ := h.skillRepo.List()

	run := h.runs.Start(project.ID, "build", "skills-analyze")
	if run == nil {
		http.Error(w, "skill analysis already running", http.StatusConflict)
		return
	}
	writeActivity(h.activityRepo, project.HostDir, model.StageBuild, "skills-analyze")

	// Resolve the LLM provider for the Build stage before entering the goroutine.
	prov, modelID, provErr := h.resolveProvider(project.ID, project.HostDir, model.StageBuild)
	if provErr != nil {
		run.Emit(agent.StreamEvent{Type: "error", Content: provErr.Error()})
		clearActivity(h.activityRepo, project.HostDir, model.StageBuild)
		run.Finish(h.runs)
		run.StreamTo(w, r, 0)
		return
	}

	// Build system prompt and user message outside the goroutine.
	systemPrompt, userMsg := agent.BuildAnalyzeSkillsRequest(buildContent, archContent, existingSkills)

	go func() {
		defer run.Finish(h.runs)
		defer clearActivity(h.activityRepo, project.HostDir, model.StageBuild)

		runStart := time.Now()
		events, err := prov.Chat(run.Context(), provider.ChatRequest{
			Model:        modelID,
			SystemPrompt: systemPrompt,
			UserMessage:  userMsg,
			ProjectDir:   project.HostDir,
		})
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

		// Record session
		if tokens > 0 {
			project.AddStageTokens(model.StageBuild, tokens)
			h.registry.Update(project)
			recordSession(project.HostDir, model.StageBuild, model.SessionSkillAnalyze, project.Iteration, runStart, tokens)
		}

		// Extract and save suggestions
		suggestions, ok := agent.ExtractSkillSuggestions(fullText.String())
		if ok && len(suggestions) > 0 {
			fsrepo.WriteSkillSuggestions(project.HostDir, &model.SkillSuggestions{Suggestions: suggestions})
		}
	}()

	run.StreamTo(w, r, 0)
}

// GetSuggestions returns the last set of skill suggestions for a project.
func (h *SkillHandler) GetSuggestions(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	suggestions, err := fsrepo.ReadSkillSuggestions(project.HostDir)
	if err != nil {
		http.Error(w, "failed to read suggestions: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(suggestions)
}

// GetObserved returns observer-detected skill suggestions.
func (h *SkillHandler) GetObserved(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	observed, err := fsrepo.ReadObservedSkills(project.HostDir)
	if err != nil {
		http.Error(w, "failed to read observed skills: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(observed)
}

type approveSkillsRequest struct {
	Indices []int `json:"indices"` // indices of suggestions to approve
}

// ApproveSuggestions creates skills from selected suggestions.
func (h *SkillHandler) ApproveSuggestions(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	var req approveSkillsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Load pre-phase suggestions
	suggestions, err := fsrepo.ReadSkillSuggestions(project.HostDir)
	if err != nil {
		http.Error(w, "failed to read suggestions", http.StatusInternalServerError)
		return
	}

	// Also check observed suggestions
	observed, _ := fsrepo.ReadObservedSkills(project.HostDir)

	// Combine both sources
	all := append(suggestions.Suggestions, observed.Suggestions...)

	var created []model.Skill
	for _, idx := range req.Indices {
		if idx < 0 || idx >= len(all) {
			continue
		}
		sg := all[idx]
		if sg.Approved {
			continue
		}

		skill := &model.Skill{
			ID:          fsrepo.Slugify(sg.Name),
			Name:        sg.Name,
			Description: sg.Description,
			Category:    sg.Category,
			Tags:        sg.Tags,
			Parameters:  sg.Parameters,
			CreatedBy:   project.Name,
		}
		if err := h.skillRepo.Create(skill, sg.PromptTemplate); err != nil {
			continue
		}
		created = append(created, *skill)

		// Mark as approved in the source
		if idx < len(suggestions.Suggestions) {
			suggestions.Suggestions[idx].Approved = true
		} else {
			observed.Suggestions[idx-len(suggestions.Suggestions)].Approved = true
		}
	}

	// Save updated approval states
	fsrepo.WriteSkillSuggestions(project.HostDir, suggestions)
	fsrepo.WriteObservedSkills(project.HostDir, observed)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"created": created,
		"count":   len(created),
	})
}

// ListAll returns all skills in the library (project-independent).
func (h *SkillHandler) ListAll(w http.ResponseWriter, r *http.Request) {
	skills, err := h.skillRepo.List()
	if err != nil {
		http.Error(w, "failed to list skills: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if skills == nil {
		skills = []model.Skill{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(skills)
}

// GetSkill returns a specific skill with its prompt template.
func (h *SkillHandler) GetSkill(w http.ResponseWriter, r *http.Request) {
	skillID := chi.URLParam(r, "skillId")
	skill, prompt, err := h.skillRepo.Get(skillID)
	if err != nil {
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"skill":          skill,
		"promptTemplate": prompt,
	})
}
