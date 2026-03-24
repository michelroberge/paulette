package autopilot

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

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

// StartAll resumes autonomous goroutines for all in-progress autonomous projects.
// Also marks any stale "running" activities as failed (the process died mid-run).
// Call once at server startup in a goroutine.
func (o *Orchestrator) StartAll() {
	projects, err := o.registry.List()
	if err != nil {
		log.Printf("orchestrator: failed to list projects: %v", err)
		return
	}
	for _, p := range projects {
		o.clearStaleActivities(&p)
		if p.Autonomous {
			o.Ensure(p.ID)
		}
	}
}

// clearStaleActivities marks any "running" activity entries as failed.
// Called at startup because the process that set them is no longer alive.
func (o *Orchestrator) clearStaleActivities(p *model.Project) {
	activities, err := o.activityRepo.ReadActivity(p.HostDir)
	if err != nil || len(activities) == 0 {
		return
	}
	for stage, a := range activities {
		if a.Status == "running" {
			updated := *a
			updated.Status = "failed"
			updated.Error = "server restarted while this operation was running"
			if setErr := o.activityRepo.SetActivity(p.HostDir, stage, &updated); setErr != nil {
				log.Printf("orchestrator: clearStaleActivities(%s/%s): %v", p.ID, stage, setErr)
			}
		}
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
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func extractSuggestions(summary string) string {
	m := suggestionsRe.FindStringSubmatch(summary)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}
