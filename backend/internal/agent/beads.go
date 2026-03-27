package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/michelroberge/paulette/backend/internal/model"
)

// ParseBuildPlanSystemPrompt is exported so handlers can pass it to provider.Chat directly.
const ParseBuildPlanSystemPrompt = parseBuildPlanSystemPrompt

const parseBuildPlanSystemPrompt = `You are a Build Plan Parser for an AI App Factory. Read the build plan and architecture below and extract all milestones and tasks into a structured JSON format. 

OUTPUT FORMAT: Wrap in <!-- RESPONSE:START -->...<!-- RESPONSE:END -->. Put JSON only in <jsonplan>...</jsonplan>. No discussion.
<!-- RESPONSE:START -->
<jsonplan>{"epics":[...]}</jsonplan>
<!-- RESPONSE:END -->

Example:
<!-- RESPONSE:START -->
<jsonplan>{
  "epics": [
    {
      "title": "Milestone name",
      "description": "Short description of the milestone goal",
      "tasks": [
        {
          "title": "Task name",
          "description": "What needs to be implemented",
          "depsOn": ["Other Task Title"],
          "priority": 2,
          "tags": ["backend", "api"],
          "targetFiles": ["src/api/auth.go", "src/middleware/"],
          "journeyRefs": ["JRN-v1.0-001"],
          "archRefs": ["ARCH-v1.0-001", "ARCH-v1.0-002"]
        }
      ]
    }
  ]
}</jsonplan>
<!-- RESPONSE:END -->

Rules:
- Each milestone in the build plan becomes an epic
- Each deliverable, task, or sub-item within a milestone becomes a task
- depsOn contains the exact titles of tasks this task depends on (can reference tasks across epics by exact title)
- priority: 0=critical, 1=high, 2=medium (default), 3=low, 4=backlog
- tags: one or more from this set: backend, frontend, api, database, styling, config, testing, devops. Use these to classify what area of the codebase the task touches.
- targetFiles: relative file/directory paths (from the project root) the task should create or modify. NEVER use absolute paths. If unsure, omit rather than guess.
- journeyRefs: array of JRN-* IDs from the UX document that this task directly serves. Extract these from the task description and build plan text. If not explicit, infer from context (e.g. a login task serves the authentication journey). Always output as an array (use [] if genuinely unknown).
- archRefs: array of ARCH-* IDs from the architecture document that this task directly implements or modifies. Extract from task descriptions and architecture references. Always output as an array (use [] if genuinely unknown).
- Do not include any text, explanation, or markdown outside the XML envelope
- Do not include any time estimates`

const devilAdvocateSystemPrompt = `You are the Devil's Advocate Agent for an AI App Factory. Your role is to critically review code just written by another agent and challenge its quality, completeness, and correctness.

You have read-only access to the project files via Bash. Review what was implemented for the given task.
Use relative paths in all Bash commands (e.g., ` + "`cat src/app.py`" + `, not ` + "`cat /root/project/src/app.py`" + `). Your working directory is already set to the project root.

Challenge:
- Is the implementation complete or are there stubs/placeholders?
- Does it match the task description and architecture requirements?
- Are there obvious bugs, missing error handling, or edge cases?
- Does it integrate correctly with the rest of the codebase?
- Is there anything the code writer clearly missed?

Be a tough reviewer, but pragmatic. Focus on real issues, not style preferences.

CRITICAL — your final text response determines what happens next. The VERY FIRST word of your <discussion> content decides the outcome:
1. If the implementation is satisfactory: start your <discussion> with "LGTM" optionally followed by a brief reason.
2. If there are real issues: start your <discussion> with a concise bullet list of specific, actionable issues. Do NOT include "LGTM" anywhere.

You may use Bash to inspect files before responding, but your final output must use this format:
<!-- RESPONSE:START -->
<discussion>LGTM (or bullet list of issues)</discussion>
<!-- RESPONSE:END -->

No preamble, no narration of what you did — just the verdict wrapped in the XML envelope.`

