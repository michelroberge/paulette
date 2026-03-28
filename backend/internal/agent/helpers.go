package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/michelroberge/paulette/backend/internal/model"
)

// dynamicPlanLimitMsg is a sentinel value for streamOptions.planLimitMsg that
// instructs processStreamEvents to use event.Result as the plan_limit content,
// falling back to a literal when event.Result is empty.
const dynamicPlanLimitMsg = "\x00dynamic"

// streamOptions controls the behavior of processStreamEvents.
type streamOptions struct {
	// filter, when non-nil, is applied to each text chunk before sending.
	filter *StreamFilter
	// planLimitMsg enables IsError handling. Use dynamicPlanLimitMsg to forward
	// event.Result as the content (falling back to the literal on empty).
	// Use "" to skip IsError handling entirely.
	planLimitMsg string
	// tokenField controls how the token count is reported:
	//   "content" → StreamEvent{Type:"tokens", Content: fmt.Sprintf("%d", total)}
	//   "tokens"  → StreamEvent{Type:"tokens", Tokens: total}
	// Any other value (including "") skips token reporting.
	tokenField string
}

// processStreamEvents reads newline-delimited Claude stream-json events from scanner,
// accumulating text into fullText and dispatching StreamEvents to ch.
// Returns true if processing completed normally, false if a plan_limit event was emitted
// and the loop exited early (the caller should return without sending a "done" event).
func processStreamEvents(
	scanner *bufio.Scanner,
	fullText *strings.Builder,
	ch chan<- StreamEvent,
	opts streamOptions,
) bool {
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
						chunk := c.Text
						if opts.filter != nil {
							chunk = opts.filter.Feed(c.Text)
						}
						if chunk != "" {
							ch <- StreamEvent{Type: "chunk", Content: chunk}
						}
					}
				}
			}
		case "result":
			if opts.planLimitMsg != "" && event.IsError {
				msg := opts.planLimitMsg
				if msg == dynamicPlanLimitMsg {
					msg = event.Result
					if msg == "" {
						msg = "Claude plan limit reached"
					}
				}
				ch <- StreamEvent{Type: "plan_limit", Content: msg}
				return false
			}
			if event.Usage != nil {
				switch opts.tokenField {
				case "content":
					total := event.Usage.InputTokens + event.Usage.OutputTokens
					ch <- StreamEvent{Type: "tokens", Content: fmt.Sprintf("%d", total)}
				case "tokens":
					total := event.Usage.InputTokens + event.Usage.OutputTokens
					ch <- StreamEvent{Type: "tokens", Tokens: total}
				}
			}
			if fullText.Len() == 0 && event.Result != "" {
				fullText.WriteString(event.Result)
				chunk := event.Result
				if opts.filter != nil {
					chunk = opts.filter.Feed(event.Result)
				}
				if chunk != "" {
					ch <- StreamEvent{Type: "chunk", Content: chunk}
				}
			}
		}
	}
	return true
}

// buildArtifactContext builds the formatted artifact context string for the given
// ordered slice of stage names. Stages with no content in artifacts are skipped.
func buildArtifactContext(stages []model.StageName, artifacts map[model.StageName]string) string {
	var b strings.Builder
	for _, stage := range stages {
		if content, ok := artifacts[stage]; ok && content != "" {
			b.WriteString(strings.ToUpper(string(stage)))
			b.WriteString(":\n---\n")
			b.WriteString(content)
			b.WriteString("\n---\n\n")
		}
	}
	return b.String()
}

