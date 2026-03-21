package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/michelroberge/ai-app-factory/backend/internal/model"
)

// StreamEvent represents an event sent to the client via SSE.
type StreamEvent struct {
	Type    string `json:"type"`    // "chunk", "artifact", "done", "error"
	Content string `json:"content"` // text content for chunk, path for artifact, message for error
}

// claudeEvent represents a line from claude --output-format stream-json --verbose
type claudeEvent struct {
	Type    string          `json:"type"`
	Message *claudeMessage  `json:"message,omitempty"`
	Result  string          `json:"result,omitempty"`
	IsError bool            `json:"is_error,omitempty"`
}

type claudeMessage struct {
	Content []claudeContent `json:"content"`
}

type claudeContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

var artifactRegex = regexp.MustCompile(`(?s)<!-- ARTIFACT:START -->\s*(.*?)\s*<!-- ARTIFACT:END -->`)

// Chat spawns a Claude CLI subprocess and streams the response.
func Chat(ctx context.Context, systemPrompt string, history []model.Message, userMessage string) (<-chan StreamEvent, error) {
	prompt := formatConversation(history, userMessage)

	cmd := exec.CommandContext(ctx, "claude",
		"--print",
		"--output-format", "stream-json",
		"--verbose",
		"--system-prompt", systemPrompt,
	)
	cmd.Stdin = strings.NewReader(prompt)

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
							ch <- StreamEvent{Type: "chunk", Content: c.Text}
						}
					}
				}
			case "result":
				// result is the final event; use it if assistant produced nothing
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

// ExtractArtifact extracts content between ARTIFACT:START and ARTIFACT:END markers.
func ExtractArtifact(response string) (string, bool) {
	matches := artifactRegex.FindStringSubmatch(response)
	if len(matches) < 2 {
		return "", false
	}
	return strings.TrimSpace(matches[1]), true
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
