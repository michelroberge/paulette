package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/michelroberge/paulette/backend/internal/model"
	ollamaprompts "github.com/michelroberge/paulette/backend/internal/prompts/ollama"
)

// StepLogger captures per-step LLM exchange details for run-log tracing.
// The handler package provides a runTrace implementation that writes each
// exchange (prompt + raw response) to a trace file under the current run's
// directory so it can be inspected from the UI. Nil is a no-op.
type StepLogger interface {
	LogStep(stepName, status, detail, content string, dur time.Duration)
}

// ChatExecutorRequest is the neutral request type passed to a ChatExecutor.
// It is a deliberate subset of provider.ChatRequest — defined here to keep the
// agent package free of any dependency on the provider package (the provider
// package already imports agent, so the reverse would be a cycle).
type ChatExecutorRequest struct {
	Model        string
	SystemPrompt string
	History      []model.Message
	UserMessage  string
	ProjectDir   string
	Stage        string
	Temperature  *float64
	NumCtx       *int
	Stream       *bool
	// Format is forwarded to providers that support structured-output
	// enforcement (currently Ollama's /api/chat "format" field).
	Format json.RawMessage
}

// ChatExecutor is a minimal interface the Vision turn runner needs from a
// provider: a single streaming chat call. Callers (e.g. the handler package)
// adapt their concrete provider.Provider to this interface.
type ChatExecutor interface {
	Chat(ctx context.Context, req ChatExecutorRequest) (<-chan StreamEvent, error)
}

// VisionTurnOptions configures the Vision turn runner.
type VisionTurnOptions struct {
	Executor    ChatExecutor
	Model       string
	History     []model.Message
	UserMessage string
	ProjectDir  string
	Stage       string
	Temperature *float64
	NumCtx      *int
	Stream      *bool
	// Logger, if set, receives one LogStep call per LLM exchange (draft,
	// critique, repair) with the full prompt + raw response as content so the
	// run-log UI can surface each exchange.
	Logger StepLogger
}

// logVisionStep records one LLM exchange in the run log (when a Logger is
// attached). Content is the full prompt + raw response formatted by
// buildExchangeTrace, so the UI's trace viewer shows the complete round-trip.
func logVisionStep(opts VisionTurnOptions, stepName, status, detail, systemPrompt, userMsg, raw string, dur time.Duration) {
	if opts.Logger == nil {
		return
	}
	content := buildExchangeTrace(opts.Model, opts.Stage, stepName, systemPrompt, opts.History, userMsg, raw)
	opts.Logger.LogStep(stepName, status, detail, content, dur)
}

// buildExchangeTrace formats a single LLM exchange (system prompt, history,
// user message, raw response) as a human-readable trace file body. It is used
// as the `content` argument to StepLogger.LogStep so the UI's trace viewer
// shows the complete round-trip the agent performed.
func buildExchangeTrace(modelID, stage, stepName, systemPrompt string, history []model.Message, userMsg, raw string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Step:  %s\n", stepName)
	fmt.Fprintf(&b, "Stage: %s\n", stage)
	fmt.Fprintf(&b, "Model: %s\n", modelID)
	b.WriteString("\n===== SYSTEM PROMPT =====\n")
	b.WriteString(systemPrompt)
	b.WriteString("\n\n===== HISTORY =====\n")
	if len(history) == 0 {
		b.WriteString("(none)\n")
	} else {
		for i, m := range history {
			fmt.Fprintf(&b, "[%d] %s:\n%s\n\n", i, m.Role, m.Content)
		}
	}
	b.WriteString("===== USER MESSAGE =====\n")
	b.WriteString(userMsg)
	b.WriteString("\n\n===== RAW RESPONSE =====\n")
	b.WriteString(raw)
	b.WriteString("\n")
	return b.String()
}

