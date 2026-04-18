package refinement

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/michelroberge/paulette/backend/internal/model"
	fsrepo "github.com/michelroberge/paulette/backend/internal/repository/fs"
)

// StepTrace records a single LLM interaction with full context.
type StepTrace struct {
	Step         string  `json:"step"`                   // e.g. "summarize", "classify_fact_2"
	Phase        string  `json:"phase"`                  // loop phase
	Iteration    int     `json:"iteration"`              // loop iteration counter
	Timestamp    string  `json:"timestamp"`              // ISO 8601
	SystemPrompt string  `json:"systemPrompt"`           // full system prompt sent
	UserMessage  string  `json:"userMessage"`            // full user message sent
	RawResponse  string  `json:"rawResponse"`            // full raw LLM response
	Tokens       int     `json:"tokens"`                 // token count for this call
	DurationMs   int64   `json:"durationMs"`             // wall-clock duration
	Error        string  `json:"error,omitempty"`        // error if any
	ModelID      string  `json:"modelId,omitempty"`      // which model was used
	ParsedResult string  `json:"parsedResult,omitempty"` // what the parser extracted
}

// Trace collects all LLM interactions for a refinement run and writes them
// to the run log directory for full debuggability.
type Trace struct {
	logBase     string
	projectName string
	runID       string
	modelID     string

	mu    sync.Mutex
	steps []StepTrace
	refs  []model.StepLogRef
}

// NewTrace creates a trace context. Returns nil if logging is disabled.
func NewTrace(logBase, projectName, runID, modelID string) *Trace {
	if logBase == "" || runID == "" {
		return nil
	}
	return &Trace{
		logBase:     logBase,
		projectName: fsrepo.SanitizeProjectName(projectName),
		runID:       runID,
		modelID:     modelID,
	}
}

// LogCall records a completed LLM call with full input/output.
func (t *Trace) LogCall(step, phase string, iteration int, systemPrompt, userMessage, rawResponse, parsedResult string, tokens int, dur time.Duration, err error) {
	if t == nil {
		return
	}

	entry := StepTrace{
		Step:         step,
		Phase:        phase,
		Iteration:    iteration,
		Timestamp:    time.Now().Format(time.RFC3339Nano),
		SystemPrompt: systemPrompt,
		UserMessage:  userMessage,
		RawResponse:  rawResponse,
		Tokens:       tokens,
		DurationMs:   dur.Milliseconds(),
		ModelID:      t.modelID,
		ParsedResult: parsedResult,
	}
	if err != nil {
		entry.Error = err.Error()
	}

	// Write individual step file as JSON for easy inspection.
	filename := t.writeStepFile(step, entry)

	status := "ok"
	detail := fmt.Sprintf("tokens=%d resp=%d chars", tokens, len(rawResponse))
	if err != nil {
		status = "error"
		detail = err.Error()
	}

	t.mu.Lock()
	t.steps = append(t.steps, entry)
	if filename != "" {
		t.refs = append(t.refs, model.StepLogRef{
			Step:     step,
			File:     filename,
			Status:   status,
			Detail:   detail,
			Duration: dur.Milliseconds(),
		})
	}
	t.mu.Unlock()
}

// writeStepFile writes a single step's trace as a JSON file in the run directory.
func (t *Trace) writeStepFile(step string, entry StepTrace) string {
	dir := filepath.Join(t.logBase, t.projectName, "runs", t.runID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("trace: mkdir %s: %v", dir, err)
		return ""
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		log.Printf("trace: marshal %s: %v", step, err)
		return ""
	}

	// Use a unique filename to avoid collisions (same step name runs multiple times).
	filename := fmt.Sprintf("%s_%s.json", step, time.Now().Format("150405.000"))
	fpath := filepath.Join(dir, filename)
	if err := os.WriteFile(fpath, data, 0o644); err != nil {
		log.Printf("trace: write %s: %v", fpath, err)
		return ""
	}
	return filename
}

// Flush writes the accumulated step references into the run's meta.json.
func (t *Trace) Flush() {
	if t == nil {
		return
	}
	t.mu.Lock()
	refs := make([]model.StepLogRef, len(t.refs))
	copy(refs, t.refs)
	t.mu.Unlock()

	if len(refs) == 0 {
		return
	}

	entry, err := fsrepo.ReadRunMeta(t.logBase, t.projectName, t.runID)
	if err != nil {
		log.Printf("trace: flush %s: %v", t.runID, err)
		return
	}
	entry.StepLogs = refs
	if err := fsrepo.WriteRunMeta(t.logBase, entry); err != nil {
		log.Printf("trace: flush write %s: %v", t.runID, err)
	}
}

// Summary returns a human-readable summary of all traced calls.
func (t *Trace) Summary() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	var totalTokens int
	var totalDur int64
	for _, s := range t.steps {
		totalTokens += s.Tokens
		totalDur += s.DurationMs
	}
	return fmt.Sprintf("%d LLM calls, %d tokens, %dms total", len(t.steps), totalTokens, totalDur)
}
