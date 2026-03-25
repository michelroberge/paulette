package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/michelroberge/paulette/backend/internal/model"
)

const skillAnalysisSystemPrompt = `You are a senior software architect analyzing a build plan and architecture to identify reusable patterns that can become "skills" — parameterized prompt templates for code generation.

A skill is a repeatable pattern that appears across multiple tasks in this build plan, or that is commonly needed across software projects. Examples:
- "Create a REST CRUD endpoint" (parameterized by entity name, fields, database)
- "Set up authentication middleware" (parameterized by strategy: JWT, session, OAuth)
- "Create a React form with validation" (parameterized by fields, validation rules)
- "Configure CI/CD pipeline" (parameterized by platform, test commands, deploy target)

For each identified skill, produce:
1. A clear name and description
2. A category (backend, frontend, devops, testing, database, infrastructure)
3. Relevant tags for matching
4. Named parameters with descriptions and optional defaults
5. A prompt template in Markdown using {{parameter_name}} placeholders

IMPORTANT: Check the existing skills list provided below. Do NOT suggest skills that duplicate existing ones.

OUTPUT FORMAT: Return a JSON array wrapped in delimiters:
<!-- SKILLS:START -->
[
  {
    "name": "Skill Name",
    "description": "What this skill does",
    "category": "backend",
    "tags": ["api", "crud"],
    "parameters": [
      {"name": "entityName", "description": "Name of the entity", "default": ""}
    ],
    "promptTemplate": "Create a REST API endpoint for {{entityName}}..."
  }
]
<!-- SKILLS:END -->

Identify 3-8 skills. Focus on patterns that appear multiple times in the build plan or are universally useful.`

const skillObserverSystemPrompt = `You are analyzing completed code generation outputs to identify emergent patterns — repeated code structures, similar prompt patterns, or shared utilities that appeared across multiple tasks.

For each pattern you identify, suggest a reusable "skill" (parameterized prompt template) that could accelerate similar work in future projects.

Focus on:
- Code that was generated with similar structure across multiple beads
- Utility functions or helpers that were recreated independently
- Configuration patterns that repeat
- Testing patterns that could be templated

Do NOT suggest patterns that duplicate existing skills listed below.

OUTPUT FORMAT: Return a JSON array wrapped in delimiters:
<!-- SKILLS:START -->
[
  {
    "name": "Skill Name",
    "description": "What this skill does",
    "category": "backend",
    "tags": ["api", "crud"],
    "parameters": [
      {"name": "entityName", "description": "Name of the entity", "default": ""}
    ],
    "promptTemplate": "Create a REST API endpoint for {{entityName}}...",
    "sourceBeads": ["bead-id-1", "bead-id-2"]
  }
]
<!-- SKILLS:END -->`

// AnalyzeSkills calls Claude to analyze a build plan for reusable skill patterns.
func AnalyzeSkills(ctx context.Context, buildPlan, archContent string, existingSkills []model.Skill) (<-chan StreamEvent, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)

	var prompt strings.Builder
	prompt.WriteString("## Build Plan\n---\n")
	prompt.WriteString(buildPlan)
	prompt.WriteString("\n---\n\n")

	if archContent != "" {
		prompt.WriteString("## Architecture\n---\n")
		prompt.WriteString(archContent)
		prompt.WriteString("\n---\n\n")
	}

	prompt.WriteString("## Existing Skills\n")
	if len(existingSkills) == 0 {
		prompt.WriteString("(none)\n")
	} else {
		for _, s := range existingSkills {
			prompt.WriteString(fmt.Sprintf("- %s (%s): %s [tags: %s]\n", s.Name, s.Category, s.Description, strings.Join(s.Tags, ", ")))
		}
	}

	return runSkillAgent(ctx, cancel, skillAnalysisSystemPrompt, prompt.String())
}

// ObserveBeads calls Claude to analyze completed bead outputs for emergent patterns.
func ObserveBeads(ctx context.Context, beads []ObservedBead, existingSkills []model.Skill) (<-chan StreamEvent, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)

	var prompt strings.Builder
	prompt.WriteString("## Completed Bead Outputs\n\n")
	for _, b := range beads {
		prompt.WriteString(fmt.Sprintf("### Bead: %s (ID: %s)\n", b.Title, b.ID))
		prompt.WriteString(fmt.Sprintf("Tags: %s\n", strings.Join(b.Tags, ", ")))
		if b.PromptUsed != "" {
			prompt.WriteString(fmt.Sprintf("Prompt used:\n```\n%s\n```\n", truncate(b.PromptUsed, 1000)))
		}
		if b.CodeOutput != "" {
			prompt.WriteString(fmt.Sprintf("Generated code (summary):\n```\n%s\n```\n", truncate(b.CodeOutput, 2000)))
		}
		prompt.WriteString("\n")
	}

	prompt.WriteString("## Existing Skills\n")
	if len(existingSkills) == 0 {
		prompt.WriteString("(none)\n")
	} else {
		for _, s := range existingSkills {
			prompt.WriteString(fmt.Sprintf("- %s (%s): %s [tags: %s]\n", s.Name, s.Category, s.Description, strings.Join(s.Tags, ", ")))
		}
	}

	return runSkillAgent(ctx, cancel, skillObserverSystemPrompt, prompt.String())
}

// ObservedBead captures data from a completed bead for observer analysis.
type ObservedBead struct {
	ID         string
	Title      string
	Tags       []string
	PromptUsed string
	CodeOutput string
}

func runSkillAgent(ctx context.Context, cancel context.CancelFunc, systemPrompt, prompt string) (<-chan StreamEvent, error) {
	cmd := exec.CommandContext(ctx, claudeBin,
		"--print",
		"--output-format", "stream-json",
		"--verbose",
		"--include-partial-messages",
		"--model", "claude-sonnet-4-6",
		"--system-prompt", systemPrompt,
	)
	cmd.Stdin = strings.NewReader(prompt)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
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

// ExtractSkillSuggestions parses the <!-- SKILLS:START -->...<!-- SKILLS:END --> JSON block.
func ExtractSkillSuggestions(text string) ([]model.SkillSuggestion, bool) {
	const start = "<!-- SKILLS:START -->"
	const end = "<!-- SKILLS:END -->"

	si := strings.Index(text, start)
	ei := strings.Index(text, end)
	if si < 0 || ei < 0 || ei <= si {
		return nil, false
	}

	jsonStr := strings.TrimSpace(text[si+len(start) : ei])
	var suggestions []model.SkillSuggestion
	if err := json.Unmarshal([]byte(jsonStr), &suggestions); err != nil {
		return nil, false
	}
	return suggestions, true
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n... (truncated)"
}