// RunVisionTurn executes a draft → critique → (optional) repair loop for the
// Vision stage and returns a channel of StreamEvents compatible with the
// provider.Provider.Chat contract.
//
// The final "done" event's Content is an XML-enveloped response containing the
// discussion and the rendered markdown artifact. This lets the caller's
// ExtractArtifact and StripArtifact pipeline work unchanged — the runner is a
// drop-in replacement for one prov.Chat call at the Vision stage.
//
// The runner emits:
//   - "log" events at each phase transition so the UI can show progress.
//   - "tokens" events aggregating each underlying call's token count.
//   - "chunk" events carrying the final discussion text (so the existing
//     streaming UI renders it once).
//   - a final "done" event whose Content is the full XML envelope.
//
// On unrecoverable failure the runner emits an "error" event followed by
// "done"; the caller should detect this (e.g. via an empty artifact after
// ExtractArtifact) and fall back to the legacy single-shot provider Chat.
func RunVisionTurn(ctx context.Context, opts VisionTurnOptions) (<-chan StreamEvent, error) {
	if opts.Executor == nil {
		return nil, fmt.Errorf("vision turn: executor is nil")
	}
	if opts.Model == "" {
		return nil, fmt.Errorf("vision turn: model is empty")
	}

	// First call is synchronous so HTTP/connection errors surface via the error
	// return value, letting the caller wrap them in a structured connection-error
	// payload (same behaviour as the legacy prov.Chat path). Subsequent phases
	// run inside the goroutine — any of their failures fall back to accepting
	// the draft and emitting a log event, so the turn still completes.
	draftStream, err := opts.Executor.Chat(ctx, ChatExecutorRequest{
		Model:        opts.Model,
		SystemPrompt: ollamaprompts.VisionDraftJSON,
		History:      opts.History,
		UserMessage:  opts.UserMessage,
		ProjectDir:   opts.ProjectDir,
		Stage:        opts.Stage,
		Temperature:  opts.Temperature,
		NumCtx:       opts.NumCtx,
		Stream:       opts.Stream,
		Format:       VisionDraftSchemaJSON,
	})
	if err != nil {
		return nil, err
	}

	ch := make(chan StreamEvent, 32)
	go func() {
		defer close(ch)
		runVisionPipeline(ctx, opts, draftStream, ch)
	}()
	return ch, nil
}

func runVisionPipeline(ctx context.Context, opts VisionTurnOptions, draftStream <-chan StreamEvent, ch chan<- StreamEvent) {
	stepStart := time.Now()
	emitLog(ch, "[vision] drafting...")
	raw, tokens, streamErr := drainStream(draftStream)
	emitTokens(ch, tokens)
	if streamErr != "" && raw == "" {
		logVisionStep(opts, "vision_draft", "error", "stream: "+streamErr, ollamaprompts.VisionDraftJSON, opts.UserMessage, raw, time.Since(stepStart))
		emitError(ch, "vision draft failed: "+streamErr)
		return
	}
	var draft VisionDraftOutput
	if err := unmarshalVisionJSON(raw, &draft); err != nil {
		// Small models often fail to produce clean JSON. If we have *any* model
		// output, treat it as a question-mode discussion so the user can still
		// see the reply and continue. Only hard-fail when the model produced
		// literally nothing.
		if strings.TrimSpace(raw) == "" {
			logVisionStep(opts, "vision_draft", "error", "empty response", ollamaprompts.VisionDraftJSON, opts.UserMessage, raw, time.Since(stepStart))
			emitError(ch, "vision draft failed: "+err.Error())
			return
		}
		logVisionStep(opts, "vision_draft", "error", "parse: "+err.Error()+" — fell back to raw discussion", ollamaprompts.VisionDraftJSON, opts.UserMessage, raw, time.Since(stepStart))
		emitLog(ch, "[vision] draft not structured — returning raw discussion ("+err.Error()+")")
		envelope := wrapVisionEnvelope(raw, "")
		emitFinalVisionTurn(ch, raw, envelope)
		return
	}
	logVisionStep(opts, "vision_draft", "ok", "mode="+draft.Mode, ollamaprompts.VisionDraftJSON, opts.UserMessage, raw, time.Since(stepStart))
	draft.Mode = strings.ToLower(strings.TrimSpace(draft.Mode))
	if draft.Mode == "" {
		if draft.Sections != nil {
			draft.Mode = "draft"
		} else {
			draft.Mode = "question"
		}
	}

	// Question-mode turns don't touch the artifact — just emit the discussion.
	if draft.Mode == "question" || draft.Sections == nil {
		envelope := wrapVisionEnvelope(draft.Discussion, "")
		emitFinalVisionTurn(ch, draft.Discussion, envelope)
		return
	}

	sections := *draft.Sections

	emitLog(ch, "[vision] reviewing...")
	critique, cTokens, cErr := visionCritique(ctx, opts, sections)
	emitTokens(ch, cTokens)
	if cErr != nil {
		// Critique itself failed — log and accept the draft rather than fail the turn.
		emitLog(ch, "[vision] critique unavailable, accepting draft: "+cErr.Error())
	} else if !critique.Approved {
		sections = applyCritique(ctx, opts, ch, sections, critique, draft)
	}

	// Ensure all sections are populated. If still missing, fall back to the draft.
	if !sections.HasAllSections() && draft.Sections != nil {
		sections = mergeVisionSections(sections, *draft.Sections)
	}

	artifact := RenderVisionMarkdown(sections)
	envelope := wrapVisionEnvelope(draft.Discussion, artifact)
	emitFinalVisionTurn(ch, draft.Discussion, envelope)
}

