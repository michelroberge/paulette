package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/paulette/backend/internal/agent"
	"github.com/michelroberge/paulette/backend/internal/git"
	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/pipeline"
	"github.com/michelroberge/paulette/backend/internal/repository"
	fsrepo "github.com/michelroberge/paulette/backend/internal/repository/fs"
	"github.com/michelroberge/paulette/backend/internal/stream"
)

// beadGraphMu serializes writes to beads-graph.json per project host directory.
var beadGraphMu sync.Map // key: hostDir string, value: *sync.Mutex

func beadGraphLock(hostDir string) *sync.Mutex {
	mu, _ := beadGraphMu.LoadOrStore(hostDir, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

type BeadHandler struct {
	registry     repository.RegistryRepo
	projectRepo  repository.ProjectRepo
	artifactRepo repository.ArtifactRepo
	activityRepo repository.ActivityRepo
	runs         *stream.Manager
	skillRepo    *fsrepo.SkillRepo
	gitSvc       *git.Service
}

func NewBeadHandler(registry repository.RegistryRepo, projectRepo repository.ProjectRepo, artifactRepo repository.ArtifactRepo, activityRepo repository.ActivityRepo, runs *stream.Manager, skillRepo *fsrepo.SkillRepo) *BeadHandler {
	return &BeadHandler{registry: registry, projectRepo: projectRepo, artifactRepo: artifactRepo, activityRepo: activityRepo, runs: runs, skillRepo: skillRepo, gitSvc: git.NewService()}
}

// GetGraph returns the current bead graph JSON for a project,
// reconciled with live bd status so the UI always reflects reality.
func (h *BeadHandler) GetGraph(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	graph, err := fsrepo.ReadBeadGraph(project.HostDir)
	if err != nil {
		http.Error(w, "failed to read bead graph: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Reconcile graph with live bd status
	if len(graph.Beads) > 0 {
		// Reset stale in_progress beads if no execution is currently active (e.g. after a restart)
		if h.runs.Active(project.ID, "build", "beads-execute") == nil {
			bdResetStale(r.Context(), project.HostDir)
		}
		if changed := reconcileGraphWithBd(r.Context(), project.HostDir, graph); changed {
			fsrepo.WriteBeadGraph(project.HostDir, graph)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(graph)
}

// reconcileGraphWithBd queries `bd list --json` for all beads and updates
// the graph in-place so statuses match reality. Returns true if anything changed.
func reconcileGraphWithBd(ctx context.Context, hostDir string, graph *model.BeadGraph) bool {
	out, err := runBd(ctx, hostDir, "list", "--json")
	if err != nil || strings.TrimSpace(out) == "" || strings.TrimSpace(out) == "[]" {
		return false
	}

	var live []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &live); err != nil {
		return false
	}

	liveStatus := make(map[string]model.BeadStatus, len(live))
	for _, b := range live {
		liveStatus[b.ID] = model.BeadStatus(b.Status)
	}

	changed := false
	for i := range graph.Beads {
		if s, ok := liveStatus[graph.Beads[i].ID]; ok && s != graph.Beads[i].Status {
			graph.Beads[i].Status = s
			changed = true
		}
	}
	return changed
}

// Watch opens a long-lived SSE connection that polls bd every 10s
// and pushes bead_update events when statuses change.
func (h *BeadHandler) Watch(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()

	ctx := r.Context()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Track last-known status per bead to detect changes
	prev := map[string]model.BeadStatus{}

	// Seed from the current graph file
	if graph, err := fsrepo.ReadBeadGraph(project.HostDir); err == nil {
		for _, b := range graph.Beads {
			prev[b.ID] = b.Status
		}
	}

	emitJSON := func(event agent.StreamEvent) {
		data, _ := json.Marshal(event)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	// Send an initial ping so the client knows the connection is live
	emitJSON(agent.StreamEvent{Type: "ping", Content: "watching"})

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			out, err := runBd(ctx, project.HostDir, "list", "--json")
			if err != nil || strings.TrimSpace(out) == "" || strings.TrimSpace(out) == "[]" {
				continue
			}

			var live []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			}
			if json.Unmarshal([]byte(out), &live) != nil {
				continue
			}

			mu := beadGraphLock(project.HostDir)
			var updates []model.Bead
			for _, b := range live {
				s := model.BeadStatus(b.Status)
				if old, ok := prev[b.ID]; ok && old != s {
					prev[b.ID] = s
					updates = append(updates, model.Bead{ID: b.ID, Status: s})
				} else if !ok {
					prev[b.ID] = s
				}
			}

			if len(updates) > 0 {
				// Persist to graph file
				mu.Lock()
				graph, err := fsrepo.ReadBeadGraph(project.HostDir)
				if err == nil {
					for i := range graph.Beads {
						for _, u := range updates {
							if graph.Beads[i].ID == u.ID {
								graph.Beads[i].Status = u.Status
							}
						}
					}
					fsrepo.WriteBeadGraph(project.HostDir, graph)
				}
				mu.Unlock()

				// Stream each change
				for _, u := range updates {
					beadJSON, _ := json.Marshal(u)
					emitJSON(agent.StreamEvent{Type: "bead_update", Content: string(beadJSON)})
				}
			}
		}
	}
}

// Generate parses build.md and creates beads via the bd CLI, streaming progress via SSE.
func (h *BeadHandler) Generate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	// Check for existing active run — reconnect to it
	if existing := h.runs.Active(id, "build", "beads-generate"); existing != nil {
		existing.StreamTo(w, r, 0)
		return
	}

	run, err := h.StartGenerateRun(project)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if run == nil {
		http.Error(w, "bead generation is already running", http.StatusConflict)
		return
	}

	run.StreamTo(w, r, 0)
}

// StartGenerateRun starts the beads generation run without HTTP plumbing.
// Returns (nil, nil) on race condition.
func (h *BeadHandler) StartGenerateRun(project *model.Project) (*stream.Run, error) {
	buildContent, _ := h.artifactRepo.ReadWithFallback(project.HostDir, project.Version, model.StageBuild)
	if buildContent == "" {
		return nil, fmt.Errorf("no build artifact — complete the Build stage first")
	}

	archContent, _ := h.artifactRepo.ReadWithFallback(project.HostDir, project.Version, model.StageArchitecture)

	run := h.runs.Start(project.ID, "build", "beads-generate")
	if run == nil {
		return nil, nil // race: already started
	}
	writeActivity(h.activityRepo, project.HostDir, model.StageBuild, "beads-generate")

	go func() {
		defer run.Finish(h.runs)
		defer clearActivity(h.activityRepo, project.HostDir, model.StageBuild)

		var generateTokens int
		runStart := time.Now()
		defer func() {
			if generateTokens > 0 {
				project.AddStageTokens(model.StageBuild, generateTokens)
				h.registry.Update(project)
				recordSession(project.HostDir, model.StageBuild, model.SessionBeadGenerate, project.Iteration, runStart, generateTokens)
			}
		}()

		ctx := run.Context()

		// Step 1: Parse build.md with Claude (architecture provided for scoping inference)
		run.Emit(agent.StreamEvent{Type: "log", Content: "Parsing build plan..."})

		events, err := agent.ParseBuildPlan(ctx, buildContent, archContent)
		if err != nil {
			run.Emit(agent.StreamEvent{Type: "error", Content: "Failed to start parser: " + err.Error()})
			return
		}

		var fullResponse string
		for event := range events {
			if event.Type == "plan_limit" {
				run.Emit(event)
				return
			} else if event.Type == "done" {
				fullResponse = event.Content
			} else if event.Type == "tokens" {
				var n int
				fmt.Sscanf(event.Content, "%d", &n)
				generateTokens += n
			} else if event.Type == "chunk" {
				run.Emit(event)
			}
		}

		jsonBytes, ok2 := agent.ExtractBeadJSON(fullResponse)
		if !ok2 {
			run.Emit(agent.StreamEvent{Type: "error", Content: "Failed to extract JSON from parser response"})
			return
		}

		var plan model.ParsedBuildPlan
		if err := json.Unmarshal(jsonBytes, &plan); err != nil {
			run.Emit(agent.StreamEvent{Type: "error", Content: "Failed to parse JSON: " + err.Error()})
			return
		}

		// Step 2: Ensure bd is initialized
		run.Emit(agent.StreamEvent{Type: "log", Content: "Checking bd status..."})
		if err := ensureBdInitRun(ctx, project.HostDir, run); err != nil {
			run.Emit(agent.StreamEvent{Type: "error", Content: "bd init failed: " + err.Error()})
			return
		}

		// Step 3: Create epics and tasks
		graph := &model.BeadGraph{
			GeneratedAt: time.Now(),
			ProjectID:   project.ID,
			Beads:       []model.Bead{},
		}

		// title -> bead ID map for dep resolution
		titleToID := map[string]string{}

		for _, epic := range plan.Epics {
			if ctx.Err() != nil {
				return
			}

			epicID, err := bdCreate(ctx, project.HostDir, epic.Title, epic.Description, "epic", 2)
			if err != nil {
				run.Emit(agent.StreamEvent{Type: "error", Content: fmt.Sprintf("Failed to create epic %q: %v", epic.Title, err)})
				continue
			}

			epicBead := model.Bead{
				ID:          epicID,
				Title:       epic.Title,
				Description: epic.Description,
				Type:        model.BeadTypeEpic,
				Status:      model.BeadStatusOpen,
				Priority:    2,
				Deps:        []string{},
			}
			titleToID[epic.Title] = epicID
			graph.Beads = append(graph.Beads, epicBead)

			beadJSON, _ := json.Marshal(epicBead)
			run.Emit(agent.StreamEvent{Type: "bead_created", Content: string(beadJSON)})

			fsrepo.WriteBeadGraph(project.HostDir, graph)

			for _, task := range epic.Tasks {
				if ctx.Err() != nil {
					return
				}

				taskID, err := bdCreate(ctx, project.HostDir, task.Title, task.Description, "task", task.Priority)
				if err != nil {
					run.Emit(agent.StreamEvent{Type: "error", Content: fmt.Sprintf("Failed to create task %q: %v", task.Title, err)})
					continue
				}

				if task.JourneyRefs == nil {
					task.JourneyRefs = []string{}
				}
				if task.ArchRefs == nil {
					task.ArchRefs = []string{}
				}
				taskBead := model.Bead{
					ID:          taskID,
					Title:       task.Title,
					Description: task.Description,
					Type:        model.BeadTypeTask,
					Status:      model.BeadStatusOpen,
					Priority:    task.Priority,
					EpicID:      epicID,
					Deps:        []string{},
					Tags:        task.Tags,
					TargetFiles: task.TargetFiles,
					JourneyRefs: task.JourneyRefs,
					ArchRefs:    task.ArchRefs,
				}
				titleToID[task.Title] = taskID
				graph.Beads = append(graph.Beads, taskBead)

				beadJSON, _ := json.Marshal(taskBead)
				run.Emit(agent.StreamEvent{Type: "bead_created", Content: string(beadJSON)})

				fsrepo.WriteBeadGraph(project.HostDir, graph)
			}
		}

		// Step 4: Set up dependencies
		run.Emit(agent.StreamEvent{Type: "log", Content: "Setting up dependencies..."})

		for _, epic := range plan.Epics {
			for _, task := range epic.Tasks {
				taskID, ok := titleToID[task.Title]
				if !ok {
					continue
				}
				for _, depTitle := range task.DepsOn {
					depID, ok := titleToID[depTitle]
					if !ok {
						continue
					}
					bdDepAdd(ctx, project.HostDir, taskID, depID)

					// Update in-memory graph
					for i := range graph.Beads {
						if graph.Beads[i].ID == taskID {
							graph.Beads[i].Deps = append(graph.Beads[i].Deps, depID)
							break
						}
					}
				}
			}
		}

		fsrepo.WriteBeadGraph(project.HostDir, graph)

		// Emit bead_update for every bead that received deps, so the frontend graph is current
		for _, bead := range graph.Beads {
			if len(bead.Deps) > 0 || bead.EpicID != "" {
				beadJSON, _ := json.Marshal(bead)
				run.Emit(agent.StreamEvent{Type: "bead_update", Content: string(beadJSON)})
			}
		}

		summary := fmt.Sprintf("Generated %d beads (%d epics)", len(graph.Beads), len(plan.Epics))
		run.Emit(agent.StreamEvent{Type: "done", Content: summary})
	}()

	return run, nil
}

type executeBeadsRequest struct {
	MaxParallel int `json:"maxParallel"`
}

// Execute runs parallel Claude agents to implement ready beads, streaming status via SSE.
func (h *BeadHandler) Execute(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	// Check for existing active run — reconnect to it
	if existing := h.runs.Active(id, "build", "beads-execute"); existing != nil {
		existing.StreamTo(w, r, 0)
		return
	}

	var req executeBeadsRequest
	req.MaxParallel = 2
	json.NewDecoder(r.Body).Decode(&req)
	if req.MaxParallel < 1 {
		req.MaxParallel = 1
	}
	if req.MaxParallel > 10 {
		req.MaxParallel = 10
	}

	run, err := h.StartExecuteRun(project, req.MaxParallel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if run == nil {
		http.Error(w, "bead execution is already running", http.StatusConflict)
		return
	}

	run.StreamTo(w, r, 0)
}

// StartExecuteRun starts the beads execution run without HTTP plumbing.
// Returns (nil, nil) on race condition.
func (h *BeadHandler) StartExecuteRun(project *model.Project, maxParallel int) (*stream.Run, error) {
	if maxParallel < 1 {
		maxParallel = 2
	}
	if maxParallel > 10 {
		maxParallel = 10
	}

	run := h.runs.Start(project.ID, "build", "beads-execute")
	if run == nil {
		return nil, nil // race: already started
	}
	writeActivity(h.activityRepo, project.HostDir, model.StageBuild, "beads-execute")

	go func() {
		defer run.Finish(h.runs)
		defer clearActivity(h.activityRepo, project.HostDir, model.StageBuild)

		var totalBuildTokens atomic.Int64
		runStart := time.Now()
		defer func() {
			if n := int(totalBuildTokens.Load()); n > 0 {
				project.AddStageTokens(model.StageBuild, n)
				h.registry.Update(project)
				recordSession(project.HostDir, model.StageBuild, model.SessionBeadExecute, project.Iteration, runStart, n)
			}
		}()

		ctx := run.Context()

		// Reset any stale in_progress beads from a previous interrupted run.
		if err := bdResetStale(ctx, project.HostDir); err != nil {
			run.Emit(agent.StreamEvent{Type: "log", Content: "warn: reset stale: " + err.Error()})
		}

		// Load all artifacts for context injection
		artifacts := map[model.StageName]string{}
		for _, stage := range []model.StageName{model.StageVision, model.StageUX, model.StageArchitecture, model.StageBuild} {
			content, _ := h.artifactRepo.ReadWithFallback(project.HostDir, project.Version, stage)
			if content != "" {
				artifacts[stage] = content
			}
		}

		// Build enhancement context if this is an enhancement iteration
		var enhCtx *agent.EnhancementContext
		if project.EnhancementVision != "" {
			enhCtx = &agent.EnhancementContext{Vision: project.EnhancementVision}
			summaryPath := filepath.Join(project.HostDir, ".paulette", "summary.md")
			if data, err := os.ReadFile(summaryPath); err == nil {
				enhCtx.Summary = string(data)
			}
		}

		// Load skills from the global library for matching to beads
		type cachedSkill struct {
			skill  model.Skill
			prompt string
		}
		var skillCache []cachedSkill
		if allSkills, _ := h.skillRepo.List(); len(allSkills) > 0 {
			for _, s := range allSkills {
				_, promptText, err := h.skillRepo.Get(s.ID)
				if err != nil || promptText == "" {
					continue
				}
				skillCache = append(skillCache, cachedSkill{skill: s, prompt: promptText})
				// Copy to project for local agent access
				h.skillRepo.CopyToProject(project.HostDir, s.ID)
			}
		}

		// Set up skill observer to detect emergent patterns during execution
		observer := NewSkillObserver(project.HostDir, h.skillRepo, run, project, 5)
		defer observer.Flush()

		var wg sync.WaitGroup
		sem := make(chan struct{}, maxParallel)
		mu := beadGraphLock(project.HostDir)

		for {
			if ctx.Err() != nil {
				break
			}

			bead, err := bdReady(ctx, project.HostDir)
			if err != nil || bead == nil {
				break
			}

			// Enrich bead with scoping metadata from graph (bd CLI doesn't return these)
			enrichBeadFromGraph(project.HostDir, bead)

			sem <- struct{}{}

			// Capture git commit hash before execution starts, for diff view
			preCommit := h.gitSvc.CurrentHash(project.HostDir)

			if err := bdClaim(ctx, project.HostDir, bead.ID); err != nil {
				<-sem
				run.Emit(agent.StreamEvent{Type: "error", Content: fmt.Sprintf("[%s] claim failed: %v", bead.ID, err)})
				continue
			}
			wg.Add(1)
			b := *bead
			b.Status = model.BeadStatusInProgress
			b.PreExecutionCommit = preCommit
			updateGraphBeadFields(mu, project.HostDir, b.ID, func(stored *model.Bead) {
				stored.Status = model.BeadStatusInProgress
				stored.PreExecutionCommit = preCommit
			})
			beadJSON, _ := json.Marshal(b)
			run.Emit(agent.StreamEvent{Type: "bead_update", Content: string(beadJSON)})
			run.Emit(agent.StreamEvent{Type: "log", Content: fmt.Sprintf("[%s] Starting: %s", b.ID, b.Title)})

			go func(b model.Bead) {
				defer wg.Done()
				defer func() { <-sem }()

				const maxReviewIterations = 3
				currentBead := b

				// Match skills to this bead by tag overlap and inject via context
				beadCtx := ctx
				if len(skillCache) > 0 && len(b.Tags) > 0 {
					beadTags := make(map[string]bool, len(b.Tags))
					for _, t := range b.Tags {
						beadTags[t] = true
					}
					var matched []agent.MatchedSkill
					for _, cs := range skillCache {
						for _, st := range cs.skill.Tags {
							if beadTags[st] {
								matched = append(matched, agent.MatchedSkill{Name: cs.skill.Name, Prompt: cs.prompt})
								break
							}
						}
					}
					if len(matched) > 0 {
						beadCtx = agent.WithSkillContext(ctx, &agent.SkillContext{Skills: matched})
					}
				}

				// Execute initial bead
				agentEvents, err := agent.ExecuteBead(beadCtx, project.HostDir, currentBead, artifacts, enhCtx)
				if err != nil {
					run.Emit(agent.StreamEvent{Type: "error", Content: fmt.Sprintf("[%s] execute failed: %v", currentBead.ID, err)})
					return
				}
				var execContent string
				for ev := range agentEvents {
					if ev.Type == "plan_limit" {
						run.Emit(ev)
						return
					} else if ev.Type == "done" {
						execContent = ev.Content
					} else if ev.Type == "chunk" {
						run.Emit(agent.StreamEvent{Type: "log", Content: fmt.Sprintf("[%s] %s", currentBead.ID, ev.Content)})
					} else if ev.Type == "tokens" && ev.Tokens > 0 {
						currentBead.Tokens += ev.Tokens
						totalBuildTokens.Add(int64(ev.Tokens))
						beadJSON, _ := json.Marshal(currentBead)
						run.Emit(agent.StreamEvent{Type: "bead_update", Content: string(beadJSON)})
					}
				}
				if execContent != "" {
					if docErr := fsrepo.WriteBeadDoc(project.HostDir, project.Version, currentBead.ID, execContent); docErr != nil {
						run.Emit(agent.StreamEvent{Type: "log", Content: fmt.Sprintf("[%s] warn: docs mirror failed: %v", currentBead.ID, docErr)})
					}
				}

				// Ralph loop: devil's advocate reviews, up to maxReviewIterations times
				for iteration := 0; iteration < maxReviewIterations; iteration++ {
					// Signal: devil is reviewing
					currentBead.Status = model.BeadStatusReviewing
					updateGraphStatus(mu, project.HostDir, currentBead.ID, model.BeadStatusReviewing)
					beadJSON, _ := json.Marshal(currentBead)
					run.Emit(agent.StreamEvent{Type: "bead_update", Content: string(beadJSON)})
					run.Emit(agent.StreamEvent{Type: "log", Content: fmt.Sprintf("[%s] 😈 Devil's advocate reviewing (attempt %d/%d)...", currentBead.ID, iteration+1, maxReviewIterations)})

					siblings := siblingBeads(project.HostDir, currentBead)
					findings, reviewTokens, reviewErr := agent.ReviewBead(ctx, project.HostDir, currentBead, artifacts, siblings, enhCtx)
					if reviewTokens > 0 {
						currentBead.Tokens += reviewTokens
						totalBuildTokens.Add(int64(reviewTokens))
						beadJSON, _ := json.Marshal(currentBead)
						run.Emit(agent.StreamEvent{Type: "bead_update", Content: string(beadJSON)})
					}
					if reviewErr != nil {
						if errors.Is(reviewErr, agent.ErrPlanLimit) {
							run.Emit(agent.StreamEvent{Type: "plan_limit", Content: "Claude plan limit reached during review"})
							return
						}
						run.Emit(agent.StreamEvent{Type: "log", Content: fmt.Sprintf("[%s] review error: %v (closing anyway)", currentBead.ID, reviewErr)})
						findings = ""
					}

					isApproved := findings == ""

					if isApproved || iteration == maxReviewIterations-1 {
						if !isApproved {
							run.Emit(agent.StreamEvent{Type: "log", Content: fmt.Sprintf("[%s] ⚠️ Max review iterations reached, closing anyway", currentBead.ID)})
						}
						if err := bdClose(ctx, project.HostDir, currentBead.ID); err != nil {
							run.Emit(agent.StreamEvent{Type: "error", Content: fmt.Sprintf("[%s] close failed: %v", currentBead.ID, err)})
							return
						}
						currentBead.Status = model.BeadStatusClosed
						updateGraphStatus(mu, project.HostDir, currentBead.ID, model.BeadStatusClosed)
						beadJSON, _ = json.Marshal(currentBead)
						run.Emit(agent.StreamEvent{Type: "bead_update", Content: string(beadJSON)})
						run.Emit(agent.StreamEvent{Type: "log", Content: fmt.Sprintf("[%s] ✅ Done: %s", currentBead.ID, currentBead.Title)})
						observer.RecordBead(currentBead, currentBead.Title, currentBead.Description)
						return
					}

					// Issues found — close current bead, create and execute a correction bead
					run.Emit(agent.StreamEvent{Type: "log", Content: fmt.Sprintf("[%s] 🔧 Issues found, creating correction bead...", currentBead.ID)})
					if err := bdClose(ctx, project.HostDir, currentBead.ID); err != nil {
						run.Emit(agent.StreamEvent{Type: "error", Content: fmt.Sprintf("[%s] close failed: %v", currentBead.ID, err)})
						return
					}
					currentBead.Status = model.BeadStatusClosed
					updateGraphStatus(mu, project.HostDir, currentBead.ID, model.BeadStatusClosed)
					beadJSON, _ = json.Marshal(currentBead)
					run.Emit(agent.StreamEvent{Type: "bead_update", Content: string(beadJSON)})

					shortFindings := strings.TrimSpace(findings)
					if len(shortFindings) > 80 {
						shortFindings = shortFindings[:77] + "..."
					}
					corrTitle := fmt.Sprintf("Fix review findings: %s", shortFindings)
					corrDesc := fmt.Sprintf("Devil's advocate found issues with '%s':\n\n%s", currentBead.Title, findings)
					corrID, err := bdCreate(ctx, project.HostDir, corrTitle, corrDesc, string(model.BeadTypeTask), currentBead.Priority)
					if err != nil {
						run.Emit(agent.StreamEvent{Type: "error", Content: fmt.Sprintf("[%s] correction bead create failed: %v", currentBead.ID, err)})
						return
					}
					if err := bdClaim(ctx, project.HostDir, corrID); err != nil {
						run.Emit(agent.StreamEvent{Type: "error", Content: fmt.Sprintf("[%s] correction claim failed: %v", corrID, err)})
						return
					}

					corrBead := model.Bead{
						ID:          corrID,
						Title:       corrTitle,
						Description: corrDesc,
						Type:        model.BeadTypeTask,
						Status:      model.BeadStatusInProgress,
						Priority:    currentBead.Priority,
						Deps:        []string{},
						Tags:        currentBead.Tags,
						TargetFiles: currentBead.TargetFiles,
						JourneyRefs: currentBead.JourneyRefs,
						ArchRefs:    currentBead.ArchRefs,
					}
					updateGraphStatus(mu, project.HostDir, corrID, model.BeadStatusInProgress)
					beadJSON, _ = json.Marshal(corrBead)
					run.Emit(agent.StreamEvent{Type: "bead_created", Content: string(beadJSON)})
					run.Emit(agent.StreamEvent{Type: "bead_update", Content: string(beadJSON)})
					run.Emit(agent.StreamEvent{Type: "log", Content: fmt.Sprintf("[%s] Starting correction: %s", corrID, corrTitle)})

					corrEvents, err := agent.ExecuteBead(ctx, project.HostDir, corrBead, artifacts, enhCtx)
					if err != nil {
						run.Emit(agent.StreamEvent{Type: "error", Content: fmt.Sprintf("[%s] correction execute failed: %v", corrID, err)})
						return
					}
					var corrContent string
					for ev := range corrEvents {
						if ev.Type == "plan_limit" {
							run.Emit(ev)
							return
						} else if ev.Type == "done" {
							corrContent = ev.Content
						} else if ev.Type == "chunk" {
							run.Emit(agent.StreamEvent{Type: "log", Content: fmt.Sprintf("[%s] %s", corrID, ev.Content)})
						} else if ev.Type == "tokens" && ev.Tokens > 0 {
							corrBead.Tokens += ev.Tokens
							totalBuildTokens.Add(int64(ev.Tokens))
							beadJSON, _ := json.Marshal(corrBead)
							run.Emit(agent.StreamEvent{Type: "bead_update", Content: string(beadJSON)})
						}
					}
					if corrContent != "" {
						if docErr := fsrepo.WriteBeadDoc(project.HostDir, project.Version, corrID, corrContent); docErr != nil {
							run.Emit(agent.StreamEvent{Type: "log", Content: fmt.Sprintf("[%s] warn: docs mirror failed: %v", corrID, docErr)})
						}
					}
					currentBead = corrBead
				}
			}(b)
		}

		wg.Wait()

		// --- Validation Gate ---
		vcmds := pipeline.CommandsFromProject(project)
		if vcmds.HasCommands() {
			for attempt := 0; attempt < pipeline.MaxValidationRetries; attempt++ {
				run.Emit(agent.StreamEvent{Type: "validation_start", Content: fmt.Sprintf("Validation attempt %d/%d", attempt+1, pipeline.MaxValidationRetries)})

				results, allPassed := pipeline.RunValidation(ctx, project.HostDir, vcmds, func(ev agent.StreamEvent) {
					run.Emit(ev)
				})

				if allPassed {
					run.Emit(agent.StreamEvent{Type: "log", Content: "✅ All validation commands passed"})
					break
				}

				if attempt == pipeline.MaxValidationRetries-1 {
					run.Emit(agent.StreamEvent{Type: "error", Content: "❌ Validation failed after max retries"})
					break
				}

				// Create a fix bead with the error output
				fixDesc := pipeline.FormatFailureSummary(results)
				fixTitle := fmt.Sprintf("Fix validation failures (attempt %d)", attempt+1)

				fixID, err := bdCreate(ctx, project.HostDir, fixTitle, fixDesc, string(model.BeadTypeTask), 0)
				if err != nil {
					run.Emit(agent.StreamEvent{Type: "error", Content: "Failed to create fix bead: " + err.Error()})
					break
				}
				if err := bdClaim(ctx, project.HostDir, fixID); err != nil {
					run.Emit(agent.StreamEvent{Type: "error", Content: "Failed to claim fix bead: " + err.Error()})
					break
				}

				fixBead := model.Bead{
					ID:          fixID,
					Title:       fixTitle,
					Description: fixDesc,
					Type:        model.BeadTypeTask,
					Status:      model.BeadStatusInProgress,
					Priority:    0,
					Deps:        []string{},
				}
				beadJSON, _ := json.Marshal(fixBead)
				run.Emit(agent.StreamEvent{Type: "bead_created", Content: string(beadJSON)})
				run.Emit(agent.StreamEvent{Type: "log", Content: fmt.Sprintf("[%s] 🔧 Fixing validation failures...", fixID)})

				fixEvents, err := agent.ExecuteBead(ctx, project.HostDir, fixBead, artifacts, enhCtx)
				if err != nil {
					run.Emit(agent.StreamEvent{Type: "error", Content: "Fix bead execution failed: " + err.Error()})
					break
				}
				for ev := range fixEvents {
					if ev.Type == "plan_limit" {
						run.Emit(ev)
						break
					} else if ev.Type == "chunk" {
						run.Emit(agent.StreamEvent{Type: "log", Content: fmt.Sprintf("[%s] %s", fixID, ev.Content)})
					} else if ev.Type == "tokens" && ev.Tokens > 0 {
						totalBuildTokens.Add(int64(ev.Tokens))
					}
				}
				bdClose(ctx, project.HostDir, fixID)
				run.Emit(agent.StreamEvent{Type: "log", Content: fmt.Sprintf("[%s] ✅ Fix applied, re-validating...", fixID)})
			}
		}
		// --- End Validation Gate ---

		run.Emit(agent.StreamEvent{Type: "done", Content: "Execution complete"})
	}()

	return run, nil
}

// --- bd CLI helpers ---

func ensureBdInitRun(ctx context.Context, hostDir string, run *stream.Run) error {
	beadsDir := filepath.Join(hostDir, ".beads")
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		run.Emit(agent.StreamEvent{Type: "log", Content: "Initializing bd in " + hostDir + "..."})
		out2, err2 := runBd(ctx, hostDir, "init")
		if err2 != nil {
			return fmt.Errorf("bd init: %w\n%s", err2, out2)
		}
		run.Emit(agent.StreamEvent{Type: "log", Content: "bd initialized: " + strings.TrimSpace(out2)})
		return nil
	}
	out, err := runBd(ctx, hostDir, "status")
	if err != nil {
		return fmt.Errorf("bd status: %w\n%s", err, out)
	}
	run.Emit(agent.StreamEvent{Type: "log", Content: "bd ready: " + strings.TrimSpace(out)})
	return nil
}

func bdCreate(ctx context.Context, hostDir, title, description, beadType string, priority int) (string, error) {
	args := []string{"create",
		"--title", title,
		"--type", beadType,
		"--priority", fmt.Sprintf("%d", priority),
	}
	if description != "" {
		args = append(args, "--description", description)
	}

	out, err := runBd(ctx, hostDir, args...)
	if err != nil {
		return "", fmt.Errorf("bd create: %w\n%s", err, out)
	}

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		idx := strings.Index(line, "Created issue:")
		if idx < 0 {
			continue
		}
		after := strings.TrimSpace(line[idx+len("Created issue:"):])
		parts := strings.Fields(after)
		if len(parts) > 0 {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("could not parse bd create output: %q", out)
}

func bdDepAdd(ctx context.Context, hostDir, issueID, dependsOnID string) {
	runBd(ctx, hostDir, "dep", "add", issueID, dependsOnID)
}

func bdReady(ctx context.Context, hostDir string) (*model.Bead, error) {
	out, err := runBd(ctx, hostDir, "ready", "--json", "-n", "1")
	if err != nil || strings.TrimSpace(out) == "" || strings.TrimSpace(out) == "[]" {
		return nil, nil
	}

	var beads []struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Type        string `json:"issue_type"`
		Priority    int    `json:"priority"`
	}
	if err := json.Unmarshal([]byte(out), &beads); err != nil || len(beads) == 0 {
		return nil, nil
	}

	b := beads[0]
	return &model.Bead{
		ID:          b.ID,
		Title:       b.Title,
		Description: b.Description,
		Type:        model.BeadType(b.Type),
		Status:      model.BeadStatusOpen,
		Priority:    b.Priority,
		Deps:        []string{},
	}, nil
}

func bdResetStale(ctx context.Context, hostDir string) error {
	out, err := runBd(ctx, hostDir, "list", "--status=in_progress", "--json")
	if err != nil || strings.TrimSpace(out) == "" || strings.TrimSpace(out) == "[]" {
		return err
	}
	var beads []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(out), &beads); err != nil {
		return err
	}
	for _, b := range beads {
		runBd(ctx, hostDir, "update", b.ID, "--status=open", "--assignee=")
	}
	return nil
}

func bdClaim(ctx context.Context, hostDir, id string) error {
	out, err := runBd(ctx, hostDir, "update", id, "--claim")
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(out))
	}
	return nil
}

