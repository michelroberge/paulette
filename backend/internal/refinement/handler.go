package refinement

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/provider"
	"github.com/michelroberge/paulette/backend/internal/repository"
	fsrepo "github.com/michelroberge/paulette/backend/internal/repository/fs"
	"github.com/michelroberge/paulette/backend/internal/stream"
)

// Handler handles HTTP requests for the refinement loop.
type Handler struct {
	registry         repository.RegistryRepo
	artifactRepo     repository.ArtifactRepo
	activityRepo     repository.ActivityRepo
	runs             *stream.Manager
	providerRegistry *provider.Registry
	stageConfig      *provider.StageConfigStore
	logBase          string

	// answerChannels tracks per-project answer channels for interactive mode.
	mu       sync.Mutex
	answerCh map[string]chan string // projectID → channel
}

// NewHandler constructs a refinement Handler.
func NewHandler(
	registry repository.RegistryRepo,
	artifactRepo repository.ArtifactRepo,
	activityRepo repository.ActivityRepo,
	runs *stream.Manager,
	providerRegistry *provider.Registry,
	stageConfig *provider.StageConfigStore,
	logBase string,
) *Handler {
	return &Handler{
		registry:         registry,
		artifactRepo:     artifactRepo,
		activityRepo:     activityRepo,
		runs:             runs,
		providerRegistry: providerRegistry,
		stageConfig:      stageConfig,
		logBase:          logBase,
		answerCh:         make(map[string]chan string),
	}
}

// resolveProvider returns the provider for the vision stage.
func (h *Handler) resolveProvider(projectID, hostDir string) (provider.Provider, string, *provider.StageAssignment, error) {
	if h.providerRegistry != nil {
		return h.providerRegistry.ResolveForStageWithSettings(projectID, model.StageVision, h.stageConfig, hostDir)
	}
	return provider.NewClaudeCLIProvider(), provider.FallbackModel(model.StageVision), nil, nil
}

type startRequest struct {
	Message string `json:"message"`
}

