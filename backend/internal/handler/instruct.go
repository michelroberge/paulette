package handler

import (
	"context"
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

// InstructHandler handles "further instructions" planning and application.
type InstructHandler struct {
	registry         repository.RegistryRepo
	artifactRepo     repository.ArtifactRepo
	activityRepo     repository.ActivityRepo
	runs             *stream.Manager
	providerRegistry *provider.Registry
	stageConfig      *provider.StageConfigStore
}

func NewInstructHandler(registry repository.RegistryRepo, artifactRepo repository.ArtifactRepo, activityRepo repository.ActivityRepo, runs *stream.Manager) *InstructHandler {
	return &InstructHandler{
		registry:     registry,
		artifactRepo: artifactRepo,
		activityRepo: activityRepo,
		runs:         runs,
	}
}

// SetProviderRegistry wires in the provider registry and stage config.
func (h *InstructHandler) SetProviderRegistry(reg *provider.Registry, sc *provider.StageConfigStore) {
	h.providerRegistry = reg
	h.stageConfig = sc
}

// resolveProviderOp resolves a provider for an instruct operation.
func (h *InstructHandler) resolveProviderOp(projectID, hostDir string) (provider.Provider, string, *provider.StageAssignment, error) {
	if h.providerRegistry != nil {
		return h.providerRegistry.ResolveForStageOperation(projectID, model.StageBuild, provider.OperationKey("build.instruct"), h.stageConfig, hostDir)
	}
	return provider.NewClaudeCLIProvider(), provider.FallbackModel(model.StageBuild), nil, nil
}

type instructRequest struct {
	Message string `json:"message"`
}

// InstructionPlan is the structured proposal returned by Claude's planning pass.
// It is also what the frontend sends back when applying approved items.
type InstructionPlan struct {
	Reasoning          string           `json:"reasoning"`
	BuildPlanChanges   string           `json:"buildPlanChanges,omitempty"`
	ArchitectureChanges string          `json:"architectureChanges,omitempty"`
	NewBeads           []InstructBead   `json:"newBeads,omitempty"`
	UpdatedBeads       []InstructUpdate `json:"updatedBeads,omitempty"`
}

type InstructBead struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	EpicID      string   `json:"epicId,omitempty"`
	Deps        []string `json:"deps,omitempty"`
	TargetFiles []string `json:"targetFiles,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Priority    int      `json:"priority"`
}

type InstructUpdate struct {
	ID          string  `json:"id"`
	Description *string `json:"description,omitempty"`
	Title       *string `json:"title,omitempty"`
}

// Plan streams a planning proposal based on the user's instruction.
func (h *InstructHandler) Plan(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	var req instructRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Message == "" {
		http.Error(w, "invalid request body: message required", http.StatusBadRequest)
		return
	}

	// Check for existing active run
	if existing := h.runs.Active(id, "build", "instruct"); existing != nil {
		existing.StreamTo(w, r, 0)
		return
	}

	run := h.runs.Start(id, "build", "instruct")
	if run == nil {
		http.Error(w, "instruction planning already running", http.StatusConflict)
		return
	}

	// Load context: build plan, architecture, current bead graph
	buildContent, _ := h.artifactRepo.ReadWithFallback(project.DataDir, project.HostDir, project.Version, model.StageBuild)
	archContent, _ := h.artifactRepo.ReadWithFallback(project.DataDir, project.HostDir, project.Version, model.StageArchitecture)

	graph, _ := fsrepo.ReadBdBeadGraph(r.Context(), project.HostDir)
	var graphJSON string
	if graph != nil {
		b, _ := json.MarshalIndent(graph.Beads, "", "  ")
		graphJSON = string(b)
	}

	systemPrompt := buildInstructSystemPrompt(buildContent, archContent, graphJSON)

	prov, modelID, sa, provErr := h.resolveProviderOp(id, project.HostDir)
	if provErr != nil {
		run.Finish(h.runs)
		http.Error(w, "failed to resolve provider: "+provErr.Error(), http.StatusInternalServerError)
		return
	}
	var temp *float64
	var numCtx *int
	var streamFlag *bool
	if sa != nil {
		temp, numCtx, streamFlag = sa.Fields()
	}

	go func() {
		defer run.Finish(h.runs)

		ctx := run.Context()
		events, err := prov.Chat(ctx, provider.ChatRequest{
			Model:        modelID,
			SystemPrompt: systemPrompt,
			UserMessage:  req.Message,
			ProjectDir:   project.HostDir,
			Stage:        string(model.StageBuild),
			Temperature:  temp,
			NumCtx:       numCtx,
			Stream:       streamFlag,
		})
		if err != nil {
			run.Emit(agent.StreamEvent{Type: "error", Content: "failed to start planning: " + err.Error()})
			return
		}

		var fullContent strings.Builder
		for ev := range events {
			if ev.Type == "chunk" {
				fullContent.WriteString(ev.Content)
				run.Emit(ev)
			} else if ev.Type == "done" {
				fullContent.WriteString(ev.Content)
				// Try to extract structured plan from artifact tags
				if plan, ok := extractInstructionPlan(fullContent.String()); ok {
					planJSON, _ := json.Marshal(plan)
					run.Emit(agent.StreamEvent{Type: "artifact", Content: string(planJSON)})
				}
				run.Emit(agent.StreamEvent{Type: "done", Content: ev.Content})
			} else {
				run.Emit(ev)
			}
		}
	}()

	run.StreamTo(w, r, 0)
}

type applyInstructRequest struct {
	Plan InstructionPlan `json:"plan"`
}

// Apply applies an approved InstructionPlan: updates build plan, architecture, and creates/updates beads.
func (h *InstructHandler) Apply(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	var req applyInstructRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	plan := req.Plan
	ctx := r.Context()

	// Apply build plan changes
	if strings.TrimSpace(plan.BuildPlanChanges) != "" {
		existing, _ := h.artifactRepo.ReadWithFallback(project.DataDir, project.HostDir, project.Version, model.StageBuild)
		updated := appendInstructionSection(existing, plan.BuildPlanChanges)
		if wErr := h.artifactRepo.Write(project.DataDir, model.StageBuild, updated); wErr != nil {
			http.Error(w, "failed to update build plan: "+wErr.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Apply architecture changes
	if strings.TrimSpace(plan.ArchitectureChanges) != "" {
		existing, _ := h.artifactRepo.ReadWithFallback(project.DataDir, project.HostDir, project.Version, model.StageArchitecture)
		updated := appendInstructionSection(existing, plan.ArchitectureChanges)
		if wErr := h.artifactRepo.Write(project.DataDir, model.StageArchitecture, updated); wErr != nil {
			http.Error(w, "failed to update architecture: "+wErr.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Create new beads
	type createdBead struct {
		RequestedTitle string `json:"requestedTitle"`
		ID             string `json:"id"`
	}
	var created []createdBead

	for _, nb := range plan.NewBeads {
		beadType := nb.Type
		if beadType == "" {
			beadType = "task"
		}

		beadID, createErr := bdCreate(ctx, project.HostDir, nb.Title, nb.Description, beadType, nb.Priority)
		if createErr != nil {
			http.Error(w, "failed to create bead '"+nb.Title+"': "+createErr.Error(), http.StatusInternalServerError)
			return
		}

		// Add deps
		for _, dep := range nb.Deps {
			bdDepAdd(ctx, project.HostDir, beadID, dep)
		}

		// Persist extended metadata to bd notes
		fsrepo.WriteBeadMeta(ctx, project.HostDir, beadID, &fsrepo.BeadMeta{ //nolint:errcheck
			EpicID:      nb.EpicID,
			TargetFiles: nb.TargetFiles,
			Tags:        nb.Tags,
		})

		created = append(created, createdBead{RequestedTitle: nb.Title, ID: beadID})
	}

	// Update existing beads
	for _, ub := range plan.UpdatedBeads {
		if ub.Description != nil {
			runBd(ctx, project.HostDir, "update", ub.ID, "--description", *ub.Description)
		}
		if ub.Title != nil {
			runBd(ctx, project.HostDir, "update", ub.ID, "--title", *ub.Title)
		}
	}

	// Return updated graph
	graph, _ := fsrepo.ReadBdBeadGraph(ctx, project.HostDir)

	resp := struct {
		Created []createdBead     `json:"created"`
		Graph   *model.BeadGraph  `json:"graph"`
	}{
		Created: created,
		Graph:   graph,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func buildInstructSystemPrompt(buildContent, archContent, graphJSON string) string {
	var sb strings.Builder
	sb.WriteString(`You are a software architect assistant helping to refine a project build plan.

The user will give you a natural-language instruction about additional work that needs to be done.
Your job is to analyze the instruction and produce a structured plan.

IMPORTANT: You MUST respond with your reasoning first (as plain text), then wrap your structured plan
in an <artifact type="instruction-plan"> tag containing valid JSON.

The JSON must conform to this schema:
{
  "reasoning": "brief explanation of what you're proposing and why",
  "buildPlanChanges": "markdown text to APPEND to the build plan (null if no changes needed)",
  "architectureChanges": "markdown text to APPEND to the architecture doc (null if no changes needed)",
  "newBeads": [
    {
      "title": "...",
      "description": "...",
      "type": "task|feature|epic",
      "epicId": "existing-epic-id or empty",
      "deps": ["existing-bead-id", ...],
      "targetFiles": ["relative/path/to/file.ts", ...],
      "tags": ["frontend", "backend", "api", ...],
      "priority": 2
    }
  ],
  "updatedBeads": [
    {
      "id": "existing-bead-id",
      "description": "new description (null if not changing)",
      "title": "new title (null if not changing)"
    }
  ]
}

Rules:
- Only include buildPlanChanges / architectureChanges if the instruction truly requires updating those docs
- For new beads, use deps to reference existing bead IDs from the current bead graph
- Use the existing epic IDs from the bead graph when a new task belongs in an existing epic
- Tags should be one or more of: frontend, backend, api, database, styling, config, testing, devops
- targetFiles should be relative paths from the project root
`)

	if buildContent != "" {
		sb.WriteString("\n## Current Build Plan\n")
		sb.WriteString(buildContent)
		sb.WriteString("\n")
	}
	if archContent != "" {
		sb.WriteString("\n## Current Architecture\n")
		sb.WriteString(archContent)
		sb.WriteString("\n")
	}
	if graphJSON != "" {
		sb.WriteString("\n## Current Bead Graph\n```json\n")
		sb.WriteString(graphJSON)
		sb.WriteString("\n```\n")
	}

	return sb.String()
}

