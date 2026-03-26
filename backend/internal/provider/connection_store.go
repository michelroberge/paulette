package provider

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// connectionsEnvelope is the top-level wrapper persisted at
// ~/.paulette/connections.json. Using an envelope object (rather than a bare
// array) makes it easy to add top-level metadata in a future version without
// breaking the schema.
type connectionsEnvelope struct {
	Connections []Connection `json:"connections"`
}

// ConnectionStore manages CRUD for named LLM provider connections.
// All state is persisted to ~/.paulette/connections.json with chmod 0600 to
// protect credentials stored in plain text.
//
// An in-memory slice is kept in sync with the file on every mutation so reads
// never touch the filesystem (fast path for provider resolution during chat).
type ConnectionStore struct {
	// path is the absolute path to connections.json.
	path string
	mu   sync.RWMutex
	// connections is the in-memory authoritative state.
	connections []Connection
}

// NewConnectionStore creates a ConnectionStore backed by
// registryPath/connections.json (typically ~/.paulette/connections.json).
// Existing data is loaded eagerly; a missing file is treated as an empty store.
func NewConnectionStore(registryPath string) *ConnectionStore {
	s := &ConnectionStore{
		path:        filepath.Join(registryPath, "connections.json"),
		connections: []Connection{},
	}
	// A missing file is expected on first run and is silently ignored.
	// Any other error (e.g. corrupted JSON) is logged so operators can
	// diagnose unexpected data loss; the store starts empty in that case.
	if err := s.loadFromDisk(); err != nil {
		log.Printf("[paulette] WARNING: could not load %s: %v — starting with empty connection store", s.path, err)
	}
	return s
}

// ---------------------------------------------------------------------------
// Public CRUD interface
// ---------------------------------------------------------------------------

// List returns a snapshot of all stored connections.
// The slice is a defensive copy; callers may modify it freely.
func (s *ConnectionStore) List() []Connection {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Connection, len(s.connections))
	copy(result, s.connections)
	return result
}

// Get returns the connection with the given ID.
// Returns an error if no such connection exists.
func (s *ConnectionStore) Get(id string) (*Connection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for i := range s.connections {
		if s.connections[i].ID == id {
			conn := s.connections[i] // defensive copy
			return &conn, nil
		}
	}
	return nil, fmt.Errorf("connection %q not found", id)
}

// Create persists a new connection. A UUID is assigned to conn.ID; timestamps
// are set to the current UTC time. The backing file is written atomically and
// chmod'd to 0600.
//
// conn.ID, conn.CreatedAt, and conn.UpdatedAt are overwritten by this method.
// Returns an error if the connection's Name is empty or ProviderType is unknown.
func (s *ConnectionStore) Create(conn Connection) (*Connection, error) {
	if err := validateConnectionInput(&conn); err != nil {
		return nil, err
	}

	id, err := newUUIDv4()
	if err != nil {
		return nil, fmt.Errorf("generate connection ID: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	conn.ID = id
	conn.CreatedAt = now
	conn.UpdatedAt = now

	// Snapshot for rollback.
	original := s.snapshot()
	s.connections = append(s.connections, conn)

	if err := s.save(); err != nil {
		s.connections = original
		return nil, fmt.Errorf("persist connection: %w", err)
	}

	result := conn // defensive copy
	return &result, nil
}

// Update replaces the stored connection whose ID matches conn.ID.
// conn.CreatedAt is preserved from the existing record; conn.UpdatedAt is set
// to the current UTC time. Returns an error if the connection does not exist or
// if the updated fields fail validation (empty Name, unknown ProviderType).
func (s *ConnectionStore) Update(conn Connection) (*Connection, error) {
	if err := validateConnectionInput(&conn); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.connections {
		if s.connections[i].ID == conn.ID {
			// Snapshot for rollback.
			original := s.snapshot()

			conn.CreatedAt = s.connections[i].CreatedAt // immutable
			conn.UpdatedAt = time.Now().UTC()
			s.connections[i] = conn

			if err := s.save(); err != nil {
				s.connections = original
				return nil, fmt.Errorf("persist connection update: %w", err)
			}

			result := conn // defensive copy
			return &result, nil
		}
	}

	return nil, fmt.Errorf("connection %q not found", conn.ID)
}

// Delete removes the connection with the given ID and persists the change.
// It returns the deduplicated list of stage names (e.g. "vision", "build") whose
// global defaults or per-project overrides referenced the deleted connection.
//
// If stageConfig is non-nil, Delete calls
// stageConfig.ClearConnectionReferences(id, projectHostDirs) under its own
// write lock (the two stores use independent mutexes — no deadlock) BEFORE
// removing the connection from disk.  This ordering means a save() failure
// leaves the connection file intact (recoverable), while stage references are
// already cleared; users would see a connection with no stage assignments, which
// is a safe degraded state.
//
// Pass a nil stageConfig (and nil projectHostDirs) when stage cleanup is not
// desired (e.g. in tests).
func (s *ConnectionStore) Delete(id string, stageConfig *StageConfigStore, projectHostDirs []string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Locate the connection first; fail fast if it doesn't exist.
	idx := -1
	for i, conn := range s.connections {
		if conn.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("connection %q not found", id)
	}

	// Clear any stage assignments that reference this connection.
	// StageConfigStore.ClearConnectionReferences acquires its own mutex so
	// there is no deadlock risk here.
	var affectedStages []string
	if stageConfig != nil {
		var err error
		affectedStages, err = stageConfig.ClearConnectionReferences(id, projectHostDirs)
		if err != nil {
			return nil, fmt.Errorf("clear stage references for connection %q: %w", id, err)
		}
	}

	// Remove the connection and persist.
	original := s.snapshot()
	s.connections = append(s.connections[:idx], s.connections[idx+1:]...)
	if err := s.save(); err != nil {
		s.connections = original
		return nil, fmt.Errorf("persist connection delete: %w", err)
	}

	return affectedStages, nil
}

// ---------------------------------------------------------------------------
// Internal helpers (must be called with appropriate lock held, or before lock
// is needed).
// ---------------------------------------------------------------------------

// ReloadFromDisk re-reads connections.json and replaces the in-memory state.
// Useful when an operator edits the file externally.  Returns an error if the
// file exists but cannot be parsed; on error the in-memory state is unchanged.
func (s *ConnectionStore) ReloadFromDisk() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadFromDisk()
}

// loadFromDisk reads the connections file into memory.
// A missing file is silently treated as an empty store (expected on first run).
// A file that exists but cannot be parsed returns an error; the caller decides
// whether to log or propagate it — the in-memory state is not modified on error.
//
// Does not acquire s.mu; the caller is responsible for holding the write lock
// when concurrent access is possible (see ReloadFromDisk).
func (s *ConnectionStore) loadFromDisk() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // first run — start with an empty store
		}
		return fmt.Errorf("read connections file %s: %w", s.path, err)
	}

	var envelope connectionsEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		// Corrupted file — start with empty store rather than hard-failing.
		// Operators can restore from a backup; starting empty is safer than
		// crashing the whole server.
		return fmt.Errorf("parse connections file: %w", err)
	}

	if envelope.Connections == nil {
		envelope.Connections = []Connection{}
	}
	s.connections = envelope.Connections
	return nil
}