func bdClose(ctx context.Context, hostDir, id string) error {
	_, err := runBd(ctx, hostDir, "close", id)
	return err
}

func runBd(ctx context.Context, hostDir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "bd", args...)
	cmd.Dir = hostDir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func updateGraphStatus(mu *sync.Mutex, hostDir, beadID string, status model.BeadStatus) {
	updateGraphBeadFields(mu, hostDir, beadID, func(b *model.Bead) {
		b.Status = status
	})
}

// updateGraphBeadFields applies an arbitrary mutation to a single bead in the stored graph.
func updateGraphBeadFields(mu *sync.Mutex, hostDir, beadID string, mutate func(*model.Bead)) {
	mu.Lock()
	defer mu.Unlock()

	graph, err := fsrepo.ReadBeadGraph(hostDir)
	if err != nil {
		return
	}
	for i := range graph.Beads {
		if graph.Beads[i].ID == beadID {
			mutate(&graph.Beads[i])
			break
		}
	}
	fsrepo.WriteBeadGraph(hostDir, graph)
}

// enrichBeadFromGraph copies Tags, TargetFiles, and EpicID from the stored bead graph
// into a bead returned by bdReady (which only has basic fields from the bd CLI).
func enrichBeadFromGraph(hostDir string, bead *model.Bead) {
	graph, err := fsrepo.ReadBeadGraph(hostDir)
	if err != nil {
		return
	}
	for _, b := range graph.Beads {
		if b.ID == bead.ID {
			bead.Tags = b.Tags
			bead.TargetFiles = b.TargetFiles
			bead.EpicID = b.EpicID
			bead.JourneyRefs = b.JourneyRefs
			bead.ArchRefs = b.ArchRefs
			return
		}
	}
}