// extractInstructionPlan parses an <artifact type="instruction-plan"> block from text.
func extractInstructionPlan(text string) (InstructionPlan, bool) {
	const openTag = `<artifact type="instruction-plan">`
	const closeTag = `</artifact>`

	start := strings.Index(text, openTag)
	if start < 0 {
		return InstructionPlan{}, false
	}
	jsonStart := start + len(openTag)
	end := strings.Index(text[jsonStart:], closeTag)
	if end < 0 {
		return InstructionPlan{}, false
	}

	raw := strings.TrimSpace(text[jsonStart : jsonStart+end])
	var plan InstructionPlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return InstructionPlan{}, false
	}
	return plan, true
}

// appendInstructionSection appends new content to an existing document,
// separated by a timestamped header.
func appendInstructionSection(existing, additions string) string {
	additions = strings.TrimSpace(additions)
	if additions == "" {
		return existing
	}
	if existing == "" {
		return additions
	}
	header := fmt.Sprintf("\n\n---\n*Added by Further Instructions — %s*\n\n", time.Now().Format("2006-01-02"))
	return strings.TrimSpace(existing) + header + additions
}

// bdUpdate runs bd update for a field change (title or description).
func bdUpdate(ctx context.Context, hostDir, id string, args ...string) error {
	all := append([]string{"update", id}, args...)
	_, err := runBd(ctx, hostDir, all...)
	return err
}
