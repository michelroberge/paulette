package fs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/michelroberge/paulette/backend/internal/model"
)

const sessionsFile = "sessions.json"

// SessionWriter owns all writes to sessions.json via a single goroutine.
// Handlers call Append (non-blocking, sends to a buffered channel).
type SessionWriter struct {
	ch   chan model.Session
	done chan struct{}
	dir  string // .paulette directory
}

// sessionWriters tracks one SessionWriter per project host directory.
var (
	sessionWritersMu sync.Mutex
	sessionWriters   = make(map[string]*SessionWriter)
)

// GetSessionWriter returns (or creates) the singleton SessionWriter for a project.
func GetSessionWriter(hostDir string) *SessionWriter {
	dir := filepath.Join(hostDir, ".paulette")

	sessionWritersMu.Lock()
	defer sessionWritersMu.Unlock()

	if sw, ok := sessionWriters[dir]; ok {
		return sw
	}

	sw := &SessionWriter{
		ch:   make(chan model.Session, 64),
		done: make(chan struct{}),
		dir:  dir,
	}
	go sw.run()
	sessionWriters[dir] = sw
	return sw
}

// Append enqueues a session record for writing. Non-blocking.
func (sw *SessionWriter) Append(s model.Session) {
	sw.ch <- s
}

// Close drains remaining sessions and shuts down the writer goroutine.
func (sw *SessionWriter) Close() {
	close(sw.ch)
	<-sw.done
}

func (sw *SessionWriter) run() {
	defer close(sw.done)
	for s := range sw.ch {
		sw.appendToDisk(s)
	}
}

func (sw *SessionWriter) appendToDisk(s model.Session) {
	p := filepath.Join(sw.dir, sessionsFile)

	var log model.SessionLog
	if data, err := os.ReadFile(p); err == nil {
		json.Unmarshal(data, &log) // ignore error — start fresh on corrupt
	}

	log.Sessions = append(log.Sessions, s)

	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(sw.dir, 0755)
	os.WriteFile(p, data, 0644)
}

// ReadSessions loads all session records for a project. Safe to call concurrently
// with Append — the writer serializes all writes so reads see consistent state.
func ReadSessions(hostDir string) ([]model.Session, error) {
	p := filepath.Join(hostDir, ".paulette", sessionsFile)
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var log model.SessionLog
	if err := json.Unmarshal(data, &log); err != nil {
		return nil, err
	}
	return log.Sessions, nil
}

// ReadSessionsByStage returns sessions filtered by stage.
func ReadSessionsByStage(hostDir string, stage model.StageName) ([]model.Session, error) {
	all, err := ReadSessions(hostDir)
	if err != nil {
		return nil, err
	}
	var filtered []model.Session
	for _, s := range all {
		if s.Stage == stage {
			filtered = append(filtered, s)
		}
	}
	return filtered, nil
}

// BuildSessionSummary computes aggregates from a session list.
func BuildSessionSummary(sessions []model.Session) model.SessionSummary {
	summary := model.SessionSummary{
		Sessions: sessions,
		ByStage:  make(map[model.StageName]model.StageSummary),
	}
	if summary.Sessions == nil {
		summary.Sessions = []model.Session{}
	}
	for _, s := range sessions {
		ss := summary.ByStage[s.Stage]
		ss.Count++
		ss.Tokens += s.TotalTokens
		summary.ByStage[s.Stage] = ss
		summary.GrandTotal += s.TotalTokens
	}
	return summary
}