// buildExecuteUserMsg constructs the task user message for a code-writer bead invocation.
// When enhancement is non-nil, it appends the reminder about existing code.
func buildExecuteUserMsg(bead model.Bead, enhancement *EnhancementContext) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Task: %s\n\n", bead.Title))
	if bead.Description != "" {
		b.WriteString(fmt.Sprintf("Description: %s\n\n", bead.Description))
	}
	if len(bead.TargetFiles) > 0 {
		b.WriteString("Target files/directories:\n")
		for _, f := range bead.TargetFiles {
			b.WriteString(fmt.Sprintf("- %s\n", f))
		}
		b.WriteString("\nFocus your implementation on these locations.\n\n")
	}
	if len(bead.JourneyRefs) > 0 {
		b.WriteString(fmt.Sprintf("User journeys served by this task: %s\n", strings.Join(bead.JourneyRefs, ", ")))
		b.WriteString("Ensure the implementation correctly supports the UX flows for these journeys.\n\n")
	}
	if len(bead.ArchRefs) > 0 {
		b.WriteString(fmt.Sprintf("Architectural elements to implement: %s\n", strings.Join(bead.ArchRefs, ", ")))
		b.WriteString("Refer to the architecture document for the specification of these elements.\n\n")
	}
	b.WriteString("Implement this task completely. Write all necessary code and files.")
	if enhancement != nil {
		b.WriteString("\n\nREMEMBER: This project has existing code from a prior iteration. Read existing files before modifying them. Use Edit for changes to existing files, Write only for new files.")
	}
	return b.String()
}

// buildReviewUserMsg constructs the review user message for a devil's-advocate bead invocation.
// artifactContext should already be built via buildArtifactContext before calling.
// openBeads is the project-wide list of open/blocked beads (excluding siblings) so the reviewer
// can avoid re-raising issues that are already tracked.
func buildReviewUserMsg(
	projectDir string,
	bead model.Bead,
	artifactContext string,
	siblings []model.Bead,
	openBeads []model.Bead,
	enhancement *EnhancementContext,
) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Review the implementation of this task:\n\nTask: %s\n", bead.Title))
	if bead.Description != "" {
		b.WriteString(fmt.Sprintf("Description: %s\n", bead.Description))
	}
	if len(bead.TargetFiles) > 0 {
		b.WriteString("\nTarget files to inspect:\n")
		for _, f := range bead.TargetFiles {
			b.WriteString(fmt.Sprintf("- %s\n", f))
		}
	}
	if len(siblings) > 0 {
		b.WriteString("\nSibling tasks in the same epic (for integration context):\n")
		for _, s := range siblings {
			tags := ""
			if len(s.Tags) > 0 {
				tags = " [" + strings.Join(s.Tags, ", ") + "]"
			}
			b.WriteString(fmt.Sprintf("- [%s]%s %s: %s\n", strings.ToUpper(string(s.Status)), tags, s.Title, s.Description))
		}
		b.WriteString("\nCheck that this task integrates correctly with completed siblings and leaves the right hooks for pending ones.\n")
	}
	b.WriteString("\nProject context:\n")
	b.WriteString(artifactContext)
	if enhancement != nil {
		b.WriteString("\nENHANCEMENT CONTEXT: This is an enhancement iteration. Existing code from a prior iteration should have been preserved. Check that:\n")
		b.WriteString("- Existing files were modified (Edit), not overwritten (Write) unless a full rewrite was needed\n")
		b.WriteString("- New functionality was added without breaking existing features\n")
		b.WriteString("- The implementation is incremental, not a from-scratch rewrite\n")
	}
	if len(openBeads) > 0 {
		b.WriteString("\nAlready tracked in backlog (do not re-raise these):\n")
		for _, ob := range openBeads {
			desc := ob.Description
			if len(desc) > 120 {
				desc = desc[:120] + "..."
			}
			b.WriteString(fmt.Sprintf("- [%s] (%s) %s", ob.ID, string(ob.Status), ob.Title))
			if desc != "" {
				b.WriteString(fmt.Sprintf(": %s", desc))
			}
			b.WriteString("\n")
		}
	}
	b.WriteString(fmt.Sprintf("\nProject root: `%s`\nUse relative paths in all Bash commands.\n", projectDir))
	b.WriteString("\nUse Bash to inspect the project files, then assess whether this task was implemented correctly and completely.")
	return b.String()
}
