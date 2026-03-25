package autopilot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/michelroberge/paulette/backend/internal/agent"
	"github.com/michelroberge/paulette/backend/internal/handler"
	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/repository"
	fsrepo "github.com/michelroberge/paulette/backend/internal/repository/fs"
	"github.com/michelroberge/paulette/backend/internal/stream"
)

var kickoffMessages = map[model.StageName]string{
	model.StageVision:       "Let's start building your product vision. What's the core idea — what problem are you solving and for whom?",
	model.StageUX:           "I've reviewed the approved vision. Let me propose the initial user flows and screen descriptions for this product.",
	model.StageArchitecture: "I've reviewed the vision and UX design. Let me propose the technical architecture — stack, components, APIs, and data models.",
	model.StageBuild:        "I've reviewed all approved artifacts. Let me create a concrete build plan with milestones and tasks.",
}

var suggestionsRe = regexp.MustCompile(`(?s)## Suggested Enhancements\n(.*?)(?:\n## |$)`)

// Orchestrator manages one autonomous goroutine per project.
type Orchestrator struct {
	rootCtx    context.Context
	rootCancel context.CancelFunc
	mu         sync.Mutex
	active     map[string]context.CancelFunc

	registry     repository.RegistryRepo
	artifactRepo repository.ArtifactRepo
	activityRepo repository.ActivityRepo
	runs         *stream.Manager
	chatH        *handler.ChatHandler
	mockH        *handler.MockHandler
	beadH        *handler.BeadHandler
	pipelineH    *handler.PipelineHandler
	enhanceH     *handler.EnhanceHandler
}

func NewOrchestrator(
	registry repository.RegistryRepo,
	artifactRepo repository.ArtifactRepo,
	activityRepo repository.ActivityRepo,
	runs *stream.Manager,
	chatH *handler.ChatHandler,
	mockH *handler.MockHandler,
	beadH *handler.BeadHandler,
	pipelineH *handler.PipelineHandler,
	enhanceH *handler.EnhanceHandler,
) *Orchestrator {
	ctx, cancel := context.WithCancel(context.Background())
	return &Orchestrator{
		rootCtx:      ctx,
		rootCancel:   cancel,
		active:       make(map[string]context.CancelFunc),
		registry:     registry,
		artifactRepo: artifactRepo,
		activityRepo: activityRepo,
		runs:         runs,
		chatH:        chatH,
		mockH:        mockH,
		beadH:        beadH,
		pipelineH:    pipelineH,
		enhanceH:     enhanceH,
	}
}

// StartAll resumes any in-progress operations and starts autonomous goroutines.
// For each project, stale "running" activities are either restarted (mock,
// beads-generate, beads-execute) or marked as failed (chat, summary).
// Call once at server startup in a goroutine.
func (o *Orchestrator) StartAll() {
	projects, err := o.registry.List()
	if err != nil {
		log.Printf("orchestrator: failed to list projects: %v", err)
		return
	}
	for _, p := range projects {
		o.resumeOrClearStaleActivities(&p)
		if p.Autonomous {
			o.Ensure(p.ID)
		}
	}
}

// resumeOrClearStaleActivities handles stale "running" activity entries at startup.
// Restartable operations (mock, beads-generate, beads-execute) are restarted so
// they pick up where they left off. For autonomous projects, stale chat activities
// are cleared so the orchestrator re-evaluates the stage cleanly. Everything else
// is marked as failed so the UI shows what happened.
func (o *Orchestrator) resumeOrClearStaleActivities(p *model.Project) {
	activities, err := o.activityRepo.ReadActivity(p.HostDir)
	if err != nil || len(activities) == 0 {
		return
	}
	for stage, a := range activities {
		if a.Status != "running" {
			continue
		}
		if o.tryResumeStaleActivity(p, stage, a) {
			log.Printf("orchestrator: resumed stale %s/%s for project %s", stage, a.Operation, p.ID)
			continue
		}
		updated := *a
		updated.Status = "failed"
		updated.Error = "server restarted while this operation was running"
		if setErr := o.activityRepo.SetActivity(p.HostDir, stage, &updated); setErr != nil {
			log.Printf("orchestrator: resumeOrClearStaleActivities(%s/%s): %v", p.ID, stage, setErr)
		}
	}
}

