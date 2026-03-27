package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/michelroberge/paulette/backend/internal/agent"
)

// ClaudeCLIProvider implements Provider by delegating to the existing
// agent.ChatWithBin() subprocess wrapper. It is the backward-compatible
// default: when no stage configuration exists, ResolveForStage always
// returns this provider, preserving identical v0.1.0 behaviour.
//
// claudePath is captured at construction time from agent.GetClaudeBin() so
// that both Chat() and TestConnection() probe the same binary. This avoids
// the divergence that would occur if TestConnection hardcoded "claude" while
// Chat routed through a custom path set via agent.SetClaudePath().
type ClaudeCLIProvider struct {
	claudePath string // absolute path or bare name ("claude") for PATH lookup
}

// NewClaudeCLIProvider returns a ClaudeCLIProvider whose binary path is
// seeded from the currently configured agent binary (agent.GetClaudeBin()).
// Call this after agent.SetClaudePath() has been called in main (i.e. after
// config is loaded) to guarantee the captured path is the one the user
// configured.
func NewClaudeCLIProvider() *ClaudeCLIProvider {
	return &ClaudeCLIProvider{claudePath: agent.GetClaudeBin()}
}

// Chat delegates to agent.ChatWithBin(), passing p.claudePath explicitly so
// the subprocess uses the same binary that TestConnection probed.
func (p *ClaudeCLIProvider) Chat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	return agent.ChatWithBin(
		ctx,
		p.claudePath,
		req.Model,
		req.SystemPrompt,
		req.History,
		req.UserMessage,
		req.ProjectDir,
	)
}

// TestConnection verifies that the claude binary at p.claudePath is reachable
// by running "<claudePath> --version". A non-zero exit code or a missing
// binary returns an error. The context is forwarded so callers can apply a
// 30-second timeout (per the architecture spec).
func (p *ClaudeCLIProvider) TestConnection(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, p.claudePath, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%q --version failed: %w (output: %s)", p.claudePath, err, string(out))
	}
	return nil
}

// ListModels returns ErrModelListUnsupported because the Claude CLI does not
// expose a model enumeration endpoint. The frontend falls back to free-text
// model entry for this provider type.
func (p *ClaudeCLIProvider) ListModels(_ context.Context) ([]ModelInfo, error) {
	return nil, ErrModelListUnsupported
}

// ExecuteAgent spawns the Claude CLI in agentic mode (with tool access) and
// streams the response as StreamEvents.  The tool names from req.Tools are
// passed as --allowedTools.
func (p *ClaudeCLIProvider) ExecuteAgent(ctx context.Context, req AgentRequest) (<-chan StreamEvent, error) {
	if req.Model == "" {
		req.Model = "claude-sonnet-4-6"
	}

	names := toolNames(req.Tools)
	toolsArg := strings.Join(names, ",")
	if toolsArg == "" {
		toolsArg = "Bash"
	}

	args := []string{
		"--print",
		"--output-format", "stream-json",
		"--verbose",
		"--include-partial-messages",
		"--model", req.Model,
		"--allowedTools", toolsArg,
		"--permission-mode", "acceptEdits",
		"--system-prompt", req.SystemPrompt,
	}

	cmd := exec.CommandContext(ctx, p.claudePath, args...)
	cmd.Stdin = strings.NewReader(req.UserMessage)
	if req.ProjectDir != "" {
		cmd.Dir = req.ProjectDir
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("claude-cli execute-agent: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("claude-cli execute-agent: start: %w", err)
	}

	ch := make(chan StreamEvent, 64)

	go func() {
		defer close(ch)
		defer cmd.Wait()

		type claudeEvent struct {
			Type    string `json:"type"`
			Message *struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message,omitempty"`
			Result  string `json:"result,omitempty"`
			IsError bool   `json:"is_error,omitempty"`
			Usage   *struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage,omitempty"`
		}

		var fullResponse strings.Builder
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			var ev claudeEvent
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				continue
			}
			switch ev.Type {
			case "assistant":
				if ev.Message != nil {
					for _, c := range ev.Message.Content {
						if c.Type == "text" && c.Text != "" {
							fullResponse.WriteString(c.Text)
							ch <- StreamEvent{Type: "chunk", Content: c.Text}
						}
					}
				}
			case "result":
				if ev.IsError {
					msg := ev.Result
					if msg == "" {
						msg = "Claude plan limit reached"
					}
					ch <- StreamEvent{Type: "plan_limit", Content: msg}
					return
				}
				if ev.Usage != nil {
					total := ev.Usage.InputTokens + ev.Usage.OutputTokens
					ch <- StreamEvent{Type: "tokens", Tokens: total}
				}
				if fullResponse.Len() == 0 && ev.Result != "" {
					fullResponse.WriteString(ev.Result)
					ch <- StreamEvent{Type: "chunk", Content: ev.Result}
				}
			}
		}

		ch <- StreamEvent{Type: "done", Content: fullResponse.String()}
	}()

	return ch, nil
}
