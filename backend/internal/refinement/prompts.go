package refinement

import (
	"fmt"
	"log"
	"strings"

	"github.com/michelroberge/paulette/backend/internal/promptfiles"
)

// PromptSet defines the micro-prompts used by the refinement controller.
// Each method returns (systemPrompt, userMessage). Implement per-stage.
type PromptSet interface {
	Summarize(input string) (string, string)
	GenerateQuestions(state *LoopState) (string, string)
	ExtractFacts(summary, answer string) (string, string)
	NormalizeAnswer(summary, question, answer string) (string, string)
	ClassifyFact(fact string, sections []SectionName) (string, string)
	MergeFact(summary, existingContent, fact string) (string, string)
	ScoreSection(name SectionName, content string) (string, string)
	EvaluateQuestions(state *LoopState, questions []Question) (string, string)
	RewriteQuestion(state *LoopState, question string) (string, string)
	CoherenceCheck(state *LoopState, newFacts []string) (string, string)
	TensionCheck(state *LoopState) (string, string)
	FindGaps(state *LoopState) (string, string)
	SimulateAnswer(state *LoopState, question string) (string, string)
	Critique(state *LoopState) (string, string)
	Synthesize(state *LoopState) (string, string)
	Sections() []SectionName
}

// VisionPromptSet implements PromptSet for the Vision stage.
// Prompts are designed for 1B-7B models: single-purpose, constrained output.
type VisionPromptSet struct{}

func (VisionPromptSet) Sections() []SectionName { return VisionSections }

func (VisionPromptSet) Summarize(input string) (string, string) {
	sys := `You summarize product ideas. Output exactly 2 sentences. State only what is explicitly said. No assumptions.`
	usr := fmt.Sprintf("Summarize this product idea:\n\n%s", input)
	return sys, usr
}

func (VisionPromptSet) GenerateQuestions(state *LoopState) (string, string) {
	sys := `You identify missing information in product ideas. List up to 5 critical unknowns as numbered questions (1. 2. 3. etc).
Only ask about: users, problem, constraints, success criteria, features.
Do not repeat information already known.`

	var known strings.Builder
	if len(state.KnownFacts) > 0 {
		known.WriteString("\n\nAlready known:\n")
		for _, f := range state.KnownFacts {
			known.WriteString("- " + f + "\n")
		}
	}

	usr := fmt.Sprintf("Product idea: %s%s", state.IdeaSummary, known.String())
	return sys, usr
}

func (VisionPromptSet) ExtractFacts(summary, answer string) (string, string) {
	sys := `Extract structured items from the text about a product idea.

Rules:
- Each item must be a meaningful unit (not split into individual words)
- Preserve original phrasing — do not invent examples, placeholder values, or sample data
- Extract only what is explicitly stated in the text
- Do not break compound concepts
- If the input is a list, return each list item
- Do not wrap output in code blocks

Format:
- One item per line starting with "- "
- Maximum 5 items`
	usr := fmt.Sprintf("Product context: %s\n\nText to extract from:\n%s", summary, answer)
	return sys, usr
}

func (VisionPromptSet) NormalizeAnswer(summary, question, answer string) (string, string) {
	sys := `Rewrite the user's answer as explicit, self-contained statements.

Rules:
- Do NOT add any information not present in the answer.
- Do NOT invent details, examples, or specifics.
- Resolve vague references (e.g. "like I said") only if the meaning is obvious from the question.
- If the answer is already clear, return it unchanged.
- Output only the rewritten answer, no explanation.`

	usr := fmt.Sprintf("Project summary: %s\nQuestion asked: %s\nUser's answer: %s\n\nRewrite:", summary, question, answer)
	return sys, usr
}

func (VisionPromptSet) ClassifyFact(fact string, sections []SectionName) (string, string) {
	names := make([]string, len(sections))
	for i, s := range sections {
		names[i] = string(s)
	}
	sys := fmt.Sprintf(`Classify this fact into exactly one category.
Reply with ONLY the category name, nothing else.
Categories: %s`, strings.Join(names, ", "))
	usr := fact
	return sys, usr
}