// applyCritique handles the non-approved critique branch: it either accepts
// a provided revision, or attempts one repair call using the draft prompt with
// the critique issues appended as constraints.
func applyCritique(ctx context.Context, opts VisionTurnOptions, ch chan<- StreamEvent, sections VisionSections, critique VisionCritiqueOutput, draft VisionDraftOutput) VisionSections {
	if critique.RevisedSections != nil && critique.RevisedSections.HasAllSections() {
		emitLog(ch, "[vision] accepted critique revision")
		return *critique.RevisedSections
	}
	if len(critique.Issues) == 0 {
		return sections
	}
	emitLog(ch, "[vision] refining...")
	repaired, rTokens, rErr := visionRepair(ctx, opts, sections, critique.Issues)
	emitTokens(ch, rTokens)
	if rErr != nil {
		emitLog(ch, "[vision] refine failed, accepting draft: "+rErr.Error())
		return sections
	}
	if repaired.Sections != nil && repaired.Sections.HasAllSections() {
		return *repaired.Sections
	}
	if repaired.Sections != nil {
		return mergeVisionSections(*repaired.Sections, sections)
	}
	_ = draft // kept for symmetry; unused here but a future phase may diff against it
	return sections
}

// drainStream reads every event from ch until it closes, returning the final
// assistant text (preferring a done event's Content), the accumulated token
// count, and any error message surfaced via an "error" event.
func drainStream(ch <-chan StreamEvent) (text string, tokens int, errMsg string) {
	var buf strings.Builder
	for e := range ch {
		switch e.Type {
		case "chunk":
			buf.WriteString(e.Content)
		case "tokens":
			var n int
			fmt.Sscanf(e.Content, "%d", &n)
			tokens += n
		case "error":
			errMsg = e.Content
		case "done":
			if e.Content != "" {
				return e.Content, tokens, errMsg
			}
			return buf.String(), tokens, errMsg
		}
	}
	return buf.String(), tokens, errMsg
}

// visionCritique asks the provider to evaluate the draft sections.
func visionCritique(ctx context.Context, opts VisionTurnOptions, sections VisionSections) (VisionCritiqueOutput, int, error) {
	stepStart := time.Now()
	payload, _ := json.MarshalIndent(sections, "", "  ")
	userMsg := "Evaluate the following proposed vision document.\n\n" + string(payload)
	raw, tokens, err := collectChat(ctx, opts.Executor, ChatExecutorRequest{
		Model:        opts.Model,
		SystemPrompt: ollamaprompts.VisionCritiqueJSON,
		UserMessage:  userMsg,
		ProjectDir:   opts.ProjectDir,
		Stage:        opts.Stage,
		Temperature:  opts.Temperature,
		NumCtx:       opts.NumCtx,
		Stream:       opts.Stream,
		Format:       VisionCritiqueSchemaJSON,
	})
	if err != nil {
		logVisionStep(opts, "vision_critique", "error", err.Error(), ollamaprompts.VisionCritiqueJSON, userMsg, raw, time.Since(stepStart))
		return VisionCritiqueOutput{}, tokens, err
	}
	var out VisionCritiqueOutput
	if err := unmarshalVisionJSON(raw, &out); err != nil {
		logVisionStep(opts, "vision_critique", "error", "parse: "+err.Error(), ollamaprompts.VisionCritiqueJSON, userMsg, raw, time.Since(stepStart))
		return VisionCritiqueOutput{}, tokens, fmt.Errorf("critique: %w", err)
	}
	verdict := "approved"
	if !out.Approved {
		verdict = fmt.Sprintf("rejected (%d issues)", len(out.Issues))
	}
	logVisionStep(opts, "vision_critique", "ok", verdict, ollamaprompts.VisionCritiqueJSON, userMsg, raw, time.Since(stepStart))
	return out, tokens, nil
}