// tryResumeStaleActivity attempts to restart a stale operation.
// Returns true if the operation was successfully restarted or cleanly cleared,
// false if it should be marked as failed instead.
func (o *Orchestrator) tryResumeStaleActivity(p *model.Project, stage model.StageName, a *model.StageActivity) bool {
	switch a.Operation {
	case "mock":
		if _, err := o.mockH.StartMockRun(p, ""); err != nil {
			log.Printf("orchestrator: failed to resume mock for %s: %v", p.ID, err)
			return false
		}
		return true

	case "beads-generate":
		if _, err := o.beadH.StartGenerateRun(p); err != nil {
			log.Printf("orchestrator: failed to resume beads-generate for %s: %v", p.ID, err)
			return false
		}
		return true

	case "beads-execute":
		if _, err := o.beadH.StartExecuteRun(p, 2); err != nil {
			log.Printf("orchestrator: failed to resume beads-execute for %s: %v", p.ID, err)
			return false
		}
		return true

	case "chat":
		// For autonomous projects the orchestrator will re-evaluate the stage and
		// re-run if the artifact is still missing. Clear the stale activity so the
		// UI doesn't show a false "running" badge while the orchestrator catches up.
		if p.Autonomous {
			if clearErr := o.activityRepo.ClearActivity(p.HostDir, stage); clearErr != nil {
				log.Printf("orchestrator: failed to clear stale chat activity for %s/%s: %v", p.ID, stage, clearErr)
			}
			return true
		}
		// Non-autonomous chat needs user context to restart; mark as failed.
		return false

	default:
		return false
	}
}

// Ensure starts the orchestrator goroutine for a project if not already running. Idempotent.
func (o *Orchestrator) Ensure(projectID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, exists := o.active[projectID]; exists {
		return
	}
	ctx, cancel := context.WithCancel(o.rootCtx)
	o.active[projectID] = cancel
	go func() {
		defer func() {
			o.mu.Lock()
			delete(o.active, projectID)
			o.mu.Unlock()
		}()
		o.runProject(ctx, projectID)
	}()
}

// Cancel stops the orchestrator goroutine for a project.
func (o *Orchestrator) Cancel(projectID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if cancel, exists := o.active[projectID]; exists {
		cancel()
		delete(o.active, projectID)
	}
}

// CancelAll stops all orchestrator goroutines. Call on server shutdown.
func (o *Orchestrator) CancelAll() {
	o.rootCancel()
	o.mu.Lock()
	o.active = make(map[string]context.CancelFunc)
	o.mu.Unlock()
}

// IsRunning reports whether an orchestrator goroutine is active for a project.
func (o *Orchestrator) IsRunning(projectID string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	_, exists := o.active[projectID]
	return exists
}

// runProject is the per-project autonomous pipeline driver.
func (o *Orchestrator) runProject(ctx context.Context, projectID string) {
	log.Printf("orchestrator[%s]: starting", projectID)
	for {
		if ctx.Err() != nil {
			log.Printf("orchestrator[%s]: cancelled", projectID)
			return
		}

		project, err := o.registry.Get(projectID)
		if err != nil {
			log.Printf("orchestrator[%s]: project not found: %v", projectID, err)
			return
		}
		if !project.Autonomous {
			log.Printf("orchestrator[%s]: autonomous disabled, stopping", projectID)
			return
		}

		log.Printf("orchestrator[%s]: stage=%s", projectID, project.CurrentStage)

		var stageErr error
		switch project.CurrentStage {
		case model.StageVision, model.StageArchitecture:
			stageErr = o.handleSimpleStage(ctx, project, project.CurrentStage)
		case model.StageUX:
			stageErr = o.handleUXStage(ctx, project)
		case model.StageBuild:
			stageErr = o.handleBuildStage(ctx, project)
		case model.StageComplete:
			stageErr = o.handleCompleteStage(ctx, project)
			if stageErr != nil {
				log.Printf("orchestrator[%s]: complete stage error: %v", projectID, stageErr)
			}
			return
		default:
			log.Printf("orchestrator[%s]: unknown stage %s", projectID, project.CurrentStage)
			return
		}

		if stageErr != nil {
			var planErr *agent.PlanLimitError
			if errors.As(stageErr, &planErr) {
				resetAt := parseResetTime(planErr.Message)
				waitDur := time.Until(resetAt)
				if waitDur < time.Minute {
					waitDur = time.Hour // fallback: no parseable reset time, wait 1 hour
				}
				log.Printf("orchestrator[%s]: plan limit reached — waiting %v until %v then retrying", projectID, waitDur.Round(time.Second), resetAt.Format(time.RFC3339))
				select {
				case <-time.After(waitDur):
					log.Printf("orchestrator[%s]: resuming after plan limit wait", projectID)
					continue
				case <-ctx.Done():
					return
				}
			}
			log.Printf("orchestrator[%s]: stage %s error: %v", projectID, project.CurrentStage, stageErr)
			return
		}
	}
}

