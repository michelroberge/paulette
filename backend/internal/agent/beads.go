package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/michelroberge/claudine/backend/internal/model"
)

var beadJSONRe = regexp.MustCompile(`(?s)<!-- JSON:START -->\s*(.*?)\s*<!-- JSON:END -->`)

const parseBuildPlanSystemPrompt = `You are a Build Plan Parser for an AI App Factory. Read the build plan and architecture below and extract all milestones and tasks into a structured JSON format.

Output ONLY a JSON block wrapped in exactly these delimiters:
<!-- JSON:START -->
{
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
}
<!-- JSON:END -->

Rules:
- Each milestone in the build plan becomes an epic
- Each deliverable, task, or sub-item within a milestone becomes a task
- depsOn contains the exact titles of tasks this task depends on (can reference tasks across epics by exact title)
- priority: 0=critical, 1=high, 2=medium (default), 3=low, 4=backlog
- tags: one or more from this set: backend, frontend, api, database, styling, config, testing, devops. Use these to classify what area of the codebase the task touches.
- targetFiles: file paths or directory paths the task should create or modify, inferred from the architecture document. If unsure, omit rather than guess.
- journeyRefs: array of JRN-* IDs from the UX document that this task directly serves. Extract these from the task description and build plan text. If not explicit, infer from context (e.g. a login task serves the authentication journey). Always output as an array (use [] if genuinely unknown).
- archRefs: array of ARCH-* IDs from the architecture document that this task directly implements or modifies. Extract from task descriptions and architecture references. Always output as an array (use [] if genuinely unknown).
- Do not include any text, explanation, or markdown outside the delimiters`

const devilAdvocateSystemPrompt = `You are the Devil's Advocate Agent for an AI App Factory. Your role is to critically review code just written by another agent and challenge its quality, completeness, and correctness.

You have read-only access to the project files via Bash. Review what was implemented for the given task.

Challenge:
- Is the implementation complete or are there stubs/placeholders?
- Does it match the task description and architecture requirements?
- Are there obvious bugs, missing error handling, or edge cases?
- Does it integrate correctly with the rest of the codebase?
- Is there anything the code writer clearly missed?

Be a tough reviewer, but pragmatic. Focus on real issues, not style preferences.

Respond with EXACTLY one of:
1. "LGTM" (optionally followed by a brief reason) if the implementation is satisfactory
2. A concise bullet list of specific, actionable issues — no preamble, no "LGTM"`

const codeWriterSystemPrompt = `You are a Code Writer Agent for an AI App Factory. You implement individual tasks from an approved build plan by writing real, working code.

%s

When given a task to implement:
- Write complete, functional code — not stubs or placeholders
- Create all necessary files using the Write, Edit, and Bash tools
- Follow the architecture decisions and tech stack from the approved artifacts
- Make the code work end-to-end for this specific task
- Run tests or build commands if applicable to verify the implementation`

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

	cmd := exec.CommandContext(ctx, "claude",
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

	cmd := exec.CommandContext(ctx, "claude",
		"--print",
		"--output-format", "stream-json",
		"--verbose",
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
	userMsg.WriteString("\nUse Bash to inspect the project files, then assess whether this task was implemented correctly and completely.")

	cmd := exec.CommandContext(ctx, "claude",
		"--print",
		"--output-format", "stream-json",
		"--verbose",
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
				return "", 0, ErrPlanLimit
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

	response := strings.TrimSpace(fullResponse.String())
	if strings.HasPrefix(response, "LGTM") {
		return "", totalTokens, nil
	}
	return response, totalTokens, nil
}

// ExtractBeadJSON extracts the JSON block from Claude's response.
func ExtractBeadJSON(response string) ([]byte, bool) {
	m := beadJSONRe.FindStringSubmatch(response)
	if len(m) < 2 {
		return nil, false
	}
	return []byte(m[1]), true
}