func (VisionPromptSet) MergeFact(summary, existingContent, fact string) (string, string) {
	sys := fmt.Sprintf(`You merge new information into existing text about the following product idea: %s

Keep all existing content. Add the new fact naturally.
Return ONLY the updated text. Do not add unrelated information.`, summary)

	if strings.TrimSpace(existingContent) == "" {
		usr := fmt.Sprintf("New fact to start this section:\n%s", fact)
		return sys, usr
	}
	usr := fmt.Sprintf("Existing content:\n%s\n\nNew fact to add:\n%s", existingContent, fact)
	return sys, usr
}

func (VisionPromptSet) ScoreSection(name SectionName, content string) (string, string) {
	sys := `Rate the completeness of this product vision section from 0.0 to 1.0.
Consider: Is it clear? Specific? Actionable?
Reply with ONLY a number between 0.0 and 1.0, nothing else.`

	if strings.TrimSpace(content) == "" {
		usr := fmt.Sprintf("Section \"%s\":\n(empty - no content yet)", name)
		return sys, usr
	}
	usr := fmt.Sprintf("Section \"%s\":\n%s", name, content)
	return sys, usr
}

func (VisionPromptSet) EvaluateQuestions(state *LoopState, questions []Question) (string, string) {
	sys := `Score each question for the current product definition.
For each question, provide relevance (0-1), novelty (0-1), and impact (0-1).
Return a JSON array only, one object per question, in the same order.
Example: [{"relevance": 0.8, "novelty": 0.5, "impact": 0.9}]`

	var known strings.Builder
	if len(state.KnownFacts) > 0 {
		known.WriteString("\nKnown facts:\n")
		for _, f := range state.KnownFacts {
			known.WriteString("- " + f + "\n")
		}
	}
	var qs strings.Builder
	for i, q := range questions {
		qs.WriteString(fmt.Sprintf("%d. %s\n", i+1, q.Text))
	}
	usr := fmt.Sprintf("Product summary: %s%s\nQuestions to evaluate:\n%s", state.IdeaSummary, known.String(), qs.String())
	return sys, usr
}

func (VisionPromptSet) RewriteQuestion(state *LoopState, question string) (string, string) {
	sys := `Rewrite this question to be more specific and actionable.
Return ONLY the rewritten question, nothing else.`

	usr := fmt.Sprintf("Product summary: %s\n\nQuestion: %s", state.IdeaSummary, question)
	return sys, usr
}

func (VisionPromptSet) CoherenceCheck(state *LoopState, newFacts []string) (string, string) {
	sys := `Check if newly added information is consistent with the existing product definition.
Look for contradictions, conflicting statements, or incompatible claims.
If everything is consistent, reply with "COHERENT".
If not, list each inconsistency as a numbered item. For each, state what contradicts what.`

	var sb strings.Builder
	for _, s := range VisionSections {
		content := state.Sections[s]
		if strings.TrimSpace(content) == "" {
			content = "(empty)"
		}
		sb.WriteString(fmt.Sprintf("\n## %s\n%s\n", s, content))
	}
	sb.WriteString("\nNewly added facts:\n")
	for _, f := range newFacts {
		sb.WriteString("- " + f + "\n")
	}
	return sys, sb.String()
}

func (VisionPromptSet) TensionCheck(state *LoopState) (string, string) {
	sys := `You are a critical advisor. Challenge this product vision constructively.
Look for:
- Unrealistic assumptions
- Market risks or blind spots
- Misaligned metrics and goals
- Scope that is too broad or too narrow

List up to 2 hard questions that would strengthen the vision.
These should challenge, not just clarify.
If the vision is already well-challenged, reply with "NONE".`

	var sb strings.Builder
	for _, s := range VisionSections {
		content := state.Sections[s]
		if strings.TrimSpace(content) == "" {
			content = "(empty)"
		}
		sb.WriteString(fmt.Sprintf("\n## %s\n%s\n", s, content))
	}
	return sys, sb.String()
}

func (VisionPromptSet) FindGaps(state *LoopState) (string, string) {
	sys := `Given this product definition, list up to 3 missing or unclear areas.
Be specific. One item per line, numbered 1. 2. 3.
If the definition is complete, reply with "NONE".`

	var sb strings.Builder
	for _, s := range VisionSections {
		content := state.Sections[s]
		if strings.TrimSpace(content) == "" {
			content = "(empty)"
		}
		sb.WriteString(fmt.Sprintf("\n## %s\n%s\n", s, content))
	}
	return sys, sb.String()
}

