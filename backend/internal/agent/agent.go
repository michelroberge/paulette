package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/michelroberge/paulette/backend/internal/model"
)

// ErrPlanLimit is the sentinel error for Claude usage/plan limit.
var ErrPlanLimit = errors.New("claude plan limit reached")

// PlanLimitError wraps ErrPlanLimit and carries the raw message from the Claude CLI,
// which typically includes the reset time (e.g. "Your limit resets at 5:00 PM UTC").
type PlanLimitError struct {
	Message string
}

func (e *PlanLimitError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "claude plan limit reached"
}

// Is makes errors.Is(err, ErrPlanLimit) return true for PlanLimitError values.
func (e *PlanLimitError) Is(target error) bool { return target == ErrPlanLimit }

// claudeBin is the path to the claude executable. Defaults to "claude" (PATH lookup).
var claudeBin = "claude"

// SetClaudePath overrides the claude executable path used by all agent functions.
func SetClaudePath(path string) {
	if path != "" {
		claudeBin = path
	}
}

// StreamEvent represents an event sent to the client via SSE.
type StreamEvent struct {
	Type    string `json:"type"`             // "chunk", "artifact", "done", "error", "tokens"
	Content string `json:"content"`          // text content for chunk, path for artifact, message for error
	Tokens  int    `json:"tokens,omitempty"` // token count for "tokens" events
}

// claudeEvent represents a line from claude --output-format stream-json --verbose
type claudeEvent struct {
	Type    string         `json:"type"`
	Subtype string         `json:"subtype,omitempty"`
	Message *claudeMessage `json:"message,omitempty"`
	Result  string         `json:"result,omitempty"`
	IsError bool           `json:"is_error,omitempty"`
	Usage   *claudeUsage   `json:"usage,omitempty"`
}

type claudeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type claudeMessage struct {
	Content []claudeContent `json:"content"`
}

type claudeContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}


// Chat spawns a Claude CLI subprocess and streams the response.
// modelID selects the Claude model (e.g. "claude-sonnet-4-6"); empty string uses the CLI default.
func Chat(ctx context.Context, modelID string, systemPrompt string, history []model.Message, userMessage string, projectDir string) (<-chan StreamEvent, error) {
	prompt := formatConversation(history, userMessage)

	args := []string{
		"--print",
		"--output-format", "stream-json",
		"--verbose",
		"--include-partial-messages",
		"--system-prompt", systemPrompt,
	}
	if modelID != "" {
		args = append(args, "--model", modelID)
	}
	cmd := exec.CommandContext(ctx, claudeBin, args...)
	cmd.Stdin = strings.NewReader(prompt)
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
		var filter StreamFilter
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
							fullResponse.WriteString(c.Text)
							if visible := filter.Feed(c.Text); visible != "" {
								ch <- StreamEvent{Type: "chunk", Content: visible}
							}
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
					ch <- StreamEvent{Type: "tokens", Content: fmt.Sprintf("%d", total)}
				}
				// result is the final event; use it if assistant produced nothing
				if fullResponse.Len() == 0 && event.Result != "" {
					fullResponse.WriteString(event.Result)
					if visible := filter.Feed(event.Result); visible != "" {
						ch <- StreamEvent{Type: "chunk", Content: visible}
					}
				}
			}
		}

		ch <- StreamEvent{Type: "done", Content: fullResponse.String()}
	}()

	return ch, nil
}

// ExtractArtifact extracts the artifact section from a Claude XML envelope response.
func ExtractArtifact(response string) (string, bool) {
	a := ParseResponse(response).Artifact
	if a == "" {
		return "", false
	}
	return a, true
}

// StripArtifact returns the discussion section of a Claude XML envelope response,
// omitting the artifact block.
func StripArtifact(response string) string {
	return ParseResponse(response).Discussion
}

func formatConversation(history []model.Message, newMessage string) string {
	var sb strings.Builder
	for _, msg := range history {
		switch msg.Role {
		case model.RoleUser:
			sb.WriteString("User: ")
		case model.RoleAssistant:
			sb.WriteString("Assistant: ")
		}
		sb.WriteString(msg.Content)
		sb.WriteString("\n\n")
	}
	sb.WriteString("User: ")
	sb.WriteString(newMessage)
	return sb.String()
}
