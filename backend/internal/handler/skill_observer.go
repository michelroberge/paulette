package handler

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/michelroberge/paulette/backend/internal/agent"
	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/provider"
	fsrepo "github.com/michelroberge/paulette/backend/internal/repository/fs"
	"github.com/michelroberge/paulette/backend/internal/stream"
)

// SkillObserver collects completed bead data and periodically analyzes
// for emergent patterns that could become reusable skills.
type SkillObserver struct {
	mu               sync.Mutex
	beads            []agent.ObservedBead
	batchSize        int
	dataDir          string
	hostDir          string // git repo root — for provider config and ProjectDir
	skillRepo        *fsrepo.SkillRepo
	run              *stream.Run
	project          *model.Project
	providerRegistry *provider.Registry
	stageConfig      *provider.StageConfigStore
}

// NewSkillObserver creates an observer for a bead execution run.
func NewSkillObserver(dataDir, hostDir string, skillRepo *fsrepo.SkillRepo, run *stream.Run, project *model.Project, batchSize int, providerRegistry *provider.Registry, stageConfig *provider.StageConfigStore) *SkillObserver {
	if batchSize <= 0 {
		batchSize = 5
	}
	return &SkillObserver{
		batchSize:        batchSize,
		dataDir:          dataDir,
		hostDir:          hostDir,
		skillRepo:        skillRepo,
		run:              run,
		project:          project,
		providerRegistry: providerRegistry,
		stageConfig:      stageConfig,
	}
}

// resolveProvider returns the Provider and model ID to use for the Build stage,
// falling back to ClaudeCLI when no registry is configured.
func (o *SkillObserver) resolveProvider() (provider.Provider, string, *provider.StageAssignment, error) {
	if o.providerRegistry != nil {
		return o.providerRegistry.ResolveForStageWithSettings(o.project.ID, model.StageBuild, o.stageConfig, o.hostDir)
	}
	return provider.NewClaudeCLIProvider(), provider.FallbackModel(model.StageBuild), nil, nil
}

// RecordBead adds a completed bead to the observer's buffer.
// If the buffer reaches batchSize, triggers async analysis.
func (o *SkillObserver) RecordBead(bead model.Bead, promptUsed, codeOutput string) {
	o.mu.Lock()
	o.beads = append(o.beads, agent.ObservedBead{
		ID:         bead.ID,
		Title:      bead.Title,
		Tags:       bead.Tags,
		PromptUsed: promptUsed,
		CodeOutput: codeOutput,
	})
	count := len(o.beads)
	o.mu.Unlock()

	if count >= o.batchSize {
		go o.analyzeBatch()
	}
}

// Flush triggers analysis on any remaining beads at end of execution.
func (o *SkillObserver) Flush() {
	o.mu.Lock()
	if len(o.beads) == 0 {
		o.mu.Unlock()
		return
	}
	o.mu.Unlock()
	o.analyzeBatch()
}

func (o *SkillObserver) analyzeBatch() {
	o.mu.Lock()
	if len(o.beads) == 0 {
		o.mu.Unlock()
		return
	}
	batch := make([]agent.ObservedBead, len(o.beads))
	copy(batch, o.beads)
	o.beads = o.beads[:0] // clear buffer
	o.mu.Unlock()

	existingSkills, _ := o.skillRepo.List()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	prov, modelID, sa, err := o.resolveProvider()
	if err != nil {
		log.Printf("skill observer: resolve provider failed: %v", err)
		return
	}

	systemPrompt, userMsg := agent.BuildObserveBeadsRequest(batch, existingSkills)
	saTemp, saNumCtx, saStream := sa.Fields()
	events, err := prov.Chat(ctx, provider.ChatRequest{
		Model:        modelID,
		SystemPrompt: systemPrompt,
		UserMessage:  userMsg,
		ProjectDir:   o.hostDir,
		Stage:        string(model.StageBuild),
		Temperature:  saTemp,
		NumCtx:       saNumCtx,
		Stream:       saStream,
	})
	if err != nil {
		log.Printf("skill observer: analysis failed: %v", err)
		return
	}

	var fullText strings.Builder
	var tokens int
	for ev := range events {
		if ev.Type == "chunk" {
			fullText.WriteString(ev.Content)
		} else if ev.Type == "tokens" {
			tokens += ev.Tokens
		}
	}

	if tokens > 0 {
		o.project.AddStageTokens(model.StageBuild, tokens)
		recordSession(o.dataDir, model.StageBuild, model.SessionSkillObserve, o.project.Iteration, o.run.StartedAt, tokens)
	}

	suggestions, ok := agent.ExtractSkillSuggestions(fullText.String())
	if !ok || len(suggestions) == 0 {
		return
	}

	// Append to observed skills file
	existing, _ := fsrepo.ReadObservedSkills(o.dataDir)
	existing.Suggestions = append(existing.Suggestions, suggestions...)
	fsrepo.WriteObservedSkills(o.dataDir, existing)

	o.run.Emit(agent.StreamEvent{
		Type:    "log",
		Content: fmt.Sprintf("🔍 Observer detected %d potential skill patterns", len(suggestions)),
	})
}