func (o *Orchestrator) handleSimpleStage(ctx context.Context, project *model.Project, stage model.StageName) error {
	content, _ := o.artifactRepo.ReadWithFallback(project.HostDir, project.Version, stage)
	if strings.TrimSpace(content) == "" {
		msg := kickoffMessages[stage]
		if stage == model.StageVision && project.EnhancementVision != "" {
			msg = "This is an enhancement iteration. Here's what I want to improve: " + project.EnhancementVision
		}
		if err := o.runChatWithBtw(ctx, project, stage, msg); err != nil {
			return fmt.Errorf("chat run: %w", err)
		}
	}

	select {
	case <-time.After(2 * time.Second):
	case <-ctx.Done():
		return ctx.Err()
	}

	return o.pipelineH.ApproveInternal(project.ID)
}

func (o *Orchestrator) handleUXStage(ctx context.Context, project *model.Project) error {
	content, _ := o.artifactRepo.ReadWithFallback(project.HostDir, project.Version, model.StageUX)
	if strings.TrimSpace(content) == "" {
		msg := kickoffMessages[model.StageUX]
		if project.EnhancementVision != "" {
			msg = "This is an enhancement iteration. Here's what I want to improve: " + project.EnhancementVision
		}
		if err := o.runChatWithBtw(ctx, project, model.StageUX, msg); err != nil {
			return fmt.Errorf("ux chat run: %w", err)
		}
	}

	// Generate mock if missing
	mockPath := filepath.Join(project.HostDir, ".paulette", "ux", "mock.html")
	if _, statErr := os.Stat(mockPath); os.IsNotExist(statErr) {
		if b, _ := fsrepo.ReadMockDoc(project.HostDir, project.Version); b == nil {
			run, err := o.startOrJoinMockRun(ctx, project)
			if err != nil {
				return fmt.Errorf("mock run: %w", err)
			}
			if run != nil {
				if err := o.waitForRunDone(ctx, run); err != nil {
					return err
				}
			}
		}
	}

	select {
	case <-time.After(2 * time.Second):
	case <-ctx.Done():
		return ctx.Err()
	}

	return o.pipelineH.ApproveInternal(project.ID)
}

func (o *Orchestrator) handleBuildStage(ctx context.Context, project *model.Project) error {
	content, _ := o.artifactRepo.ReadWithFallback(project.HostDir, project.Version, model.StageBuild)
	if strings.TrimSpace(content) == "" {
		msg := kickoffMessages[model.StageBuild]
		if project.EnhancementVision != "" {
			msg = "This is an enhancement iteration. Here's what I want to improve: " + project.EnhancementVision
		}
		if err := o.runChatWithBtw(ctx, project, model.StageBuild, msg); err != nil {
			return fmt.Errorf("build chat run: %w", err)
		}
	}

	// Generate beads if missing
	graph, _ := fsrepo.ReadBeadGraph(project.HostDir)
	if graph == nil || len(graph.Beads) == 0 {
		run, err := o.startOrJoinGenerateRun(ctx, project)
		if err != nil {
			return fmt.Errorf("beads generate run: %w", err)
		}
		if run != nil {
			if err := o.waitForRunDone(ctx, run); err != nil {
				return err
			}
		}
	}

	// Execute beads
	run, err := o.startOrJoinExecuteRun(ctx, project)
	if err != nil {
		return fmt.Errorf("beads execute run: %w", err)
	}
	if run != nil {
		if err := o.waitForRunDone(ctx, run); err != nil {
			return err
		}
	}

	select {
	case <-time.After(2 * time.Second):
	case <-ctx.Done():
		return ctx.Err()
	}

	return o.pipelineH.ApproveInternal(project.ID)
}

