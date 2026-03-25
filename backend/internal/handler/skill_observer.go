package handler

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/michelroberge/paulette/backend/internal/agent"
	"github.com/michelroberge/paulette/backend/internal/model"
	fsrepo "github.com/michelroberge/paulette/backend/internal/repository/fs"
	"github.com/michelroberge/paulette/backend/internal/stream"
)

// SkillObserver collects completed bead data and periodically analyzes
// for emergent patterns that could become reusable skills.
type SkillObserver struct {
	mu        sync.Mutex
	beads     []agent.ObservedBead
	batchSize int
	hostDir   string
	skillRepo *fsrepo.SkillRepo
	run       *stream.Run
	project   *model.Project
}

// NewSkillObserver creates an observer for a bead execution run.
func NewSkillObserver(hostDir string, skillRepo *fsrepo.SkillRepo, run *stream.Run, project *model.Project, batchSize int) *SkillObserver {
	if batchSize <= 0 {
		batchSize = 5
	}
	return &SkillObserver{
		batchSize: batchSize,
		hostDir:   hostDir,
		skillRepo: skillRepo,
		run:       run,
		project:   project,
	}
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

	events, err := agent.ObserveBeads(ctx, batch, existingSkills)
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
			fmt.Sscanf(ev.Content, "%d", &tokens)
		}
	}

	if tokens > 0 {
		o.project.AddStageTokens(model.StageBuild, tokens)
		recordSession(o.hostDir, model.StageBuild, model.SessionSkillObserve, o.project.Iteration, o.run.StartedAt, tokens)
	}

	suggestions, ok := agent.ExtractSkillSuggestions(fullText.String())
	if !ok || len(suggestions) == 0 {
		return
	}

	// Append to observed skills file
	existing, _ := fsrepo.ReadObservedSkills(o.hostDir)
	existing.Suggestions = append(existing.Suggestions, suggestions...)
	fsrepo.WriteObservedSkills(o.hostDir, existing)

	o.run.Emit(agent.StreamEvent{
		Type:    "log",
		Content: fmt.Sprintf("🔍 Observer detected %d potential skill patterns", len(suggestions)),
	})
}
