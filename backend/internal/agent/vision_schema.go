package agent

import (
	"encoding/json"
	"strings"
)

// VisionDraftSchemaJSON is a JSON Schema matching VisionDraftOutput. Sent via
// Ollama's /api/chat "format" parameter to force the model's output to match
// the schema at the inference layer — dramatically improves reliability on
// small models (e.g. llama3.2:1b) that otherwise drift away from format rules.
//
// Ollama requires the schema to be a JSON object value, not a string. Callers
// pass it as json.RawMessage so the provider can embed it directly.
var VisionDraftSchemaJSON = json.RawMessage(`{
  "type": "object",
  "properties": {
    "mode":       { "type": "string", "enum": ["draft", "refine", "question"] },
    "discussion": { "type": "string" },
    "sections": {
      "type": "object",
      "properties": {
        "problem_statement":           { "type": "string" },
        "target_users":                { "type": "string" },
        "core_features":               { "type": "string" },
        "user_experience":             { "type": "string" },
        "success_metrics":             { "type": "string" },
        "constraints_and_assumptions": { "type": "string" },
        "out_of_scope_v1":             { "type": "string" }
      },
      "required": [
        "problem_statement","target_users","core_features","user_experience",
        "success_metrics","constraints_and_assumptions","out_of_scope_v1"
      ]
    },
    "questions_for_user": { "type": "array", "items": { "type": "string" } }
  },
  "required": ["mode","discussion"]
}`)

// VisionCritiqueSchemaJSON mirrors VisionCritiqueOutput.
var VisionCritiqueSchemaJSON = json.RawMessage(`{
  "type": "object",
  "properties": {
    "approved": { "type": "boolean" },
    "issues":   { "type": "array", "items": { "type": "string" } },
    "revised_sections": {
      "type": "object",
      "properties": {
        "problem_statement":           { "type": "string" },
        "target_users":                { "type": "string" },
        "core_features":               { "type": "string" },
        "user_experience":             { "type": "string" },
        "success_metrics":             { "type": "string" },
        "constraints_and_assumptions": { "type": "string" },
        "out_of_scope_v1":             { "type": "string" }
      }
    }
  },
  "required": ["approved"]
}`)

// VisionSections holds the seven required sections of the Vision artifact.
// JSON tags are the snake_case keys the LLM is instructed to produce.
type VisionSections struct {
	ProblemStatement       string `json:"problem_statement"`
	TargetUsers            string `json:"target_users"`
	CoreFeatures           string `json:"core_features"`
	UserExperience         string `json:"user_experience"`
	SuccessMetrics         string `json:"success_metrics"`
	ConstraintsAssumptions string `json:"constraints_and_assumptions"`
	OutOfScope             string `json:"out_of_scope_v1"`
}

// VisionDraftOutput is the structured output returned by the draft phase.
type VisionDraftOutput struct {
	Mode             string          `json:"mode"`
	Discussion       string          `json:"discussion"`
	Sections         *VisionSections `json:"sections,omitempty"`
	QuestionsForUser []string        `json:"questions_for_user,omitempty"`
}

// VisionCritiqueOutput is the structured output returned by the critique phase.
type VisionCritiqueOutput struct {
	Approved        bool            `json:"approved"`
	Issues          []string        `json:"issues,omitempty"`
	RevisedSections *VisionSections `json:"revised_sections,omitempty"`
}

// RenderVisionMarkdown deterministically converts VisionSections into the
// seven-heading markdown artifact format the rest of the pipeline expects.
// Empty sections are rendered with a placeholder so the structure is stable.
func RenderVisionMarkdown(s VisionSections) string {
	var b strings.Builder
	b.WriteString("# Product Vision\n\n")
	writeVisionSection(&b, "Problem Statement", s.ProblemStatement)
	writeVisionSection(&b, "Target Users", s.TargetUsers)
	writeVisionSection(&b, "Core Features", s.CoreFeatures)
	writeVisionSection(&b, "User Experience", s.UserExperience)
	writeVisionSection(&b, "Success Metrics", s.SuccessMetrics)
	writeVisionSection(&b, "Constraints & Assumptions", s.ConstraintsAssumptions)
	writeVisionSection(&b, "Out of Scope (V1)", s.OutOfScope)
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func writeVisionSection(b *strings.Builder, heading, body string) {
	b.WriteString("## ")
	b.WriteString(heading)
	b.WriteString("\n\n")
	body = strings.TrimSpace(body)
	if body == "" {
		b.WriteString("_To be determined._\n\n")
		return
	}
	b.WriteString(body)
	b.WriteString("\n\n")
}

// HasAllSections reports whether every required section has non-whitespace content.
func (s VisionSections) HasAllSections() bool {
	return strings.TrimSpace(s.ProblemStatement) != "" &&
		strings.TrimSpace(s.TargetUsers) != "" &&
		strings.TrimSpace(s.CoreFeatures) != "" &&
		strings.TrimSpace(s.UserExperience) != "" &&
		strings.TrimSpace(s.SuccessMetrics) != "" &&
		strings.TrimSpace(s.ConstraintsAssumptions) != "" &&
		strings.TrimSpace(s.OutOfScope) != ""
}

// MissingSections returns the human-readable names of sections that are blank.
func (s VisionSections) MissingSections() []string {
	checks := []struct {
		name string
		val  string
	}{
		{"Problem Statement", s.ProblemStatement},
		{"Target Users", s.TargetUsers},
		{"Core Features", s.CoreFeatures},
		{"User Experience", s.UserExperience},
		{"Success Metrics", s.SuccessMetrics},
		{"Constraints & Assumptions", s.ConstraintsAssumptions},
		{"Out of Scope (V1)", s.OutOfScope},
	}
	var missing []string
	for _, c := range checks {
		if strings.TrimSpace(c.val) == "" {
			missing = append(missing, c.name)
		}
	}
	return missing
}

// mergeVisionSections returns a VisionSections where empty fields in primary
// are filled from fallback. Used so a partially-valid repair still improves on
// the original draft instead of replacing good content with blanks.
func mergeVisionSections(primary, fallback VisionSections) VisionSections {
	pick := func(a, b string) string {
		if strings.TrimSpace(a) != "" {
			return a
		}
		return b
	}
	return VisionSections{
		ProblemStatement:       pick(primary.ProblemStatement, fallback.ProblemStatement),
		TargetUsers:            pick(primary.TargetUsers, fallback.TargetUsers),
		CoreFeatures:           pick(primary.CoreFeatures, fallback.CoreFeatures),
		UserExperience:         pick(primary.UserExperience, fallback.UserExperience),
		SuccessMetrics:         pick(primary.SuccessMetrics, fallback.SuccessMetrics),
		ConstraintsAssumptions: pick(primary.ConstraintsAssumptions, fallback.ConstraintsAssumptions),
		OutOfScope:             pick(primary.OutOfScope, fallback.OutOfScope),
	}
}