const codeWriterSystemPrompt = `You are a Code Writer Agent for an AI App Factory. You implement individual tasks from an approved build plan by writing real, working code.

%s

When given a task to implement:
- Write complete, functional code — not stubs or placeholders
- Create all necessary files using the Write, Edit, and Bash tools
- Follow the architecture decisions and tech stack from the approved artifacts
- Make the code work end-to-end for this specific task
- Run tests or build commands if applicable to verify the implementation
- ALWAYS use relative paths for file operations (e.g., ` + "`src/app.py`" + `, not ` + "`/root/project/src/app.py`" + `). Your working directory is already set to the project root.`

// ParseBuildPlan streams Claude's response while parsing build.md into structured epics/tasks.
// The caller should collect the full "done" event content and call ExtractBeadJSON on it.
// archContent is optional; when provided it helps Claude infer target files and tags.
func ParseBuildPlan(ctx context.Context, buildMdContent, archContent string) (<-chan StreamEvent, error) {
	var prompt strings.Builder
	prompt.WriteString("Build Plan:\n---\n")
	prompt.WriteString(buildMdContent)
	prompt.WriteString("\n---\n\n")
	if archContent != "" {
		prompt.WriteString("Architecture (use this to infer target files and tags for each task):\n---\n")
		prompt.WriteString(archContent)
		prompt.WriteString("\n---\n\n")
	}
	prompt.WriteString("Extract all milestones and tasks from this build plan into the required JSON format. Include tags and targetFiles for each task.")

	cmd := exec.CommandContext(ctx, claudeBin,
		"--print",
		"--output-format", "stream-json",
		"--verbose",
		"--model", "claude-opus-4-6",
		"--system-prompt", parseBuildPlanSystemPrompt,
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

		var fullResponse strings.Builder
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

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
							fullResponse.WriteString(c.Text)
							ch <- StreamEvent{Type: "chunk", Content: c.Text}
						}
					}
				}
			case "result":
				if event.IsError {
					ch <- StreamEvent{Type: "plan_limit", Content: "Claude plan limit reached"}
					return
				}
				if fullResponse.Len() == 0 && event.Result != "" {
					fullResponse.WriteString(event.Result)
					ch <- StreamEvent{Type: "chunk", Content: event.Result}
				}
			}
		}

		ch <- StreamEvent{Type: "done", Content: fullResponse.String()}
	}()

	return ch, nil
}

// beadExecutionModel returns the model to use for executing a bead based on its tags.
// Haiku is used only when ALL tags are "simple" (styling, config, devops).
// Sonnet is used for any complex tag (backend, api, database, testing, frontend) or no tags.
func beadExecutionModel(tags []string) string {
	if len(tags) == 0 {
		return "claude-sonnet-4-6"
	}
	simpleTags := map[string]bool{"styling": true, "config": true, "devops": true}
	for _, t := range tags {
		if !simpleTags[t] {
			return "claude-sonnet-4-6"
		}
	}
	return "claude-haiku-4-5-20251001"
}

// ExecuteBead streams Claude's code-writing response for a single bead.
// Claude is invoked with Write, Edit, and Bash tool access in the project directory.
// Artifacts are filtered based on bead tags to reduce context waste.
type skillContextKey struct{}

// WithSkillContext attaches skill context to a context.Context for bead execution.
func WithSkillContext(ctx context.Context, sc *SkillContext) context.Context {
	return context.WithValue(ctx, skillContextKey{}, sc)
}

// SkillContext holds matched skill prompts to inject during bead execution.
type SkillContext struct {
	Skills []MatchedSkill
}

// MatchedSkill pairs a skill name with its rendered prompt template.
type MatchedSkill struct {
	Name   string
	Prompt string
}