func (o *Orchestrator) handleCompleteStage(ctx context.Context, project *model.Project) error {
	// Wait for summary
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		p, err := o.registry.Get(project.ID)
		if err != nil {
			return err
		}
		if p.SummaryReady {
			project = p
			break
		}
		// Attach to active summary run if present
		if run := o.runs.Active(project.ID, "complete", "summary"); run != nil {
			if err := o.waitForRunDone(ctx, run); err != nil {
				return err
			}
			continue
		}
		select {
		case <-time.After(1 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if project.Iteration >= 10 {
		log.Printf("orchestrator[%s]: max iterations reached", project.ID)
		return nil
	}

	summaryPath := filepath.Join(project.HostDir, ".paulette", "summary.md")
	summaryBytes, err := os.ReadFile(summaryPath)
	if err != nil {
		return fmt.Errorf("read summary: %w", err)
	}

	suggestions := extractSuggestions(string(summaryBytes))
	if suggestions == "" {
		log.Printf("orchestrator[%s]: no suggestions, stopping", project.ID)
		return nil
	}

	select {
	case <-time.After(3 * time.Second):
	case <-ctx.Done():
		return ctx.Err()
	}

	_, err = o.enhanceH.EnhanceInternal(project.ID, suggestions, "minor")
	return err
}

// runChatWithBtw starts (or joins) a chat run and, after it finishes, processes any
// queued /btw messages as follow-up runs in the same stage context.
// Messages received within 10 seconds of each other are combined into one prompt.
func (o *Orchestrator) runChatWithBtw(ctx context.Context, project *model.Project, stage model.StageName, message string) error {
	run, err := o.startOrJoinChatRun(ctx, project, stage, message)
	if err != nil {
		return err
	}
	if run != nil {
		if err := o.waitForRunDone(ctx, run); err != nil {
			return err
		}
	}

	// Process pending btw bursts until the queue is empty.
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		msgs, err := o.activityRepo.ClearBtw(project.HostDir, stage)
		if err != nil || len(msgs) == 0 {
			break
		}
		for _, combined := range combineBtwBurst(msgs, 10*time.Second) {
			run, err := o.startOrJoinChatRun(ctx, project, stage, combined)
			if err != nil {
				return err
			}
			if run != nil {
				if err := o.waitForRunDone(ctx, run); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// combineBtwBurst groups btw messages by temporal proximity and joins each burst.
// Messages within `window` of the previous one are combined into one string.
func combineBtwBurst(msgs []model.BtwMessage, window time.Duration) []string {
	if len(msgs) == 0 {
		return nil
	}
	sort.Slice(msgs, func(i, j int) bool { return msgs[i].SentAt.Before(msgs[j].SentAt) })

	var bursts []string
	var current []string
	last := msgs[0].SentAt

	for _, m := range msgs {
		if m.SentAt.Sub(last) > window && len(current) > 0 {
			bursts = append(bursts, strings.Join(current, "\n"))
			current = current[:0]
		}
		current = append(current, m.Message)
		last = m.SentAt
	}
	if len(current) > 0 {
		bursts = append(bursts, strings.Join(current, "\n"))
	}
	return bursts
}

// startOrJoinChatRun starts a new chat run or attaches to an existing one.
func (o *Orchestrator) startOrJoinChatRun(ctx context.Context, project *model.Project, stage model.StageName, message string) (*stream.Run, error) {
	if existing := o.runs.Active(project.ID, string(stage), "chat"); existing != nil {
		return existing, nil
	}
	run, err := o.chatH.StartChatRun(project, stage, message)
	if err != nil {
		return nil, err
	}
	if run == nil {
		// Race: another goroutine just started it
		return o.runs.Active(project.ID, string(stage), "chat"), nil
	}
	return run, nil
}

func (o *Orchestrator) startOrJoinMockRun(ctx context.Context, project *model.Project) (*stream.Run, error) {
	if existing := o.runs.Active(project.ID, "ux", "mock"); existing != nil {
		return existing, nil
	}
	run, err := o.mockH.StartMockRun(project, "")
	if err != nil {
		return nil, err
	}
	if run == nil {
		return o.runs.Active(project.ID, "ux", "mock"), nil
	}
	return run, nil
}

func (o *Orchestrator) startOrJoinGenerateRun(ctx context.Context, project *model.Project) (*stream.Run, error) {
	if existing := o.runs.Active(project.ID, "build", "beads-generate"); existing != nil {
		return existing, nil
	}
	run, err := o.beadH.StartGenerateRun(project)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return o.runs.Active(project.ID, "build", "beads-generate"), nil
	}
	return run, nil
}

func (o *Orchestrator) startOrJoinExecuteRun(ctx context.Context, project *model.Project) (*stream.Run, error) {
	if existing := o.runs.Active(project.ID, "build", "beads-execute"); existing != nil {
		return existing, nil
	}
	run, err := o.beadH.StartExecuteRun(project, 2)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return o.runs.Active(project.ID, "build", "beads-execute"), nil
	}
	return run, nil
}

// waitForRunDone subscribes to a run and blocks until it finishes or ctx is cancelled.
// Returns agent.ErrPlanLimit (as *agent.PlanLimitError) if Claude hits its usage limit.
func (o *Orchestrator) waitForRunDone(ctx context.Context, run *stream.Run) error {
	ch, _ := run.Subscribe(0)
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return nil // run finished
			}
			if event.Type == "error" {
				return fmt.Errorf("run error: %s", event.Content)
			}
			if event.Type == "plan_limit" {
				return &agent.PlanLimitError{Message: event.Content}
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// parseResetTime tries to extract a future reset time from a Claude CLI plan-limit message.
// Handles ISO 8601 timestamps, "January 2, 2006 at 3:04 PM MST", and bare "3:04 PM MST".
// Returns time.Now().Add(1 hour) as a fallback when no time can be parsed.
func parseResetTime(msg string) time.Time {
	fallback := time.Now().Add(time.Hour)

	// ISO 8601 / RFC3339
	rfc := regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}[^\s,]*`)
	if m := rfc.FindString(msg); m != "" {
		for _, layout := range []string{time.RFC3339, time.RFC3339Nano} {
			if t, err := time.Parse(layout, m); err == nil {
				return t
			}
		}
	}

	// "January 2, 2006 at 3:04 PM MST" or "Jan 2, 2006 at 3:04 PM MST"
	fullRe := regexp.MustCompile(`(?i)((?:January|February|March|April|May|June|July|August|September|October|November|December|Jan|Feb|Mar|Apr|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\s+\d{1,2},?\s+\d{4}\s+(?:at\s+)?\d{1,2}:\d{2}\s+[AP]M(?:\s+[A-Z]{2,5})?)`)
	if m := fullRe.FindString(msg); m != "" {
		m = regexp.MustCompile(`(?i)\bat\b\s*`).ReplaceAllString(m, "")
		for _, layout := range []string{"January 2, 2006 3:04 PM MST", "Jan 2, 2006 3:04 PM MST", "January 2 2006 3:04 PM MST"} {
			if t, err := time.Parse(layout, strings.TrimSpace(m)); err == nil {
				return t
			}
		}
	}

	// Bare time "3:04 PM MST" — assume today, or tomorrow if already past
	bareRe := regexp.MustCompile(`(?i)\b(\d{1,2}:\d{2}\s+[AP]M(?:\s+[A-Z]{2,5})?)\b`)
	if m := bareRe.FindString(msg); m != "" {
		now := time.Now()
		base := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		for _, layout := range []string{"3:04 PM MST", "3:04 PM"} {
			if t, err := time.Parse(layout, strings.TrimSpace(m)); err == nil {
				candidate := base.Add(time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute)
				if candidate.Before(now) {
					candidate = candidate.Add(24 * time.Hour)
				}
				return candidate
			}
		}
	}

	return fallback
}

func extractSuggestions(summary string) string {
	m := suggestionsRe.FindStringSubmatch(summary)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}
