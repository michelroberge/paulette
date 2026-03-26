package provider

import (
	"context"
	"fmt"
	"os/exec"

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