func ExecuteBead(ctx context.Context, projectDir string, bead model.Bead, artifacts map[model.StageName]string, enhancement ...*EnhancementContext) (<-chan StreamEvent, error) {
	var systemPrompt strings.Builder
	var artifactContext strings.Builder

	relevantStages := model.RelevantStages(bead.Tags)
	for _, stage := range relevantStages {
		if content, ok := artifacts[stage]; ok && content != "" {
			artifactContext.WriteString(strings.ToUpper(string(stage)))
			artifactContext.WriteString(":\n---\n")
			artifactContext.WriteString(content)
			artifactContext.WriteString("\n---\n\n")
		}
	}

	systemPrompt.WriteString(fmt.Sprintf(codeWriterSystemPrompt, artifactContext.String()))
	systemPrompt.WriteString(fmt.Sprintf("\n\nProject root: `%s`\nALWAYS use relative paths for file operations — never hardcode absolute paths.", projectDir))

	// Inject enhancement context for code writer when iterating on existing code
	if len(enhancement) > 0 && enhancement[0] != nil {
		enhCtx := enhancement[0]
		systemPrompt.WriteString("\n\n--- ENHANCEMENT CONTEXT ---\n")
		systemPrompt.WriteString(`IMPORTANT: This is an ENHANCEMENT ITERATION. Code already exists in this project from a previous iteration.

Your approach MUST be:
1. READ existing files before writing — use Bash(cat/ls) to understand what's already there.
2. Use the Edit tool to MODIFY existing files rather than overwriting them with Write.
3. Only use Write for genuinely NEW files that don't exist yet.
4. Preserve existing functionality — your changes should be additive or surgical modifications.
5. Do NOT rewrite files from scratch unless the task explicitly requires a complete replacement.
6. Reference the previous iteration summary below to understand what already exists.
`)
		if enhCtx.Summary != "" {
			systemPrompt.WriteString("\nPrevious iteration summary (describes what code already exists):\n---\n")
			systemPrompt.WriteString(enhCtx.Summary)
			systemPrompt.WriteString("\n---\n\n")
		}
		systemPrompt.WriteString("Enhancement request: ")
		systemPrompt.WriteString(enhCtx.Vision)
		systemPrompt.WriteString("\n--- END ENHANCEMENT CONTEXT ---\n")
	}

	// Inject matched skill templates if provided via context value
	if skillCtx, ok := ctx.Value(skillContextKey{}).(*SkillContext); ok && skillCtx != nil && len(skillCtx.Skills) > 0 {
		systemPrompt.WriteString("\n\n--- REUSABLE SKILL TEMPLATES ---\n")
		systemPrompt.WriteString("The following skill templates are available to guide your implementation. Use them as patterns where applicable:\n\n")
		for _, ms := range skillCtx.Skills {
			systemPrompt.WriteString(fmt.Sprintf("### Skill: %s\n", ms.Name))
			systemPrompt.WriteString(ms.Prompt)
			systemPrompt.WriteString("\n\n")
		}
		systemPrompt.WriteString("--- END SKILL TEMPLATES ---\n")
	}

	var userMsg strings.Builder
	userMsg.WriteString(fmt.Sprintf("Task: %s\n\n", bead.Title))
	if bead.Description != "" {
		userMsg.WriteString(fmt.Sprintf("Description: %s\n\n", bead.Description))
	}
	if len(bead.TargetFiles) > 0 {
		userMsg.WriteString("Target files/directories:\n")
		for _, f := range bead.TargetFiles {
			userMsg.WriteString(fmt.Sprintf("- %s\n", f))
		}
		userMsg.WriteString("\nFocus your implementation on these locations.\n\n")
	}
	if len(bead.JourneyRefs) > 0 {
		userMsg.WriteString(fmt.Sprintf("User journeys served by this task: %s\n", strings.Join(bead.JourneyRefs, ", ")))
		userMsg.WriteString("Ensure the implementation correctly supports the UX flows for these journeys.\n\n")
	}
	if len(bead.ArchRefs) > 0 {
		userMsg.WriteString(fmt.Sprintf("Architectural elements to implement: %s\n", strings.Join(bead.ArchRefs, ", ")))
		userMsg.WriteString("Refer to the architecture document for the specification of these elements.\n\n")
	}
	userMsg.WriteString("Implement this task completely. Write all necessary code and files.")
	if len(enhancement) > 0 && enhancement[0] != nil {
		userMsg.WriteString("\n\nREMEMBER: This project has existing code from a prior iteration. Read existing files before modifying them. Use Edit for changes to existing files, Write only for new files.")
	}

	cmd := exec.CommandContext(ctx, claudeBin,
		"--print",
		"--output-format", "stream-json",
		"--verbose",
		"--include-partial-messages",
		"--model", beadExecutionModel(bead.Tags),
		"--allowedTools", "Write,Edit,Bash",
		"--permission-mode", "acceptEdits",
		"--system-prompt", systemPrompt.String(),
	)
	cmd.Stdin = strings.NewReader(userMsg.String())
	cmd.Dir = projectDir

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

		var fullResponse strings.Builder
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

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
							fullResponse.WriteString(c.Text)
							ch <- StreamEvent{Type: "chunk", Content: c.Text}
						}
					}
				}
			case "result":
				if event.IsError {
					ch <- StreamEvent{Type: "plan_limit", Content: "Claude plan limit reached"}
					return
				}
				if event.Usage != nil {
					total := event.Usage.InputTokens + event.Usage.OutputTokens
					ch <- StreamEvent{Type: "tokens", Tokens: total}
				}
				if fullResponse.Len() == 0 && event.Result != "" {
					fullResponse.WriteString(event.Result)
					ch <- StreamEvent{Type: "chunk", Content: event.Result}
				}
			}
		}

		ch <- StreamEvent{Type: "done", Content: fullResponse.String()}
	}()

	return ch, nil
}

