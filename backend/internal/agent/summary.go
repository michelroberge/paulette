package agent

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/michelroberge/paulette/backend/internal/model"
)

const summarySystemPrompt = `You are a technical writer summarizing a completed product development iteration.

You will receive the approved artifacts from all pipeline stages (Vision, UX, Architecture, Build Plan).

Produce a concise summary document that captures the essential decisions and outcomes. This summary will be used as context for future enhancement iterations, so focus on what a future AI agent would need to understand to build upon this work.

OUTPUT FORMAT: Wrap your entire response in <!-- RESPONSE:START -->...<!-- RESPONSE:END --> and put all content in <discussion>...</discussion>:
<!-- RESPONSE:START -->
<discussion>
...summary here...
</discussion>
<!-- RESPONSE:END -->

Output the summary inside the XML envelope:

# Iteration Summary: {Product Name} v{version}

## Product Overview
One paragraph capturing the core product idea, target users, and problem solved.

## Key UX Decisions
Bullet points of the most important UX choices (navigation patterns, key screens, interaction model).

## Architecture Summary
Tech stack, major components, API surface, data model highlights.

## Build Strategy
How the work was organized (milestones, key dependencies, risks addressed).

## Suggested Enhancements
Concrete, actionable improvements ordered by impact. For each:
- A one-line title
- Brief description of what it adds or improves
Aim for 3-5 suggestions. Think about: missing features from the vision, UX gaps, architectural improvements, performance optimizations, security hardening, and developer experience improvements.

## Known Limitations
Items explicitly deferred or flagged as current limitations.

Be concise — aim for a document that can be quickly scanned. Avoid repeating full artifact contents; summarize the decisions and rationale.`

// BuildStreamSummaryRequest returns the system prompt and user message for summary generation.
// Used by non-CLI providers that call provider.Chat directly.
// maxContextChars limits the total user message size; 0 means no limit.
func BuildStreamSummaryRequest(artifacts map[model.StageName]string, projectName, version string, maxContextChars int) (systemPrompt, userMsg string) {
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
	}

	// When a context budget is set, divide it equally among present artifacts.
	perArtifactBudget := 0
	if maxContextChars > 0 {
		count := 0
		for _, s := range stages {
			if artifacts[s.name] != "" {
				count++
			}
		}
		if count > 0 {
			perArtifactBudget = maxContextChars / count
		}
	}

	for _, s := range stages {
		content := artifacts[s.name]
		if content == "" {
			continue
		}
		if perArtifactBudget > 0 {
			content = TruncateArtifact(content, perArtifactBudget, 8)
		}
		prompt.WriteString(fmt.Sprintf("## %s Artifact\n---\n%s\n---\n\n", s.label, content))
	}
	return summarySystemPrompt, prompt.String()
}

// StreamSummary calls Claude to produce a concise summary of all approved artifacts,
// streaming chunks as StreamEvents. The channel is closed when generation finishes.
func StreamSummary(ctx context.Context, artifacts map[model.StageName]string, projectName string, version string) (<-chan StreamEvent, error) {
	// Guard against claude CLI hanging forever
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)

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
	}

	for _, s := range stages {
		content := artifacts[s.name]
		if content == "" {
			continue
		}
		prompt.WriteString(fmt.Sprintf("## %s Artifact\n---\n%s\n---\n\n", s.label, content))
	}

	cmd := exec.CommandContext(ctx, claudeBin,
		"--print",
		"--output-format", "stream-json",
		"--verbose",
		"--include-partial-messages",
		"--model", "claude-sonnet-4-6",
		"--system-prompt", summarySystemPrompt,
	)
	cmd.Stdin = strings.NewReader(prompt.String())

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start claude: %w", err)
	}

	ch := make(chan StreamEvent, 64)

	go func() {
		defer cancel()
		defer close(ch)
		defer cmd.Wait()

		var fullText strings.Builder
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		processStreamEvents(scanner, &fullText, ch, streamOptions{tokenField: "content"})

		summary := ParseResponse(fullText.String()).Discussion
		if summary == "" {
			summary = strings.TrimSpace(fullText.String())
		}
		ch <- StreamEvent{Type: "done", Content: summary}
	}()

	return ch, nil
}