// save atomically writes the current in-memory state to disk and applies
// chmod 0600 to protect API keys stored in plain text.
// Must be called with s.mu held (write lock).
func (s *ConnectionStore) save() error {
	envelope := connectionsEnvelope{Connections: s.connections}

	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal connections: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create connections directory: %w", err)
	}

	// Write to a temp file in the same directory so the rename is on the same
	// filesystem and therefore atomic on POSIX systems.
	tmp, err := os.CreateTemp(dir, ".tmp-connections-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// chmod before rename so the sensitive data is never world-readable,
	// even transiently.
	if err := os.Chmod(tmpName, 0600); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("rename to %s: %w", s.path, err)
	}

	committed = true
	// Belt-and-suspenders: also chmod the final path in case the rename
	// preserved a different mode from the destination directory's umask.
	_ = os.Chmod(s.path, 0600)
	return nil
}

// snapshot returns a deep copy of the current connections slice for rollback.
// Must be called with s.mu held.
func (s *ConnectionStore) snapshot() []Connection {
	cp := make([]Connection, len(s.connections))
	copy(cp, s.connections)
	return cp
}

// ---------------------------------------------------------------------------
// Validation helpers
// ---------------------------------------------------------------------------

// validProviderTypes is the authoritative set of provider identifiers.
// It mirrors the constants declared in connection.go so that validation stays
// in sync with the type definitions at compile time.
var validProviderTypes = map[ProviderType]struct{}{
	ProviderOllama:        {},
	ProviderLMStudio:      {},
	ProviderAnthropic:     {},
	ProviderOpenAI:        {},
	ProviderGemini:        {},
	ProviderClaudeCLI:     {},
	ProviderGitHubCopilot: {},
}

// validateConnectionInput checks the user-supplied fields of a Connection
// before a Create or Update operation.  It returns a descriptive error when:
//   - Name is empty or whitespace-only
//   - ProviderType is not a recognised provider identifier
//   - DefaultModel is empty (unless ProviderType is claude_cli, which uses its
//     own hardcoded model map)
//
// It does NOT validate credentials — those are the user's responsibility.
func validateConnectionInput(conn *Connection) error {
	if strings.TrimSpace(conn.Name) == "" {
		return fmt.Errorf("connection name must not be empty")
	}
	if _, ok := validProviderTypes[conn.ProviderType]; !ok {
		return fmt.Errorf("unknown provider type %q; must be one of: ollama, lmstudio, anthropic, openai, gemini, claude_cli, github_copilot", conn.ProviderType)
	}
	// claude_cli uses a hardcoded model map — an empty DefaultModel is fine.
	if conn.ProviderType != ProviderClaudeCLI && strings.TrimSpace(conn.DefaultModel) == "" {
		return fmt.Errorf("defaultModel must not be empty for provider type %q", conn.ProviderType)
	}
	return nil
}

// ---------------------------------------------------------------------------
// UUID generation
// ---------------------------------------------------------------------------

// newUUIDv4 generates a random UUID (version 4, variant 2) using the OS
// cryptographically secure random source. No external dependency is needed.
func newUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	// Set version bits (version 4).
	b[6] = (b[6] & 0x0f) | 0x40
	// Set variant bits (variant 10xxxxxx).
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		b[0:4],
		b[4:6],
		b[6:8],
		b[8:10],
		b[10:16],
	), nil
}