// ReviewBead runs the devil's advocate agent to review a completed bead's implementation.
// Returns empty string if approved (LGTM), or a findings string describing issues.
// Also returns the token count consumed by the review.
// All artifacts are provided regardless of bead tags so the reviewer has full context.
// siblings contains the other tasks in the same epic for integration awareness.
func ReviewBead(ctx context.Context, projectDir string, bead model.Bead, artifacts map[model.StageName]string, siblings []model.Bead, enhancement ...*EnhancementContext) (string, int, error) {
	// Devil's advocate always gets all artifacts — it needs the full picture to spot integration gaps.
	var artifactContext strings.Builder
	for _, stage := range []model.StageName{model.StageVision, model.StageUX, model.StageArchitecture, model.StageBuild} {
		if content, ok := artifacts[stage]; ok && content != "" {
			artifactContext.WriteString(strings.ToUpper(string(stage)))
			artifactContext.WriteString(":\n---\n")
			artifactContext.WriteString(content)
			artifactContext.WriteString("\n---\n\n")
		}
	}

	var userMsg strings.Builder
	userMsg.WriteString(fmt.Sprintf("Review the implementation of this task:\n\nTask: %s\n", bead.Title))
	if bead.Description != "" {
		userMsg.WriteString(fmt.Sprintf("Description: %s\n", bead.Description))
	}
	if len(bead.TargetFiles) > 0 {
		userMsg.WriteString("\nTarget files to inspect:\n")
		for _, f := range bead.TargetFiles {
			userMsg.WriteString(fmt.Sprintf("- %s\n", f))
		}
	}
	if len(siblings) > 0 {
		userMsg.WriteString("\nSibling tasks in the same epic (for integration context):\n")
		for _, s := range siblings {
			tags := ""
			if len(s.Tags) > 0 {
				tags = " [" + strings.Join(s.Tags, ", ") + "]"
			}
			userMsg.WriteString(fmt.Sprintf("- [%s]%s %s: %s\n", strings.ToUpper(string(s.Status)), tags, s.Title, s.Description))
		}
		userMsg.WriteString("\nCheck that this task integrates correctly with completed siblings and leaves the right hooks for pending ones.\n")
	}
	userMsg.WriteString("\nProject context:\n")
	userMsg.WriteString(artifactContext.String())
	if len(enhancement) > 0 && enhancement[0] != nil {
		userMsg.WriteString("\nENHANCEMENT CONTEXT: This is an enhancement iteration. Existing code from a prior iteration should have been preserved. Check that:\n")
		userMsg.WriteString("- Existing files were modified (Edit), not overwritten (Write) unless a full rewrite was needed\n")
		userMsg.WriteString("- New functionality was added without breaking existing features\n")
		userMsg.WriteString("- The implementation is incremental, not a from-scratch rewrite\n")
	}
	userMsg.WriteString(fmt.Sprintf("\nProject root: `%s`\nUse relative paths in all Bash commands.\n", projectDir))
	userMsg.WriteString("\nUse Bash to inspect the project files, then assess whether this task was implemented correctly and completely.")

	cmd := exec.CommandContext(ctx, claudeBin,
		"--print",
		"--output-format", "stream-json",
		"--verbose",
		"--include-partial-messages",
		"--model", "claude-sonnet-4-6",
		"--allowedTools", "Bash",
		"--permission-mode", "acceptEdits",
		"--system-prompt", devilAdvocateSystemPrompt,
	)
	cmd.Stdin = strings.NewReader(userMsg.String())
	cmd.Dir = projectDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", 0, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return "", 0, fmt.Errorf("start claude: %w", err)
	}

	var fullResponse strings.Builder
	var totalTokens int
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

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
						fullResponse.WriteString(c.Text)
					}
				}
			}
		case "result":
			if event.IsError {
				cmd.Wait()
				return "", 0, &PlanLimitError{Message: event.Result}
			}
			if event.Usage != nil {
				totalTokens = event.Usage.InputTokens + event.Usage.OutputTokens
			}
			if fullResponse.Len() == 0 && event.Result != "" {
				fullResponse.WriteString(event.Result)
			}
		}
	}
	cmd.Wait()

	discussion := ParseResponse(fullResponse.String()).Discussion
	if discussion == "" {
		// Fallback: treat raw response as discussion if envelope is missing
		discussion = strings.TrimSpace(fullResponse.String())
	}
	if isLGTM(discussion) {
		return "", totalTokens, nil
	}
	return discussion, totalTokens, nil
}

