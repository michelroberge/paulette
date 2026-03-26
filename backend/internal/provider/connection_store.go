package provider

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	// Ignore load errors on startup — a missing file is expected on first run.
	_ = s.loadFromDisk()
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
func (s *ConnectionStore) Create(conn Connection) (*Connection, error) {
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
// to the current UTC time. Returns an error if the connection does not exist.
func (s *ConnectionStore) Update(conn Connection) (*Connection, error) {
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
// It returns a slice of stage names whose global-default assignments referenced
// the deleted connection; callers should surface this list to the user.
//
// Stage reference cleanup is delegated to StageConfigStore.ClearConnectionReferences
// which is called by the handler layer.  Delete itself returns nil for the
// affected-stages slice and lets the handler compose the full response.
func (s *ConnectionStore) Delete(id string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, conn := range s.connections {
		if conn.ID == id {
			// Snapshot for rollback.
			original := s.snapshot()
			s.connections = append(s.connections[:i], s.connections[i+1:]...)

			if err := s.save(); err != nil {
				s.connections = original
				return nil, fmt.Errorf("persist connection delete: %w", err)
			}

			// Affected stage assignments are resolved by the handler via
			// StageConfigStore.ClearConnectionReferences; nothing to report here.
			return nil, nil
		}
	}

	return nil, fmt.Errorf("connection %q not found", id)
}

// ---------------------------------------------------------------------------
// Internal helpers (must be called with appropriate lock held, or before lock
// is needed).
// ---------------------------------------------------------------------------

// loadFromDisk reads the connections file into memory.
// A missing file is silently treated as an empty store.
// Must NOT be called while the lock is held.
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
