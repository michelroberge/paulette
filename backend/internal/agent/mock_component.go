package agent

import (
	"fmt"

	"github.com/michelroberge/paulette/backend/internal/model"
	ollamaprompts "github.com/michelroberge/paulette/backend/internal/prompts/ollama"
)

// BuildMockComponentPlannerSystemPrompt returns the system prompt for the component
// planner step. It appends the framework style contract so the LLM writes
// framework-aware component descriptions.
func BuildMockComponentPlannerSystemPrompt(cfg *model.FrameworkConfig) string {
	return ollamaprompts.MockComponentPlanner +
		"\n\nStyle context (use these classes/tokens in component descriptions):\n" +
		frameworkViewInstruct(cfg)
}

// BuildMockComponentSystemPrompt returns the per-component generation system prompt
// for the given framework config.
func BuildMockComponentSystemPrompt(cfg *model.FrameworkConfig) string {
	return fmt.Sprintf(ollamaprompts.MockComponentBase, frameworkViewInstruct(cfg))
}
