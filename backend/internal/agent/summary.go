package agent

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/michelroberge/ai-app-factory/backend/internal/model"
)

const summarySystemPrompt = `You are a technical writer summarizing a completed product development iteration.

You will receive the approved artifacts from all pipeline stages (Vision, UX, Architecture, Build Plan, Review).

Produce a concise summary document that captures the essential decisions and outcomes. This summary will be used as context for future enhancement iterations, so focus on what a future AI agent would need to understand to build upon this work.

Output the summary directly (no delimiters needed):

# Iteration Summary: {Product Name} v{version}

## Product Overview
One paragraph capturing the core product idea, target users, and problem solved.

## Key UX Decisions
Bullet points of the most important UX choices (navigation patterns, key screens, interaction model).

## Architecture Summary
Tech stack, major components, API surface, data model highlights.

## Build Strategy
How the work was organized (milestones, key dependencies, risks addressed).

## Review Outcome
Final verdict and any important caveats or recommendations.

## Known Limitations & Future Work
Items explicitly deferred or flagged for future iterations.

Be concise — aim for a document that can be quickly scanned. Avoid repeating full artifact contents; summarize the decisions and rationale.`

// GenerateSummary calls Claude to produce a concise summary of all approved artifacts.
func GenerateSummary(ctx context.Context, artifacts map[model.StageName]string, projectName string, version string) (string, error) {
	var prompt strings.Builder
	prompt.WriteString(fmt.Sprintf("Project: %s, Version: %s\n\n", projectName, version))

	stages := []struct {
		name  model.StageName
		label string
	}{
		{model.StageVision, "Vision"},
		{model.StageUX, "UX Design"},
		{model.StageArchitecture, "Architecture"},
		{model.StageBuild, "Build Plan"},
		{model.StageReview, "Review"},
	}

	for _, s := range stages {
		content := artifacts[s.name]
		if content == "" {
			continue
		}
		prompt.WriteString(fmt.Sprintf("## %s Artifact\n---\n%s\n---\n\n", s.label, content))
	}

	cmd := exec.CommandContext(ctx, "claude",
		"--print",
		"--system-prompt", summarySystemPrompt,
	)
	cmd.Stdin = strings.NewReader(prompt.String())

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("generate summary: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}
