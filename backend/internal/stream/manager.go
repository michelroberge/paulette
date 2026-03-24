package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/michelroberge/Claudine/backend/internal/agent"
)

// Run represents a managed agent operation whose events are buffered for reconnection.
type Run struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"projectId"`
	Stage     string    `json:"stage"`
	Operation string    `json:"operation"` // "chat", "mock", "beads-generate", "beads-execute"
	StartedAt time.Time `json:"startedAt"`

	mu      sync.Mutex
	events  []agent.StreamEvent
	subs    map[int]chan agent.StreamEvent
	nextSub int
	done    bool
	ctx     context.Context
	cancel  context.CancelFunc
}

// RunInfo is the JSON-serializable subset of Run for the activity endpoint.
type RunInfo struct {
	ID         string `json:"id"`
	ProjectID  string `json:"projectId"`
	Stage      string `json:"stage"`
	Operation  string `json:"operation"`
	StartedAt  string `json:"startedAt"`
	EventCount int    `json:"eventCount"`
	Done       bool   `json:"done"`
}

// Manager tracks active and recently-finished runs.
type Manager struct {
	mu     sync.Mutex
	runs   map[string]*Run // key: "projectID:stage:operation"
	byID   map[string]*Run // key: run ID
	nextID int
}

// NewManager creates a new stream manager.
func NewManager() *Manager {
	return &Manager{
		runs: make(map[string]*Run),
		byID: make(map[string]*Run),
	}
}

func runKey(projectID, stage, operation string) string {
	return projectID + ":" + stage + ":" + operation
}

// Start creates a new run. If one already exists for this key, it returns nil
// (caller should check and reconnect instead).
func (m *Manager) Start(projectID, stage, operation string) *Run {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := runKey(projectID, stage, operation)

	// If there's an existing active run, don't start a new one
	if existing, ok := m.runs[key]; ok && !existing.isDone() {
		return nil
	}

	m.nextID++
	ctx, cancel := context.WithCancel(context.Background())
	run := &Run{
		ID:        fmt.Sprintf("run-%d", m.nextID),
		ProjectID: projectID,
		Stage:     stage,
		Operation: operation,
		StartedAt: time.Now(),
		subs:      make(map[int]chan agent.StreamEvent),
		ctx:       ctx,
		cancel:    cancel,
	}

	m.runs[key] = run
	m.byID[run.ID] = run

	return run
}

// Active returns the active (not done) run for a given key, or nil.
func (m *Manager) Active(projectID, stage, operation string) *Run {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := runKey(projectID, stage, operation)
	run, ok := m.runs[key]
	if !ok || run.isDone() {
		return nil
	}
	return run
}

// Get returns a run by ID (active or recently finished).
func (m *Manager) Get(id string) *Run {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.byID[id]
}

// ActiveForProject returns all active runs for a project.
func (m *Manager) ActiveForProject(projectID string) []RunInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []RunInfo
	for _, run := range m.runs {
		if run.ProjectID == projectID && !run.isDone() {
			run.mu.Lock()
			info := RunInfo{
				ID:         run.ID,
				ProjectID:  run.ProjectID,
				Stage:      run.Stage,
				Operation:  run.Operation,
				StartedAt:  run.StartedAt.Format(time.RFC3339),
				EventCount: len(run.events),
				Done:       run.done,
			}
			run.mu.Unlock()
			result = append(result, info)
		}
	}
	return result
}

// CancelAll cancels every active run, killing their agent subprocesses.
func (m *Manager) CancelAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, run := range m.runs {
		if !run.isDone() {
			run.cancel()
		}
	}
}

func (m *Manager) cleanup(key, id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Only remove if it's still the same run
	if r, ok := m.runs[key]; ok && r.ID == id {
		delete(m.runs, key)
	}
	delete(m.byID, id)
}

// Context returns the run's context (use this for agent subprocesses, not r.Context()).
func (r *Run) Context() context.Context {
	return r.ctx
}

// Emit buffers an event and broadcasts it to all subscribers.
func (r *Run) Emit(event agent.StreamEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = append(r.events, event)
	for _, ch := range r.subs {
		select {
		case ch <- event:
		default:
			// slow subscriber — drop event
		}
	}
}

// Finish marks the run as done, closes all subscriber channels, and schedules cleanup.
func (r *Run) Finish(m *Manager) {
	r.mu.Lock()
	r.done = true
	for id, ch := range r.subs {
		close(ch)
		delete(r.subs, id)
	}
	r.mu.Unlock()

	// Keep the run around for 2 minutes so a reconnecting client can read buffered events
	key := runKey(r.ProjectID, r.Stage, r.Operation)
	time.AfterFunc(2*time.Minute, func() {
		m.cleanup(key, r.ID)
	})
}

// Cancel stops the run's context (kills the agent subprocess).
func (r *Run) Cancel() {
	r.cancel()
}

func (r *Run) isDone() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.done
}

// Subscribe returns a channel that receives events starting from the given index.
// Buffered events from [fromIndex:] are sent immediately, then live events follow.
// The channel is closed when the run finishes.
func (r *Run) Subscribe(fromIndex int) (<-chan agent.StreamEvent, int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ch := make(chan agent.StreamEvent, 256)

	// Send buffered events
	if fromIndex < len(r.events) {
		for _, ev := range r.events[fromIndex:] {
			ch <- ev
		}
	}

	if r.done {
		close(ch)
		return ch, len(r.events)
	}

	subID := r.nextSub
	r.nextSub++
	r.subs[subID] = ch

	return ch, len(r.events)
}

// Unsubscribe removes a subscriber. Called when the HTTP client disconnects.
func (r *Run) Unsubscribe(ch <-chan agent.StreamEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, sub := range r.subs {
		if sub == ch {
			delete(r.subs, id)
			// Don't close the channel here — the subscriber is reading from it
			return
		}
	}
}

// StreamTo writes buffered + live events as SSE to the HTTP response.
// It blocks until the run finishes or the client disconnects.
func (r *Run) StreamTo(w http.ResponseWriter, req *http.Request, fromIndex int) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, _ := r.Subscribe(fromIndex)
	defer r.Unsubscribe(ch)

	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return // run finished
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-req.Context().Done():
			return // client disconnected
		}
	}
}

// Info returns the JSON-serializable info for this run.
func (r *Run) Info() RunInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	return RunInfo{
		ID:         r.ID,
		ProjectID:  r.ProjectID,
		Stage:      r.Stage,
		Operation:  r.Operation,
		StartedAt:  r.StartedAt.Format(time.RFC3339),
		EventCount: len(r.events),
		Done:       r.done,
	}
}
