package agent

import (
	"bufio"
	"context"
	"encoding/json"
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

// StreamSummary calls Claude to produce a concise summary of all approved artifacts,
// streaming chunks as StreamEvents. The channel is closed when generation finishes.
func StreamSummary(ctx context.Context, artifacts map[model.StageName]string, projectName string, version string) (<-chan StreamEvent, error) {
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
		"--output-format", "stream-json",
		"--verbose",
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
		defer close(ch)
		defer cmd.Wait()

		var fullText strings.Builder
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			var event claudeEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue
			}
			switch event.Type {
			case "assistant":
				if event.Message != nil {
					for _, c := range event.Message.Content {
						if c.Type == "text" && c.Text != "" {
							fullText.WriteString(c.Text)
							ch <- StreamEvent{Type: "chunk", Content: c.Text}
						}
					}
				}
			case "result":
				if event.Usage != nil {
					total := event.Usage.InputTokens + event.Usage.OutputTokens
					ch <- StreamEvent{Type: "tokens", Content: fmt.Sprintf("%d", total)}
				}
				if fullText.Len() == 0 && event.Result != "" {
					fullText.WriteString(event.Result)
					ch <- StreamEvent{Type: "chunk", Content: event.Result}
				}
			}
		}

		ch <- StreamEvent{Type: "done", Content: fullText.String()}
	}()

	return ch, nil
}
