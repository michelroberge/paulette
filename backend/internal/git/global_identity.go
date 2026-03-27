package git

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// GlobalIdentity holds the user's name and email used as defaults for all
// new projects. Stored at ~/.paulette/git.json with chmod 0600.
type GlobalIdentity struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// GlobalIdentityStore persists the global git identity.
type GlobalIdentityStore struct {
	path string
	mu   sync.RWMutex
	data GlobalIdentity
}

// NewGlobalIdentityStore creates a store backed by registryPath/git.json.
// A missing file is treated as an empty identity (no error).
func NewGlobalIdentityStore(registryPath string) *GlobalIdentityStore {
	s := &GlobalIdentityStore{
		path: filepath.Join(registryPath, "git.json"),
	}
	if err := s.load(); err != nil {
		log.Printf("[paulette] WARNING: could not load %s: %v — starting with empty git identity", s.path, err)
	}
	return s
}

// Get returns the current global identity.
func (s *GlobalIdentityStore) Get() GlobalIdentity {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data
}

// Set saves name and email to disk atomically with chmod 0600.
func (s *GlobalIdentityStore) Set(name, email string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data = GlobalIdentity{Name: name, Email: email}
	return s.save()
}

func (s *GlobalIdentityStore) load() error {
	raw, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil // first run — empty identity is fine
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, &s.data)
}

func (s *GlobalIdentityStore) save() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".tmp-git-*")
	if err != nil {
		return err
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
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if err := os.Chmod(tmpName, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return err
	}

	committed = true
	_ = os.Chmod(s.path, 0600)
	return nil
}
