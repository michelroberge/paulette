package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/michelroberge/paulette/backend/internal/model"
)

const (
	existingSkillsHeader = "## Existing Skills\n"
	existingSkillsNone   = "(none)\n"
	existingSkillsFmt    = "- %s (%s): %s [tags: %s]\n"
)

// writeExistingSkills appends the "## Existing Skills" block to b.
func writeExistingSkills(b *strings.Builder, skills []model.Skill) {
	b.WriteString(existingSkillsHeader)
	if len(skills) == 0 {
		b.WriteString(existingSkillsNone)
		return
	}
	for _, s := range skills {
		fmt.Fprintf(b, existingSkillsFmt, s.Name, s.Category, s.Description, strings.Join(s.Tags, ", "))
	}
}

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
// BuildAnalyzeSkillsRequest returns the system prompt and user message for skill analysis.
// Used by non-CLI providers that call provider.Chat directly.
func BuildAnalyzeSkillsRequest(buildPlan, archContent string, existingSkills []model.Skill) (systemPrompt, userMsg string) {
	var prompt strings.Builder
	prompt.WriteString("## Build Plan\n---\n")
	prompt.WriteString(buildPlan)
	prompt.WriteString("\n---\n\n")
	if archContent != "" {
		prompt.WriteString("## Architecture\n---\n")
		prompt.WriteString(archContent)
		prompt.WriteString("\n---\n\n")
	}
	writeExistingSkills(&prompt, existingSkills)
	return skillAnalysisSystemPrompt, prompt.String()
}

// BuildObserveBeadsRequest returns the system prompt and user message for bead observer analysis.
// Used by non-CLI providers that call provider.Chat directly.
func BuildObserveBeadsRequest(beads []ObservedBead, existingSkills []model.Skill) (systemPrompt, userMsg string) {
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
	writeExistingSkills(&prompt, existingSkills)
	return skillObserverSystemPrompt, prompt.String()
}

// ObservedBead captures data from a completed bead for observer analysis.
type ObservedBead struct {
	ID         string
	Title      string
	Tags       []string
	PromptUsed string
	CodeOutput string
}

// skillSuggestionSchema is the JSON Schema for []model.SkillSuggestion.
var skillSuggestionSchema = []byte(`{
  "type": "array",
  "items": {
    "type": "object",
    "required": ["name", "description", "promptTemplate"],
    "properties": {
      "name":           {"type": "string"},
      "description":    {"type": "string"},
      "category":       {"type": "string"},
      "tags":           {"type": "array"},
      "parameters":     {"type": "array"},
      "promptTemplate": {"type": "string"}
    }
  }
}`)

// ExtractSkillSuggestions parses the <!-- SKILLS:START -->...<!-- SKILLS:END --> JSON block.
// Applies repairJSON and schema validation before unmarshalling; returns nil, false on any failure.
// Falls back to finding a JSON array in the text if delimiters are missing.
func ExtractSkillSuggestions(text string) ([]model.SkillSuggestion, bool) {
	const start = "<!-- SKILLS:START -->"
	const end = "<!-- SKILLS:END -->"

	var raw []byte

	si := strings.Index(text, start)
	ei := strings.Index(text, end)
	if si >= 0 && ei > si {
		raw = []byte(strings.TrimSpace(text[si+len(start) : ei]))
	} else {
		// Fallback: try to find a JSON array in a code block or directly
		raw = extractJSONArray(text)
		if raw == nil {
			return nil, false
		}
	}

	raw = repairJSON(raw)
	if validateJSON(raw, skillSuggestionSchema) != nil {
		// Try without validation — some models produce slightly non-conformant JSON
		var suggestions []model.SkillSuggestion
		if err := json.Unmarshal(raw, &suggestions); err != nil {
			return nil, false
		}
		return suggestions, true
	}

	var suggestions []model.SkillSuggestion
	if err := json.Unmarshal(raw, &suggestions); err != nil {
		return nil, false
	}
	return suggestions, true
}

// extractJSONArray tries to find a JSON array in the text, possibly inside a code block.
func extractJSONArray(text string) []byte {
	// Try ```json ... ``` code block first
	jsonStart := strings.Index(text, "```json")
	if jsonStart >= 0 {
		afterStart := text[jsonStart+7:]
		jsonEnd := strings.Index(afterStart, "```")
		if jsonEnd > 0 {
			candidate := strings.TrimSpace(afterStart[:jsonEnd])
			if len(candidate) > 2 && candidate[0] == '[' {
				return []byte(candidate)
			}
		}
	}

	// Try ``` ... ``` code block
	codeStart := strings.Index(text, "```")
	if codeStart >= 0 {
		afterStart := text[codeStart+3:]
		// Skip language identifier on same line
		nlIdx := strings.Index(afterStart, "\n")
		if nlIdx >= 0 {
			afterStart = afterStart[nlIdx+1:]
		}
		codeEnd := strings.Index(afterStart, "```")
		if codeEnd > 0 {
			candidate := strings.TrimSpace(afterStart[:codeEnd])
			if len(candidate) > 2 && candidate[0] == '[' {
				return []byte(candidate)
			}
		}
	}

	// Last resort: find first [ and last ] in the text
	firstBracket := strings.Index(text, "[")
	lastBracket := strings.LastIndex(text, "]")
	if firstBracket >= 0 && lastBracket > firstBracket {
		candidate := strings.TrimSpace(text[firstBracket : lastBracket+1])
		if json.Valid([]byte(candidate)) {
			return []byte(candidate)
		}
	}

	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n... (truncated)"
}