// visionRepair reinvokes the draft prompt with an explicit list of issues to fix.
func visionRepair(ctx context.Context, opts VisionTurnOptions, sections VisionSections, issues []string) (VisionDraftOutput, int, error) {
	stepStart := time.Now()
	payload, _ := json.MarshalIndent(sections, "", "  ")
	issueList := "- " + strings.Join(issues, "\n- ")
	msg := "The previous draft has these issues:\n" + issueList +
		"\n\nHere is the previous JSON:\n" + string(payload) +
		"\n\nProduce a corrected JSON following the same schema with mode=\"refine\". Every section MUST be populated."
	raw, tokens, err := collectChat(ctx, opts.Executor, ChatExecutorRequest{
		Model:        opts.Model,
		SystemPrompt: ollamaprompts.VisionDraftJSON,
		History:      opts.History,
		UserMessage:  msg,
		ProjectDir:   opts.ProjectDir,
		Stage:        opts.Stage,
		Temperature:  opts.Temperature,
		NumCtx:       opts.NumCtx,
		Stream:       opts.Stream,
		Format:       VisionDraftSchemaJSON,
	})
	if err != nil {
		logVisionStep(opts, "vision_repair", "error", err.Error(), ollamaprompts.VisionDraftJSON, msg, raw, time.Since(stepStart))
		return VisionDraftOutput{}, tokens, err
	}
	var out VisionDraftOutput
	if err := unmarshalVisionJSON(raw, &out); err != nil {
		logVisionStep(opts, "vision_repair", "error", "parse: "+err.Error(), ollamaprompts.VisionDraftJSON, msg, raw, time.Since(stepStart))
		return VisionDraftOutput{}, tokens, fmt.Errorf("repair: %w", err)
	}
	logVisionStep(opts, "vision_repair", "ok", "mode="+out.Mode, ollamaprompts.VisionDraftJSON, msg, raw, time.Since(stepStart))
	return out, tokens, nil
}

// unmarshalVisionJSON extracts a JSON object from raw and unmarshals into v.
// Uses extractJSONPlan so the four fallback strategies (<jsonplan> tags, ```json
// code fence, line-based, aggressive outermost) all apply — matching the
// robustness the rest of the codebase gets for free.
func unmarshalVisionJSON(raw string, v any) error {
	data := extractJSONPlan(raw)
	if data == nil {
		return fmt.Errorf("no JSON object found in response")
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	return nil
}

// collectChat drains a ChatExecutor stream and returns the full assistant text
// and accumulated token count. Prefers the done event's Content when set
// (Claude CLI behaviour) and falls back to accumulated chunk content otherwise
// (Ollama behaviour).
func collectChat(ctx context.Context, exec ChatExecutor, req ChatExecutorRequest) (string, int, error) {
	events, err := exec.Chat(ctx, req)
	if err != nil {
		return "", 0, err
	}
	var buf strings.Builder
	var tokens int
	var provErr string
	for e := range events {
		switch e.Type {
		case "chunk":
			buf.WriteString(e.Content)
		case "tokens":
			var n int
			fmt.Sscanf(e.Content, "%d", &n)
			tokens += n
		case "error":
			provErr = e.Content
		case "done":
			if e.Content != "" {
				return e.Content, tokens, nil
			}
			if buf.Len() == 0 && provErr != "" {
				return "", tokens, fmt.Errorf("provider: %s", provErr)
			}
			return buf.String(), tokens, nil
		}
	}
	if buf.Len() == 0 && provErr != "" {
		return "", tokens, fmt.Errorf("provider: %s", provErr)
	}
	return buf.String(), tokens, nil
}

// wrapVisionEnvelope produces the XML envelope the caller's ExtractArtifact /
// StripArtifact pipeline expects. Mirrors the format Claude CLI emits.
func wrapVisionEnvelope(discussion, artifact string) string {
	var b strings.Builder
	b.WriteString("<!-- RESPONSE:START -->\n")
	b.WriteString("<discussion>")
	b.WriteString(discussion)
	b.WriteString("</discussion>\n")
	if artifact != "" {
		b.WriteString("<artifact>\n")
		b.WriteString(artifact)
		b.WriteString("</artifact>\n")
	}
	b.WriteString("<!-- RESPONSE:END -->\n")
	return b.String()
}

// emitFinalVisionTurn emits the discussion as a single chunk (so streaming UI
// shows it) and a done event with the full envelope (so ExtractArtifact works).
func emitFinalVisionTurn(ch chan<- StreamEvent, discussion, envelope string) {
	if strings.TrimSpace(discussion) != "" {
		ch <- StreamEvent{Type: "chunk", Content: discussion}
	}
	ch <- StreamEvent{Type: "done", Content: envelope}
}

func emitLog(ch chan<- StreamEvent, msg string) {
	ch <- StreamEvent{Type: "log", Content: msg}
}

func emitTokens(ch chan<- StreamEvent, n int) {
	if n <= 0 {
		return
	}
	ch <- StreamEvent{Type: "tokens", Content: fmt.Sprintf("%d", n), Tokens: n}
}

func emitError(ch chan<- StreamEvent, msg string) {
	ch <- StreamEvent{Type: "error", Content: msg}
	ch <- StreamEvent{Type: "done"}
}