func (VisionPromptSet) SimulateAnswer(state *LoopState, question string) (string, string) {
	sys := `You are a product owner answering a question about your product idea.
Answer based ONLY on what is known. If unsure, give a reasonable minimal answer.
Keep your answer to 2-3 sentences.`

	var known strings.Builder
	if len(state.KnownFacts) > 0 {
		known.WriteString("\nKnown facts:\n")
		for _, f := range state.KnownFacts {
			known.WriteString("- " + f + "\n")
		}
	}
	usr := fmt.Sprintf("Product summary: %s%s\n\nQuestion: %s", state.IdeaSummary, known.String(), question)
	return sys, usr
}

func (VisionPromptSet) Critique(state *LoopState) (string, string) {
	sys := `Critique this product vision. List up to 3 issues as numbered items.
For each, state:
- What is vague, missing, inconsistent, or risky
Keep each item to one sentence.
If there are no issues, reply with "NONE".`

	var sb strings.Builder
	for _, s := range VisionSections {
		content := state.Sections[s]
		if strings.TrimSpace(content) == "" {
			content = "(empty)"
		}
		sb.WriteString(fmt.Sprintf("\n## %s\n%s\n", s, content))
	}
	return sys, sb.String()
}

func (VisionPromptSet) Synthesize(state *LoopState) (string, string) {
	sys := `Generate a structured product vision document using ONLY the provided data.
Do not invent anything. Use this exact markdown format:

# Product Vision: [derive name from the data]

## Problem Statement
[content]

## Target Users
[content]

## Core Value Propositions
[content]

## User Experience
[content]

## Success Metrics
[content]

## Business Constraints & Assumptions
[content]

## Out of Scope (V1)
[content]`

	var sb strings.Builder
	sb.WriteString("Data to use:\n")
	sb.WriteString(fmt.Sprintf("\nProblem: %s", state.Sections[SectionProblem]))
	sb.WriteString(fmt.Sprintf("\nUsers: %s", state.Sections[SectionUsers]))
	sb.WriteString(fmt.Sprintf("\nFeatures: %s", state.Sections[SectionFeatures]))
	sb.WriteString(fmt.Sprintf("\nUX: %s", state.Sections[SectionUX]))
	sb.WriteString(fmt.Sprintf("\nMetrics: %s", state.Sections[SectionMetrics]))
	sb.WriteString(fmt.Sprintf("\nConstraints: %s", state.Sections[SectionConstraints]))
	sb.WriteString(fmt.Sprintf("\nOut of Scope: %s", state.Sections[SectionOutOfScope]))
	return sys, sb.String()
}

// ── FilePromptSet ─────────────────────────────────────────────────

// filePromptMap maps PromptSet method names to template file names.
var filePromptMap = map[string]string{
	"Summarize":         "refinement-summarize.md.tmpl",
	"GenerateQuestions":  "refinement-generate-questions.md.tmpl",
	"ExtractFacts":       "refinement-extract-facts.md.tmpl",
	"NormalizeAnswer":    "refinement-normalize-answer.md.tmpl",
	"ClassifyFact":       "refinement-classify-fact.md.tmpl",
	"MergeFact":          "refinement-merge-fact.md.tmpl",
	"ScoreSection":       "refinement-score-section.md.tmpl",
	"EvaluateQuestions":  "refinement-evaluate-questions.md.tmpl",
	"RewriteQuestion":    "refinement-rewrite-question.md.tmpl",
	"CoherenceCheck":     "refinement-coherence-check.md.tmpl",
	"TensionCheck":       "refinement-tension-check.md.tmpl",
	"FindGaps":           "refinement-find-gaps.md.tmpl",
	"SimulateAnswer":     "refinement-simulate-answer.md.tmpl",
	"Critique":           "refinement-critique.md.tmpl",
	"Synthesize":         "refinement-synthesize.md.tmpl",
}

// FilePromptSet wraps VisionPromptSet, overriding system prompts from a
// PromptStore while keeping user message assembly in Go code.
type FilePromptSet struct {
	VisionPromptSet
	store *promptfiles.PromptStore
}