// IsLGTM is the exported form of isLGTM for use by handler dispatch helpers.
func IsLGTM(response string) bool { return isLGTM(response) }

// BuildExecuteBeadRequest builds the system prompt and user message for a code-writer
// bead execution without actually running Claude. This is used by non-CLI providers
// that call ExecuteAgent directly.
func BuildExecuteBeadRequest(
	ctx context.Context,
	projectDir string,
	bead model.Bead,
	artifacts map[model.StageName]string,
	enhancement *EnhancementContext,
) (systemPrompt, userMsg string) {
	var sp strings.Builder
	var artifactContext strings.Builder

	relevantStages := model.RelevantStages(bead.Tags)
	for _, stage := range relevantStages {
		if content, ok := artifacts[stage]; ok && content != "" {
			artifactContext.WriteString(strings.ToUpper(string(stage)))
			artifactContext.WriteString(":\n---\n")
			artifactContext.WriteString(content)
			artifactContext.WriteString("\n---\n\n")
		}
	}

	sp.WriteString(fmt.Sprintf(codeWriterSystemPrompt, artifactContext.String()))
	sp.WriteString(fmt.Sprintf("\n\nProject root: `%s`\nALWAYS use relative paths for file operations — never hardcode absolute paths.", projectDir))

	if enhancement != nil {
		sp.WriteString("\n\n--- ENHANCEMENT CONTEXT ---\n")
		sp.WriteString(`IMPORTANT: This is an ENHANCEMENT ITERATION. Code already exists in this project from a previous iteration.

Your approach MUST be:
1. READ existing files before writing — use Bash(cat/ls) to understand what's already there.
2. Use the Edit tool to MODIFY existing files rather than overwriting them with Write.
3. Only use Write for genuinely NEW files that don't exist yet.
4. Preserve existing functionality — your changes should be additive or surgical modifications.
5. Do NOT rewrite files from scratch unless the task explicitly requires a complete replacement.
6. Reference the previous iteration summary below to understand what already exists.
`)
		if enhancement.Summary != "" {
			sp.WriteString("\nPrevious iteration summary (describes what code already exists):\n---\n")
			sp.WriteString(enhancement.Summary)
			sp.WriteString("\n---\n\n")
		}
		sp.WriteString("Enhancement request: ")
		sp.WriteString(enhancement.Vision)
		sp.WriteString("\n--- END ENHANCEMENT CONTEXT ---\n")
	}

	if skillCtx, ok := ctx.Value(skillContextKey{}).(*SkillContext); ok && skillCtx != nil && len(skillCtx.Skills) > 0 {
		sp.WriteString("\n\n--- REUSABLE SKILL TEMPLATES ---\n")
		sp.WriteString("The following skill templates are available to guide your implementation. Use them as patterns where applicable:\n\n")
		for _, ms := range skillCtx.Skills {
			sp.WriteString(fmt.Sprintf("### Skill: %s\n", ms.Name))
			sp.WriteString(ms.Prompt)
			sp.WriteString("\n\n")
		}
		sp.WriteString("--- END SKILL TEMPLATES ---\n")
	}

	var um strings.Builder
	um.WriteString(fmt.Sprintf("Task: %s\n\n", bead.Title))
	if bead.Description != "" {
		um.WriteString(fmt.Sprintf("Description: %s\n\n", bead.Description))
	}
	if len(bead.TargetFiles) > 0 {
		um.WriteString("Target files/directories:\n")
		for _, f := range bead.TargetFiles {
			um.WriteString(fmt.Sprintf("- %s\n", f))
		}
		um.WriteString("\nFocus your implementation on these locations.\n\n")
	}
	if len(bead.JourneyRefs) > 0 {
		um.WriteString(fmt.Sprintf("User journeys served by this task: %s\n", strings.Join(bead.JourneyRefs, ", ")))
		um.WriteString("Ensure the implementation correctly supports the UX flows for these journeys.\n\n")
	}
	if len(bead.ArchRefs) > 0 {
		um.WriteString(fmt.Sprintf("Architectural elements to implement: %s\n", strings.Join(bead.ArchRefs, ", ")))
		um.WriteString("Refer to the architecture document for the specification of these elements.\n\n")
	}
	um.WriteString("Implement this task completely. Write all necessary code and files.")
	if enhancement != nil {
		um.WriteString("\n\nREMEMBER: This project has existing code from a prior iteration. Read existing files before modifying them. Use Edit for changes to existing files, Write only for new files.")
	}

	return sp.String(), um.String()
}