// ── Bead detail endpoints ──

// beadDetailResponse is the JSON returned by GetDetail.
type beadDetailResponse struct {
	model.Bead
	Notes            string          `json:"notes"`
	ExecutionContent string          `json:"executionContent"`
	ChatMessages     []model.Message `json:"chatMessages"`
}

// GetDetail returns full bead details including execution output and notes.
func (h *BeadHandler) GetDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	beadID := chi.URLParam(r, "beadId")

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	// Find bead in graph
	graph, err := fsrepo.ReadBeadGraph(project.HostDir)
	if err != nil {
		http.Error(w, "failed to read bead graph", http.StatusInternalServerError)
		return
	}

	var found *model.Bead
	for i := range graph.Beads {
		if graph.Beads[i].ID == beadID {
			found = &graph.Beads[i]
			break
		}
	}
	if found == nil {
		http.Error(w, "bead not found", http.StatusNotFound)
		return
	}

	execContent, _ := fsrepo.ReadBeadDoc(project.HostDir, project.Version, beadID)
	notes, _ := fsrepo.ReadBeadNotes(project.HostDir, project.Version, beadID)
	chatMsgs, _ := fsrepo.ReadBeadChatHistory(project.HostDir, project.Version, beadID)
	if chatMsgs == nil {
		chatMsgs = []model.Message{}
	}

	resp := beadDetailResponse{
		Bead:             *found,
		Notes:            notes,
		ExecutionContent: execContent,
		ChatMessages:     chatMsgs,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

type updateBeadRequest struct {
	Description *string `json:"description"`
	Notes       *string `json:"notes"`
}

// UpdateBead patches a bead's description or notes.
func (h *BeadHandler) UpdateBead(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	beadID := chi.URLParam(r, "beadId")

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	var req updateBeadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Description != nil {
		_, err := runBd(r.Context(), project.HostDir, "update", beadID, "--description", *req.Description)
		if err != nil {
			http.Error(w, "failed to update description", http.StatusInternalServerError)
			return
		}
		// Update in graph
		mu := beadGraphLock(project.HostDir)
		mu.Lock()
		graph, err := fsrepo.ReadBeadGraph(project.HostDir)
		if err == nil {
			for i := range graph.Beads {
				if graph.Beads[i].ID == beadID {
					graph.Beads[i].Description = *req.Description
					break
				}
			}
			fsrepo.WriteBeadGraph(project.HostDir, graph)
		}
		mu.Unlock()
	}

	if req.Notes != nil {
		if err := fsrepo.WriteBeadNotes(project.HostDir, project.Version, beadID, *req.Notes); err != nil {
			http.Error(w, "failed to write notes", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

type controlBeadRequest struct {
	Action string `json:"action"` // "pause" or "restart"
}

// ControlBead pauses or restarts a bead.
func (h *BeadHandler) ControlBead(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	beadID := chi.URLParam(r, "beadId")

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	var req controlBeadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	switch req.Action {
	case "pause":
		if _, err := runBd(r.Context(), project.HostDir, "update", beadID, "--status=open", "--assignee="); err != nil {
			http.Error(w, "failed to pause bead", http.StatusInternalServerError)
			return
		}
		updateGraphStatus(beadGraphLock(project.HostDir), project.HostDir, beadID, model.BeadStatusOpen)

	case "restart":
		// Soft restart: reset to open so it can be re-picked by the execution loop
		if _, err := runBd(r.Context(), project.HostDir, "update", beadID, "--status=open", "--assignee="); err != nil {
			http.Error(w, "failed to restart bead", http.StatusInternalServerError)
			return
		}
		updateGraphStatus(beadGraphLock(project.HostDir), project.HostDir, beadID, model.BeadStatusOpen)

	default:
		http.Error(w, "invalid action: must be 'pause' or 'restart'", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type beadChatRequest struct {
	Message string `json:"message"`
}

// BeadChat handles per-bead AI chat via SSE streaming.
func (h *BeadHandler) BeadChat(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	beadID := chi.URLParam(r, "beadId")

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	// Check for existing active run
	runKey := "bead-chat-" + beadID
	if existing := h.runs.Active(id, "build", runKey); existing != nil {
		existing.StreamTo(w, r, 0)
		return
	}

	var req beadChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Find bead in graph
	graph, err := fsrepo.ReadBeadGraph(project.HostDir)
	if err != nil {
		http.Error(w, "failed to read bead graph", http.StatusInternalServerError)
		return
	}
	var bead *model.Bead
	for i := range graph.Beads {
		if graph.Beads[i].ID == beadID {
			bead = &graph.Beads[i]
			break
		}
	}
	if bead == nil {
		http.Error(w, "bead not found", http.StatusNotFound)
		return
	}

	// Save user message
	userMsg := model.Message{
		Role:      model.RoleUser,
		Content:   req.Message,
		Timestamp: time.Now(),
	}
	fsrepo.AppendBeadChatMessage(project.HostDir, project.Version, beadID, userMsg)

	// Build system prompt with bead context
	execContent, _ := fsrepo.ReadBeadDoc(project.HostDir, project.Version, beadID)
	notes, _ := fsrepo.ReadBeadNotes(project.HostDir, project.Version, beadID)

	var sb strings.Builder
	sb.WriteString("You are an AI assistant helping with a specific development task (bead).\n\n")
	sb.WriteString(fmt.Sprintf("## Bead: %s\n", bead.Title))
	sb.WriteString(fmt.Sprintf("- Type: %s\n- Status: %s\n- Priority: %d\n\n", bead.Type, bead.Status, bead.Priority))
	if bead.Description != "" {
		sb.WriteString(fmt.Sprintf("## Description\n%s\n\n", bead.Description))
	}
	if notes != "" {
		sb.WriteString(fmt.Sprintf("## User Notes\n%s\n\n", notes))
	}
	if execContent != "" {
		sb.WriteString(fmt.Sprintf("## Previous Execution Output\n%s\n\n", execContent))
	}
	sb.WriteString("Help the user refine this task. You can discuss implementation details, suggest approaches, or help update the task description and notes.")

	systemPrompt := sb.String()

	// Load chat history
	chatHistory, _ := fsrepo.ReadBeadChatHistory(project.HostDir, project.Version, beadID)
	// Remove last message (just appended)
	if len(chatHistory) > 0 {
		chatHistory = chatHistory[:len(chatHistory)-1]
	}

	run := h.runs.Start(id, "build", runKey)
	if run == nil {
		http.Error(w, "chat already running for this bead", http.StatusConflict)
		return
	}

	events, err := agent.Chat(run.Context(), "claude-sonnet-4-6", systemPrompt, chatHistory, req.Message, project.HostDir)
	if err != nil {
		run.Finish(h.runs)
		http.Error(w, "failed to start agent: "+err.Error(), http.StatusInternalServerError)
		return
	}

	go func() {
		defer run.Finish(h.runs)

		for event := range events {
			if event.Type == "done" {
				assistantMsg := model.Message{
					Role:      model.RoleAssistant,
					Content:   event.Content,
					Timestamp: time.Now(),
				}
				fsrepo.AppendBeadChatMessage(project.HostDir, project.Version, beadID, assistantMsg)
				run.Emit(agent.StreamEvent{Type: "done", Content: event.Content})
			} else {
				run.Emit(event)
			}
		}
	}()

	run.StreamTo(w, r, 0)
}

// ── Code tab endpoints ──

type beadFileEntry struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Language string `json:"language"`
}

type beadFilesResponse struct {
	Files []beadFileEntry `json:"files"`
}

// GetBeadFiles returns the current content of all target files for a bead.
func (h *BeadHandler) GetBeadFiles(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	beadID := chi.URLParam(r, "beadId")

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	graph, err := fsrepo.ReadBeadGraph(project.HostDir)
	if err != nil {
		http.Error(w, "failed to read bead graph", http.StatusInternalServerError)
		return
	}

	var bead *model.Bead
	for i := range graph.Beads {
		if graph.Beads[i].ID == beadID {
			bead = &graph.Beads[i]
			break
		}
	}
	if bead == nil {
		http.Error(w, "bead not found", http.StatusNotFound)
		return
	}

	resp := beadFilesResponse{Files: make([]beadFileEntry, 0, len(bead.TargetFiles))}
	for _, relPath := range bead.TargetFiles {
		absPath := filepath.Join(project.HostDir, relPath)
		data, readErr := os.ReadFile(absPath)
		if readErr != nil {
			// File doesn't exist yet — include entry with empty content
			resp.Files = append(resp.Files, beadFileEntry{
				Path:     relPath,
				Content:  "",
				Language: languageFromPath(relPath),
			})
			continue
		}
		resp.Files = append(resp.Files, beadFileEntry{
			Path:     relPath,
			Content:  string(data),
			Language: languageFromPath(relPath),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

type beadDiffEntry struct {
	Path     string `json:"path"`
	Original string `json:"original"`
	Modified string `json:"modified"`
	Language string `json:"language"`
}

type beadDiffResponse struct {
	Diffs []beadDiffEntry `json:"diffs"`
}

// GetBeadDiff returns git diff (original vs current) for each target file of a bead.
func (h *BeadHandler) GetBeadDiff(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	beadID := chi.URLParam(r, "beadId")

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	graph, err := fsrepo.ReadBeadGraph(project.HostDir)
	if err != nil {
		http.Error(w, "failed to read bead graph", http.StatusInternalServerError)
		return
	}

	var bead *model.Bead
	for i := range graph.Beads {
		if graph.Beads[i].ID == beadID {
			bead = &graph.Beads[i]
			break
		}
	}
	if bead == nil {
		http.Error(w, "bead not found", http.StatusNotFound)
		return
	}

	resp := beadDiffResponse{Diffs: make([]beadDiffEntry, 0, len(bead.TargetFiles))}
	for _, relPath := range bead.TargetFiles {
		original, modified, diffErr := h.gitSvc.FileDiff(project.HostDir, relPath, bead.PreExecutionCommit)
		if diffErr != nil {
			continue
		}
		resp.Diffs = append(resp.Diffs, beadDiffEntry{
			Path:     relPath,
			Original: original,
			Modified: modified,
			Language: languageFromPath(relPath),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// languageFromPath infers a Monaco editor language ID from a file extension.
func languageFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".py":
		return "python"
	case ".md":
		return "markdown"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".html":
		return "html"
	case ".css":
		return "css"
	case ".sh", ".bash":
		return "shell"
	case ".sql":
		return "sql"
	case ".toml":
		return "ini"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".cs":
		return "csharp"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	default:
		return "plaintext"
	}
}

// siblingBeads returns all task beads in the same epic as bead, excluding bead itself.
func siblingBeads(hostDir string, bead model.Bead) []model.Bead {
	if bead.EpicID == "" {
		return nil
	}
	graph, err := fsrepo.ReadBeadGraph(hostDir)
	if err != nil {
		return nil
	}
	var siblings []model.Bead
	for _, b := range graph.Beads {
		if b.EpicID == bead.EpicID && b.ID != bead.ID && b.Type == model.BeadTypeTask {
			siblings = append(siblings, b)
		}
	}
	return siblings
}