// StartLoop starts or resumes the refinement loop. Streams progress via SSE.
func (h *Handler) StartLoop(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	// Check for existing active run
	if existing := h.runs.Active(id, "vision", "refinement"); existing != nil {
		existing.StreamTo(w, r, 0)
		return
	}

	var req startRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Load or create state
	state, err := LoadState(project.DataDir)
	if err != nil {
		http.Error(w, "failed to load state: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if state == nil {
		idea := req.Message
		if idea == "" {
			http.Error(w, "message is required to start the loop", http.StatusBadRequest)
			return
		}
		state = NewLoopState(idea, VisionSections, 10)
	}

	// Resolve provider
	prov, modelID, sa, provErr := h.resolveProvider(project.ID, project.HostDir)
	if provErr != nil {
		http.Error(w, "provider error: "+provErr.Error(), http.StatusInternalServerError)
		return
	}

	// Create answer channel for interactive mode
	answerCh := make(chan string, 1)
	h.mu.Lock()
	h.answerCh[project.ID] = answerCh
	h.mu.Unlock()

	// Start managed run
	run := h.runs.Start(project.ID, "vision", "refinement")
	if run == nil {
		http.Error(w, "a refinement loop is already running", http.StatusConflict)
		return
	}
	writeActivity(h.activityRepo, project.DataDir, model.StageVision, "refinement")
	runLogID := h.startRunLog(project)

	saTemp, _, _ := sa.Fields()
	var numCtx *int
	if sa != nil && sa.NumCtx != nil {
		numCtx = sa.NumCtx
	}

	trace := NewTrace(h.logBase, project.Name, runLogID, modelID)

	ctrl := NewController(ControllerConfig{
		Provider:      prov,
		ModelID:       modelID,
		Temperature:   saTemp,
		NumCtx:        numCtx,
		ProjectDir:    project.HostDir,
		Stage:         "vision",
		Prompts:       VisionPromptSet{},
		Trace:         trace,
		MaxIter:       state.MaxIterations,
		MinConfidence: 0.85,
	})

	go func() {
		defer run.Finish(h.runs)
		defer clearActivity(h.activityRepo, project.DataDir, model.StageVision)
		defer func() {
			h.mu.Lock()
			delete(h.answerCh, project.ID)
			h.mu.Unlock()
		}()

		runStart := time.Now()
		emitFn := func(event LoopEvent) {
			run.Emit(LoopEventToStreamEvent(event))
		}
		saveFn := func(s *LoopState) {
			if err := SaveState(project.DataDir, s); err != nil {
				log.Printf("refinement: save state error: %v", err)
			}
		}

		artifact, totalTokens, err := ctrl.RunLoop(run.Context(), state, answerCh, emitFn, saveFn)
		if err != nil {
			state.Error = err.Error()
			saveFn(state)
			h.failRunLog(project.Name, runLogID, err.Error())
			run.Emit(model.StreamEvent{Type: "error", Content: err.Error()})
			return
		}

		// Save artifact
		if err := h.artifactRepo.Write(project.DataDir, model.StageVision, artifact); err != nil {
			h.failRunLog(project.Name, runLogID, "failed to save artifact: "+err.Error())
			run.Emit(model.StreamEvent{Type: "error", Content: "failed to save artifact"})
			return
		}
		run.Emit(model.StreamEvent{Type: "artifact", Content: "vision/vision.md"})

		// Track tokens
		if totalTokens > 0 {
			project.AddStageTokens(model.StageVision, totalTokens)
			h.registry.Update(project)
			h.recordSession(project.DataDir, project.Iteration, runStart, totalTokens)
		}

		h.successRunLog(project.Name, runLogID, totalTokens)
		run.Emit(model.StreamEvent{Type: "done", Content: artifact})
	}()

	run.StreamTo(w, r, 0)
}

type answerRequest struct {
	Answer string `json:"answer"`
}

// AnswerQuestion delivers a user answer to the running loop.
func (h *Handler) AnswerQuestion(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req answerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	ch, ok := h.answerCh[id]
	h.mu.Unlock()

	if !ok {
		http.Error(w, "no active refinement loop for this project", http.StatusNotFound)
		return
	}

	select {
	case ch <- req.Answer:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	default:
		http.Error(w, "loop is not waiting for an answer", http.StatusConflict)
	}
}

// GetState returns the current loop state.
func (h *Handler) GetState(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	state, err := LoadState(project.DataDir)
	if err != nil {
		http.Error(w, "failed to load state: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if state == nil {
		http.Error(w, "no refinement state", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

// ResetState clears the loop state.
func (h *Handler) ResetState(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	if err := ClearState(project.DataDir); err != nil {
		http.Error(w, "failed to clear state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// GetReviewItems returns pending review items for a project.
func (h *Handler) GetReviewItems(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	state, err := LoadState(project.DataDir)
	if err != nil {
		http.Error(w, "failed to load state: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if state == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]ReviewItem{})
		return
	}

	// Return only pending items
	var pending []ReviewItem
	for _, item := range state.ReviewItems {
		if item.Status == ReviewPending {
			pending = append(pending, item)
		}
	}
	if pending == nil {
		pending = []ReviewItem{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pending)
}

// DiscardReviewItem marks a review item as discarded.
func (h *Handler) DiscardReviewItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	itemID := chi.URLParam(r, "itemId")

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	state, err := LoadState(project.DataDir)
	if err != nil {
		http.Error(w, "failed to load state: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if state == nil {
		http.Error(w, "no refinement state", http.StatusNotFound)
		return
	}

	found := false
	for i := range state.ReviewItems {
		if state.ReviewItems[i].ID == itemID {
			state.ReviewItems[i].Status = ReviewDiscarded
			found = true
			break
		}
	}
	if !found {
		http.Error(w, "review item not found", http.StatusNotFound)
		return
	}

	if err := SaveState(project.DataDir, state); err != nil {
		http.Error(w, "failed to save state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

type addressRequest struct {
	Response string `json:"response"`
}

type addressResponse struct {
	Status  string `json:"status"`
	Summary string `json:"summary"`
}

// AddressReviewItem marks a review item as addressed, takes the user's response,
// calls the LLM to update the vision artifact incorporating the concern and
// response, then saves the updated artifact.
func (h *Handler) AddressReviewItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	itemID := chi.URLParam(r, "itemId")

	var req addressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Response == "" {
		http.Error(w, "response is required", http.StatusBadRequest)
		return
	}

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	state, err := LoadState(project.DataDir)
	if err != nil {
		http.Error(w, "failed to load state: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if state == nil {
		http.Error(w, "no refinement state", http.StatusNotFound)
		return
	}

	// Find and mark the review item
	var item *ReviewItem
	for i := range state.ReviewItems {
		if state.ReviewItems[i].ID == itemID {
			state.ReviewItems[i].Status = ReviewAddressed
			item = &state.ReviewItems[i]
			break
		}
	}
	if item == nil {
		http.Error(w, "review item not found", http.StatusNotFound)
		return
	}

	// Load current artifact
	artifact, artErr := h.artifactRepo.Read(project.DataDir, model.StageVision)
	if artErr != nil || artifact == "" {
		http.Error(w, "no vision artifact to update", http.StatusNotFound)
		return
	}

	// Resolve provider and call LLM to update the artifact
	prov, modelID, sa, provErr := h.resolveProvider(project.ID, project.HostDir)
	if provErr != nil {
		http.Error(w, "provider error: "+provErr.Error(), http.StatusInternalServerError)
		return
	}

	saTemp, _, _ := sa.Fields()
	temp := saTemp
	if temp == nil {
		t := 0.3
		temp = &t
	}

	systemPrompt := `You are updating a product vision document. You will receive:
1. The current vision document
2. A concern that was identified during review
3. The user's response to that concern

Update the vision document to incorporate the user's response. Address the concern directly.
Output ONLY the complete updated vision document — no commentary, no explanations.`

	userMessage := fmt.Sprintf("## Current Vision Document\n\n%s\n\n## Concern (%s — %s)\n\n%s\n\n## User's Response\n\n%s",
		artifact, item.Source, item.Section, item.Text, req.Response)

	events, llmErr := prov.Chat(r.Context(), provider.ChatRequest{
		Model:        modelID,
		SystemPrompt: systemPrompt,
		UserMessage:  userMessage,
		ProjectDir:   project.HostDir,
		Stage:        "vision",
		Temperature:  temp,
	})
	if llmErr != nil {
		http.Error(w, "LLM error: "+llmErr.Error(), http.StatusInternalServerError)
		return
	}

	// Collect response
	var sb strings.Builder
	for event := range events {
		switch event.Type {
		case "chunk":
			sb.WriteString(event.Content)
		case "done":
			if event.Content != "" {
				sb.Reset()
				sb.WriteString(event.Content)
			}
		case "error":
			http.Error(w, "LLM error: "+event.Content, http.StatusInternalServerError)
			return
		}
	}

	updatedArtifact := strings.TrimSpace(sb.String())
	if updatedArtifact == "" {
		http.Error(w, "LLM returned empty response", http.StatusInternalServerError)
		return
	}

	// Save updated artifact
	if err := h.artifactRepo.Write(project.DataDir, model.StageVision, updatedArtifact); err != nil {
		http.Error(w, "failed to save artifact: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Save state (item marked as addressed)
	if err := SaveState(project.DataDir, state); err != nil {
		log.Printf("refinement: save state after address: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(addressResponse{
		Status:  "ok",
		Summary: fmt.Sprintf("Vision document updated to address %s concern: %s", item.Source, item.Text),
	})
}

// RunAutonomous executes the refinement loop without HTTP, for use by the orchestrator.
// It creates a stream.Run, runs the loop with answerCh=nil, saves the artifact, and returns.
func (h *Handler) RunAutonomous(ctx context.Context, project *model.Project, idea string) (*stream.Run, error) {
	// Load or create state
	state, err := LoadState(project.DataDir)
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}
	if state == nil {
		state = NewLoopState(idea, VisionSections, 10)
	}

	// Resolve provider
	prov, modelID, sa, provErr := h.resolveProvider(project.ID, project.HostDir)
	if provErr != nil {
		return nil, fmt.Errorf("resolve provider: %w", provErr)
	}

	run := h.runs.Start(project.ID, "vision", "refinement")
	if run == nil {
		return nil, nil
	}
	writeActivity(h.activityRepo, project.DataDir, model.StageVision, "refinement")
	runLogID := h.startRunLog(project)

	saTemp, _, _ := sa.Fields()
	var numCtx *int
	if sa != nil && sa.NumCtx != nil {
		numCtx = sa.NumCtx
	}

	trace := NewTrace(h.logBase, project.Name, runLogID, modelID)

	ctrl := NewController(ControllerConfig{
		Provider:      prov,
		ModelID:       modelID,
		Temperature:   saTemp,
		NumCtx:        numCtx,
		ProjectDir:    project.HostDir,
		Stage:         "vision",
		Prompts:       VisionPromptSet{},
		Trace:         trace,
		MaxIter:       state.MaxIterations,
		MinConfidence: 0.85,
	})

	go func() {
		defer run.Finish(h.runs)
		defer clearActivity(h.activityRepo, project.DataDir, model.StageVision)

		runStart := time.Now()
		emitFn := func(event LoopEvent) {
			run.Emit(LoopEventToStreamEvent(event))
		}
		saveFn := func(s *LoopState) {
			if err := SaveState(project.DataDir, s); err != nil {
				log.Printf("refinement: save state error: %v", err)
			}
		}

		artifact, totalTokens, err := ctrl.RunLoop(ctx, state, nil, emitFn, saveFn)
		if err != nil {
			state.Error = err.Error()
			saveFn(state)
			h.failRunLog(project.Name, runLogID, err.Error())
			run.Emit(model.StreamEvent{Type: "error", Content: err.Error()})
			return
		}

		if err := h.artifactRepo.Write(project.DataDir, model.StageVision, artifact); err != nil {
			h.failRunLog(project.Name, runLogID, "failed to save artifact: "+err.Error())
			run.Emit(model.StreamEvent{Type: "error", Content: "failed to save artifact"})
			return
		}
		run.Emit(model.StreamEvent{Type: "artifact", Content: "vision/vision.md"})

		if totalTokens > 0 {
			project.AddStageTokens(model.StageVision, totalTokens)
			h.registry.Update(project)
			h.recordSession(project.DataDir, project.Iteration, runStart, totalTokens)
		}

		h.successRunLog(project.Name, runLogID, totalTokens)
		run.Emit(model.StreamEvent{Type: "done", Content: artifact})
	}()

	return run, nil
}

// --- Helpers (mirror handler package patterns) ---

func writeActivity(repo repository.ActivityRepo, dataDir string, stage model.StageName, op string) {
	if repo == nil {
		return
	}
	repo.SetActivity(dataDir, stage, &model.StageActivity{
		Operation: op,
		Status:    "running",
		StartedAt: time.Now(),
	})
}

func clearActivity(repo repository.ActivityRepo, dataDir string, stage model.StageName) {
	if repo == nil {
		return
	}
	repo.ClearActivity(dataDir, stage)
}

func (h *Handler) startRunLog(project *model.Project) string {
	if h.logBase == "" {
		return ""
	}
	safeName := fsrepo.SanitizeProjectName(project.Name)
	attempt := fsrepo.CountProjectRunsForOp(h.logBase, safeName, model.StageVision, "refinement") + 1
	entry := model.RunLogEntry{
		ID:          uuid.NewString(),
		ProjectID:   project.ID,
		ProjectName: safeName,
		Stage:       model.StageVision,
		Operation:   "refinement",
		Attempt:     attempt,
		Status:      model.RunLogRunning,
		StartedAt:   time.Now(),
	}
	if err := fsrepo.WriteRunMeta(h.logBase, entry); err != nil {
		log.Printf("runlog: start refinement: %v", err)
		return ""
	}
	if _, err := fsrepo.PruneProjectRuns(h.logBase, safeName, 10); err != nil {
		log.Printf("runlog: prune %s: %v", safeName, err)
	}
	return entry.ID
}

func (h *Handler) successRunLog(projectName, runID string, tokens int) {
	if h.logBase == "" || runID == "" {
		return
	}
	safeName := fsrepo.SanitizeProjectName(projectName)
	entry, err := fsrepo.ReadRunMeta(h.logBase, safeName, runID)
	if err != nil {
		return
	}
	now := time.Now()
	entry.Status = model.RunLogSuccess
	entry.EndedAt = &now
	entry.DurationMs = now.Sub(entry.StartedAt).Milliseconds()
	entry.TokensTotal = tokens
	entry.Notes = "refinement loop completed"
	fsrepo.WriteRunMeta(h.logBase, entry)
}

func (h *Handler) failRunLog(projectName, runID, errMsg string) {
	if h.logBase == "" || runID == "" {
		return
	}
	safeName := fsrepo.SanitizeProjectName(projectName)
	entry, err := fsrepo.ReadRunMeta(h.logBase, safeName, runID)
	if err != nil {
		return
	}
	now := time.Now()
	entry.Status = model.RunLogFailed
	entry.EndedAt = &now
	entry.DurationMs = now.Sub(entry.StartedAt).Milliseconds()
	entry.Error = errMsg

	detail := fmt.Sprintf("Time:      %s\nError:\n%s\n", now.Format(time.RFC3339), errMsg)
	fsrepo.WriteRunErrorLog(h.logBase, safeName, runID, detail)
	entry.ErrorLogPath = "error.log"
	fsrepo.WriteRunMeta(h.logBase, entry)
}

func (h *Handler) recordSession(dataDir string, iteration int, startedAt time.Time, totalTokens int) {
	if totalTokens <= 0 {
		return
	}
	sw := fsrepo.GetSessionWriter(dataDir)
	sw.Append(model.Session{
		ID:          uuid.NewString(),
		Stage:       model.StageVision,
		Kind:        model.SessionChat,
		Iteration:   iteration,
		StartedAt:   startedAt,
		EndedAt:     time.Now(),
		TotalTokens: totalTokens,
	})
}