// BuildReviewBeadRequest builds the system prompt and user message for a devil's-advocate
// review without actually running Claude. Used by non-CLI providers.
func BuildReviewBeadRequest(
	projectDir string,
	bead model.Bead,
	artifacts map[model.StageName]string,
	siblings []model.Bead,
	enhancement *EnhancementContext,
) (systemPrompt, userMsg string) {
	var artifactContext strings.Builder
	for _, stage := range []model.StageName{model.StageVision, model.StageUX, model.StageArchitecture, model.StageBuild} {
		if content, ok := artifacts[stage]; ok && content != "" {
			artifactContext.WriteString(strings.ToUpper(string(stage)))
			artifactContext.WriteString(":\n---\n")
			artifactContext.WriteString(content)
			artifactContext.WriteString("\n---\n\n")
		}
	}

	var um strings.Builder
	um.WriteString(fmt.Sprintf("Review the implementation of this task:\n\nTask: %s\n", bead.Title))
	if bead.Description != "" {
		um.WriteString(fmt.Sprintf("Description: %s\n", bead.Description))
	}
	if len(bead.TargetFiles) > 0 {
		um.WriteString("\nTarget files to inspect:\n")
		for _, f := range bead.TargetFiles {
			um.WriteString(fmt.Sprintf("- %s\n", f))
		}
	}
	if len(siblings) > 0 {
		um.WriteString("\nSibling tasks in the same epic (for integration context):\n")
		for _, s := range siblings {
			tags := ""
			if len(s.Tags) > 0 {
				tags = " [" + strings.Join(s.Tags, ", ") + "]"
			}
			um.WriteString(fmt.Sprintf("- [%s]%s %s: %s\n", strings.ToUpper(string(s.Status)), tags, s.Title, s.Description))
		}
		um.WriteString("\nCheck that this task integrates correctly with completed siblings and leaves the right hooks for pending ones.\n")
	}
	um.WriteString("\nProject context:\n")
	um.WriteString(artifactContext.String())
	if enhancement != nil {
		um.WriteString("\nENHANCEMENT CONTEXT: This is an enhancement iteration. Existing code from a prior iteration should have been preserved. Check that:\n")
		um.WriteString("- Existing files were modified (Edit), not overwritten (Write) unless a full rewrite was needed\n")
		um.WriteString("- New functionality was added without breaking existing features\n")
		um.WriteString("- The implementation is incremental, not a from-scratch rewrite\n")
	}
	um.WriteString(fmt.Sprintf("\nProject root: `%s`\nUse relative paths in all Bash commands.\n", projectDir))
	um.WriteString("\nUse Bash to inspect the project files, then assess whether this task was implemented correctly and completely.")

	return devilAdvocateSystemPrompt, um.String()
}