// NewFilePromptSet creates a FilePromptSet. If store is nil, behaves
// identically to VisionPromptSet.
func NewFilePromptSet(store *promptfiles.PromptStore) FilePromptSet {
	return FilePromptSet{store: store}
}

// loadSys loads the system prompt from the store, falling back to the hardcoded default.
func (f FilePromptSet) loadSys(method, fallback string) string {
	if f.store == nil {
		return fallback
	}
	name, ok := filePromptMap[method]
	if !ok {
		return fallback
	}
	content, err := f.store.Load(name)
	if err != nil {
		log.Printf("promptfiles: refinement %s load failed, using default: %v", name, err)
		return fallback
	}
	return content
}

func (f FilePromptSet) Summarize(input string) (string, string) {
	sys, usr := f.VisionPromptSet.Summarize(input)
	return f.loadSys("Summarize", sys), usr
}

func (f FilePromptSet) GenerateQuestions(state *LoopState) (string, string) {
	sys, usr := f.VisionPromptSet.GenerateQuestions(state)
	return f.loadSys("GenerateQuestions", sys), usr
}

func (f FilePromptSet) ExtractFacts(summary, answer string) (string, string) {
	sys, usr := f.VisionPromptSet.ExtractFacts(summary, answer)
	return f.loadSys("ExtractFacts", sys), usr
}

func (f FilePromptSet) NormalizeAnswer(summary, question, answer string) (string, string) {
	sys, usr := f.VisionPromptSet.NormalizeAnswer(summary, question, answer)
	return f.loadSys("NormalizeAnswer", sys), usr
}

func (f FilePromptSet) ClassifyFact(fact string, sections []SectionName) (string, string) {
	sys, usr := f.VisionPromptSet.ClassifyFact(fact, sections)
	return f.loadSys("ClassifyFact", sys), usr
}

func (f FilePromptSet) MergeFact(summary, existingContent, fact string) (string, string) {
	sys, usr := f.VisionPromptSet.MergeFact(summary, existingContent, fact)
	return f.loadSys("MergeFact", sys), usr
}

func (f FilePromptSet) ScoreSection(name SectionName, content string) (string, string) {
	sys, usr := f.VisionPromptSet.ScoreSection(name, content)
	return f.loadSys("ScoreSection", sys), usr
}

func (f FilePromptSet) EvaluateQuestions(state *LoopState, questions []Question) (string, string) {
	sys, usr := f.VisionPromptSet.EvaluateQuestions(state, questions)
	return f.loadSys("EvaluateQuestions", sys), usr
}

func (f FilePromptSet) RewriteQuestion(state *LoopState, question string) (string, string) {
	sys, usr := f.VisionPromptSet.RewriteQuestion(state, question)
	return f.loadSys("RewriteQuestion", sys), usr
}

func (f FilePromptSet) CoherenceCheck(state *LoopState, newFacts []string) (string, string) {
	sys, usr := f.VisionPromptSet.CoherenceCheck(state, newFacts)
	return f.loadSys("CoherenceCheck", sys), usr
}

func (f FilePromptSet) TensionCheck(state *LoopState) (string, string) {
	sys, usr := f.VisionPromptSet.TensionCheck(state)
	return f.loadSys("TensionCheck", sys), usr
}

func (f FilePromptSet) FindGaps(state *LoopState) (string, string) {
	sys, usr := f.VisionPromptSet.FindGaps(state)
	return f.loadSys("FindGaps", sys), usr
}

func (f FilePromptSet) SimulateAnswer(state *LoopState, question string) (string, string) {
	sys, usr := f.VisionPromptSet.SimulateAnswer(state, question)
	return f.loadSys("SimulateAnswer", sys), usr
}

func (f FilePromptSet) Critique(state *LoopState) (string, string) {
	sys, usr := f.VisionPromptSet.Critique(state)
	return f.loadSys("Critique", sys), usr
}

func (f FilePromptSet) Synthesize(state *LoopState) (string, string) {
	sys, usr := f.VisionPromptSet.Synthesize(state)
	return f.loadSys("Synthesize", sys), usr
}
