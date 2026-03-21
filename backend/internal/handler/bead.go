package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/ai-app-factory/backend/internal/agent"
	"github.com/michelroberge/ai-app-factory/backend/internal/model"
	"github.com/michelroberge/ai-app-factory/backend/internal/repository"
	fsrepo "github.com/michelroberge/ai-app-factory/backend/internal/repository/fs"
)

// beadGraphMu serializes writes to beads-graph.json per project host directory.
var beadGraphMu sync.Map // key: hostDir string, value: *sync.Mutex

func beadGraphLock(hostDir string) *sync.Mutex {
	mu, _ := beadGraphMu.LoadOrStore(hostDir, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

type BeadHandler struct {
	registry     repository.RegistryRepo
	artifactRepo repository.ArtifactRepo
}

func NewBeadHandler(registry repository.RegistryRepo, artifactRepo repository.ArtifactRepo) *BeadHandler {
	return &BeadHandler{registry: registry, artifactRepo: artifactRepo}
}

// GetGraph returns the current bead graph JSON for a project.
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(graph)
}

// Generate parses build.md and creates beads via the bd CLI, streaming progress via SSE.
func (h *BeadHandler) Generate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	buildContent, _ := h.artifactRepo.Read(project.HostDir, model.StageBuild)
	if buildContent == "" {
		http.Error(w, "no build artifact — complete the Build stage first", http.StatusBadRequest)
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

	ctx := r.Context()

	// Step 1: Parse build.md with Claude
	sseWrite(w, flusher, agent.StreamEvent{Type: "log", Content: "Parsing build plan..."})

	events, err := agent.ParseBuildPlan(ctx, buildContent)
	if err != nil {
		sseWrite(w, flusher, agent.StreamEvent{Type: "error", Content: "Failed to start parser: " + err.Error()})
		return
	}

	var fullResponse string
	for event := range events {
		if event.Type == "done" {
			fullResponse = event.Content
		} else if event.Type == "chunk" {
			sseWrite(w, flusher, event)
		}
	}

	jsonBytes, ok2 := agent.ExtractBeadJSON(fullResponse)
	if !ok2 {
		sseWrite(w, flusher, agent.StreamEvent{Type: "error", Content: "Failed to extract JSON from parser response"})
		return
	}

	var plan model.ParsedBuildPlan
	if err := json.Unmarshal(jsonBytes, &plan); err != nil {
		sseWrite(w, flusher, agent.StreamEvent{Type: "error", Content: "Failed to parse JSON: " + err.Error()})
		return
	}

	// Step 2: Ensure bd is initialized
	sseWrite(w, flusher, agent.StreamEvent{Type: "log", Content: "Checking bd status..."})
	if err := ensureBdInit(ctx, project.HostDir, w, flusher); err != nil {
		sseWrite(w, flusher, agent.StreamEvent{Type: "error", Content: "bd init failed: " + err.Error()})
		return
	}

	// Step 3: Create epics and tasks
	graph := &model.BeadGraph{
		GeneratedAt: time.Now(),
		ProjectID:   project.ID,
		Beads:       []model.Bead{},
	}

	// title → bead ID map for dep resolution
	titleToID := map[string]string{}

	for _, epic := range plan.Epics {
		if ctx.Err() != nil {
			return
		}

		epicID, err := bdCreate(ctx, project.HostDir, epic.Title, epic.Description, "epic", 2)
		if err != nil {
			sseWrite(w, flusher, agent.StreamEvent{Type: "error", Content: fmt.Sprintf("Failed to create epic %q: %v", epic.Title, err)})
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
		sseWrite(w, flusher, agent.StreamEvent{Type: "bead_created", Content: string(beadJSON)})

		fsrepo.WriteBeadGraph(project.HostDir, graph)

		for _, task := range epic.Tasks {
			if ctx.Err() != nil {
				return
			}

			taskID, err := bdCreate(ctx, project.HostDir, task.Title, task.Description, "task", task.Priority)
			if err != nil {
				sseWrite(w, flusher, agent.StreamEvent{Type: "error", Content: fmt.Sprintf("Failed to create task %q: %v", task.Title, err)})
				continue
			}

			taskBead := model.Bead{
				ID:          taskID,
				Title:       task.Title,
				Description: task.Description,
				Type:        model.BeadTypeTask,
				Status:      model.BeadStatusOpen,
				Priority:    task.Priority,
				Deps:        []string{},
			}
			titleToID[task.Title] = taskID
			graph.Beads = append(graph.Beads, taskBead)

			beadJSON, _ := json.Marshal(taskBead)
			sseWrite(w, flusher, agent.StreamEvent{Type: "bead_created", Content: string(beadJSON)})

			fsrepo.WriteBeadGraph(project.HostDir, graph)
		}
	}

	// Step 4: Set up dependencies
	sseWrite(w, flusher, agent.StreamEvent{Type: "log", Content: "Setting up dependencies..."})

	for ei, epic := range plan.Epics {
		for ti, task := range epic.Tasks {
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
				_ = ei
				_ = ti
			}
		}
	}

	fsrepo.WriteBeadGraph(project.HostDir, graph)

	summary := fmt.Sprintf("Generated %d beads (%d epics)", len(graph.Beads), len(plan.Epics))
	sseWrite(w, flusher, agent.StreamEvent{Type: "done", Content: summary})
	fmt.Fprintf(w, "\n")
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

	var req executeBeadsRequest
	req.MaxParallel = 2
	json.NewDecoder(r.Body).Decode(&req)
	if req.MaxParallel < 1 {
		req.MaxParallel = 1
	}
	if req.MaxParallel > 10 {
		req.MaxParallel = 10
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()

	// Load all artifacts for context injection
	artifacts := map[model.StageName]string{}
	for _, stage := range []model.StageName{model.StageVision, model.StageUX, model.StageArchitecture, model.StageBuild} {
		content, _ := h.artifactRepo.Read(project.HostDir, stage)
		if content != "" {
			artifacts[stage] = content
		}
	}

	events := make(chan agent.StreamEvent, 128)

	var wg sync.WaitGroup
	sem := make(chan struct{}, req.MaxParallel)
	var once sync.Once

	closeEvents := func() {
		once.Do(func() { close(events) })
	}

	mu := beadGraphLock(project.HostDir)

	go func() {
		defer closeEvents()

		for {
			if ctx.Err() != nil {
				break
			}

			bead, err := bdReady(ctx, project.HostDir)
			if err != nil || bead == nil {
				// No more ready beads — wait for in-flight to finish then exit
				break
			}

			sem <- struct{}{}
			wg.Add(1)

			go func(b model.Bead) {
				defer wg.Done()
				defer func() { <-sem }()

				// Claim
				if err := bdClaim(ctx, project.HostDir, b.ID); err != nil {
					events <- agent.StreamEvent{Type: "error", Content: fmt.Sprintf("[%s] claim failed: %v", b.ID, err)}
					return
				}

				b.Status = model.BeadStatusInProgress
				updateGraphStatus(mu, project.HostDir, b.ID, model.BeadStatusInProgress)
				beadJSON, _ := json.Marshal(b)
				events <- agent.StreamEvent{Type: "bead_update", Content: string(beadJSON)}
				events <- agent.StreamEvent{Type: "log", Content: fmt.Sprintf("[%s] Starting: %s", b.ID, b.Title)}

				// Execute
				agentEvents, err := agent.ExecuteBead(ctx, project.HostDir, b, artifacts)
				if err != nil {
					events <- agent.StreamEvent{Type: "error", Content: fmt.Sprintf("[%s] execute failed: %v", b.ID, err)}
					return
				}

				for ev := range agentEvents {
					if ev.Type == "chunk" {
						events <- agent.StreamEvent{Type: "log", Content: fmt.Sprintf("[%s] %s", b.ID, ev.Content)}
					}
				}

				// Close
				if err := bdClose(ctx, project.HostDir, b.ID); err != nil {
					events <- agent.StreamEvent{Type: "error", Content: fmt.Sprintf("[%s] close failed: %v", b.ID, err)}
					return
				}

				b.Status = model.BeadStatusClosed
				updateGraphStatus(mu, project.HostDir, b.ID, model.BeadStatusClosed)
				beadJSON, _ = json.Marshal(b)
				events <- agent.StreamEvent{Type: "bead_update", Content: string(beadJSON)}
				events <- agent.StreamEvent{Type: "log", Content: fmt.Sprintf("[%s] Done: %s", b.ID, b.Title)}
			}(bead)
		}

		wg.Wait()
		closeEvents()
	}()

	for event := range events {
		sseWrite(w, flusher, event)
	}

	sseWrite(w, flusher, agent.StreamEvent{Type: "done", Content: "Execution complete"})
	fmt.Fprintf(w, "\n")
}

// --- bd CLI helpers ---

func ensureBdInit(ctx context.Context, hostDir string, w http.ResponseWriter, flusher http.Flusher) error {
	out, err := runBd(ctx, hostDir, "status")
	if err != nil {
		// Not initialized — run init
		sseWrite(w, flusher, agent.StreamEvent{Type: "log", Content: "Initializing bd..."})
		out2, err2 := runBd(ctx, hostDir, "init")
		if err2 != nil {
			return fmt.Errorf("bd init: %w\n%s", err2, out2)
		}
		sseWrite(w, flusher, agent.StreamEvent{Type: "log", Content: "bd initialized: " + strings.TrimSpace(out2)})
		return nil
	}
	sseWrite(w, flusher, agent.StreamEvent{Type: "log", Content: "bd ready: " + strings.TrimSpace(out)})
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

	// bd create outputs the issue ID on the last line (e.g. "Created issue ai-app-001")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		// Extract ID: "Created issue <ID>" or just "<ID>"
		parts := strings.Fields(line)
		return parts[len(parts)-1], nil
	}
	return "", fmt.Errorf("could not parse bd create output: %q", out)
}

func bdDepAdd(ctx context.Context, hostDir, issueID, dependsOnID string) {
	runBd(ctx, hostDir, "dep", "add", issueID, dependsOnID)
}

func bdReady(ctx context.Context, hostDir string) (*model.Bead, error) {
	out, err := runBd(ctx, hostDir, "ready", "--output=json")
	if err != nil || strings.TrimSpace(out) == "" || strings.TrimSpace(out) == "[]" {
		return nil, nil
	}

	// Parse the first ready bead
	var beads []struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Type        string `json:"type"`
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

func bdClaim(ctx context.Context, hostDir, id string) error {
	_, err := runBd(ctx, hostDir, "update", id, "--claim")
	return err
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
	mu.Lock()
	defer mu.Unlock()

	graph, err := fsrepo.ReadBeadGraph(hostDir)
	if err != nil {
		return
	}
	for i := range graph.Beads {
		if graph.Beads[i].ID == beadID {
			graph.Beads[i].Status = status
			break
		}
	}
	fsrepo.WriteBeadGraph(hostDir, graph)
}