// isLGTM checks whether the devil's advocate response is an approval.
// Primary check: response starts with "LGTM".
// Fallback: if the response contains "LGTM" and has no actionable bullet
// points (lines starting with "- "), treat it as approval — the model
// narrated its review but ultimately approved.
func isLGTM(response string) bool {
	if strings.HasPrefix(response, "LGTM") {
		return true
	}
	upper := strings.ToUpper(response)
	if !strings.Contains(upper, "LGTM") {
		return false
	}
	// Contains LGTM but didn't start with it — only approve if there are
	// no actionable bullet points (issue lists).
	for _, line := range strings.Split(response, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			return false
		}
	}
	return true
}

// ExtractBeadJSON extracts the JSON build plan from a Claude XML envelope response.
func ExtractBeadJSON(response string) ([]byte, bool) {
	j := ParseResponse(response).JSON
	if j == nil {
		return nil, false
	}
	return j, true
}

// BuildPlanSchema is the JSON Schema for ParsedBuildPlan, used by ParseAndValidateJSON.
var BuildPlanSchema = []byte(`{
  "type": "object",
  "required": ["epics"],
  "properties": {
    "epics": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["title", "description", "tasks"],
        "properties": {
          "title": {"type": "string"},
          "description": {"type": "string"},
          "tasks": {
            "type": "array",
            "items": {
              "type": "object",
              "required": ["title", "description"],
              "properties": {
                "title": {"type": "string"},
                "description": {"type": "string"},
                "depsOn": {"type": "array"},
                "priority": {"type": "integer"},
                "tags": {"type": "array"},
                "targetFiles": {"type": "array"},
                "journeyRefs": {"type": "array"},
                "archRefs": {"type": "array"}
              }
            }
          }
        }
      }
    }
  }
}`)

// JSONFixSystemPrompt instructs the LLM to return only fixed JSON in the envelope format.
const JSONFixSystemPrompt = `You are a JSON repair tool. Fix the provided invalid JSON to exactly match the given schema.

OUTPUT FORMAT: Return ONLY the fixed JSON inside the envelope markers — no discussion, no explanation.
<!-- RESPONSE:START -->
<jsonplan>{ fixed JSON here }</jsonplan>
<!-- RESPONSE:END -->`

// BuildJSONFixPrompt constructs the user message for the LLM JSON fixer.
func BuildJSONFixPrompt(invalidJSON, schema []byte) string {
	return "Invalid JSON:\n```json\n" + string(invalidJSON) + "\n```\n\n" +
		"Expected schema:\n```json\n" + string(schema) + "\n```\n\n" +
		"Fix the JSON to match the schema and return it in the required envelope format."
}
