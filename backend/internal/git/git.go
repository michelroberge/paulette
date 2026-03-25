package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CommitEntry represents a single git log entry.
type CommitEntry struct {
	Hash      string   `json:"hash"`
	ShortHash string   `json:"shortHash"`
	Message   string   `json:"message"`
	Author    string   `json:"author"`
	Date      time.Time `json:"date"`
	Tags      []string `json:"tags"`
}

// Status represents the working tree and remote state.
type Status struct {
	Clean     bool   `json:"clean"`
	Dirty     int    `json:"dirty"`
	HasRemote bool   `json:"hasRemote"`
	RemoteURL string `json:"remoteUrl"`
}

// Service provides git operations with per-directory locking.
type Service struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func NewService() *Service {
	return &Service{locks: make(map[string]*sync.Mutex)}
}

func (s *Service) lock(dir string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.locks[dir]
	if !ok {
		m = &sync.Mutex{}
		s.locks[dir] = m
	}
	return m
}

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

// Clone clones a remote repository into dir.
// dir must not already exist (or be empty).
func (s *Service) Clone(url, dir string) error {
	out, err := exec.Command("git", "clone", url, dir).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone: %w: %s", err, out)
	}
	return nil
}

// Init initializes a git repository if one doesn't exist.
func (s *Service) Init(dir string) error {
	mu := s.lock(dir)
	mu.Lock()
	defer mu.Unlock()
	_, err := run(dir, "init")
	return err
}

// AddAndCommit stages the given paths and commits with the message.
func (s *Service) AddAndCommit(dir string, paths []string, message string) error {
	mu := s.lock(dir)
	mu.Lock()
	defer mu.Unlock()
	for _, p := range paths {
		if _, err := run(dir, "add", p); err != nil {
			return err
		}
	}
	_, err := run(dir, "commit", "-m", message)
	return err
}

// AddAllAndCommit stages all changes and commits.
func (s *Service) AddAllAndCommit(dir string, message string) error {
	mu := s.lock(dir)
	mu.Lock()
	defer mu.Unlock()
	if _, err := run(dir, "add", "-A"); err != nil {
		return err
	}
	_, err := run(dir, "commit", "-m", message)
	return err
}

// Log returns the most recent commits.
func (s *Service) Log(dir string, limit int) ([]CommitEntry, error) {
	mu := s.lock(dir)
	mu.Lock()
	defer mu.Unlock()

	if limit <= 0 {
		limit = 50
	}
	// Format: hash<SEP>shortHash<SEP>author<SEP>date<SEP>message<SEP>refs
	sep := "<SEP>"
	format := fmt.Sprintf("%%H%s%%h%s%%an%s%%aI%s%%s%s%%D", sep, sep, sep, sep, sep)
	out, err := run(dir, "log", fmt.Sprintf("--max-count=%d", limit), fmt.Sprintf("--format=%s", format))
	if err != nil {
		// Empty repo has no commits
		if strings.Contains(err.Error(), "does not have any commits") {
			return nil, nil
		}
		return nil, err
	}
	if out == "" {
		return nil, nil
	}

	lines := strings.Split(out, "\n")
	entries := make([]CommitEntry, 0, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(line, sep, 6)
		if len(parts) < 6 {
			continue
		}
		date, _ := time.Parse(time.RFC3339, parts[3])
		tags := parseTags(parts[5])
		entries = append(entries, CommitEntry{
			Hash:      parts[0],
			ShortHash: parts[1],
			Author:    parts[2],
			Date:      date,
			Message:   parts[4],
			Tags:      tags,
		})
	}
	return entries, nil
}

func parseTags(refs string) []string {
	if refs == "" {
		return nil
	}
	var tags []string
	for _, ref := range strings.Split(refs, ", ") {
		ref = strings.TrimSpace(ref)
		if strings.HasPrefix(ref, "tag: ") {
			tags = append(tags, strings.TrimPrefix(ref, "tag: "))
		}
	}
	return tags
}

// CreateTag creates an annotated tag.
func (s *Service) CreateTag(dir string, tag, message string) error {
	mu := s.lock(dir)
	mu.Lock()
	defer mu.Unlock()
	_, err := run(dir, "tag", "-a", tag, "-m", message)
	return err
}

// Status returns the working tree and remote status.
func (s *Service) Status(dir string) (Status, error) {
	mu := s.lock(dir)
	mu.Lock()
	defer mu.Unlock()

	out, err := run(dir, "status", "--porcelain")
	if err != nil {
		return Status{}, err
	}
	dirty := 0
	if out != "" {
		dirty = len(strings.Split(out, "\n"))
	}

	remoteURL := ""
	hasRemote := false
	if url, err := run(dir, "remote", "get-url", "origin"); err == nil {
		remoteURL = url
		hasRemote = true
	}

	return Status{
		Clean:     dirty == 0,
		Dirty:     dirty,
		HasRemote: hasRemote,
		RemoteURL: remoteURL,
	}, nil
}

// ResetHard resets the working tree to the given ref.
func (s *Service) ResetHard(dir string, ref string) error {
	mu := s.lock(dir)
	mu.Lock()
	defer mu.Unlock()
	_, err := run(dir, "reset", "--hard", ref)
	return err
}

// DiscardChanges discards all uncommitted changes.
func (s *Service) DiscardChanges(dir string) error {
	mu := s.lock(dir)
	mu.Lock()
	defer mu.Unlock()
	// Reset staged changes
	run(dir, "reset", "HEAD")
	// Discard working tree changes
	if _, err := run(dir, "checkout", "--", "."); err != nil {
		return err
	}
	// Clean untracked files
	_, err := run(dir, "clean", "-fd")
	return err
}

// RemoteGet returns the origin remote URL, or empty string if none.
func (s *Service) RemoteGet(dir string) string {
	mu := s.lock(dir)
	mu.Lock()
	defer mu.Unlock()
	url, err := run(dir, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	return url
}

// RemoteSet sets or adds the origin remote URL.
func (s *Service) RemoteSet(dir string, url string) error {
	mu := s.lock(dir)
	mu.Lock()
	defer mu.Unlock()
	// Try set-url first (works if remote exists)
	if _, err := run(dir, "remote", "set-url", "origin", url); err != nil {
		// If that fails, add it
		_, err = run(dir, "remote", "add", "origin", url)
		return err
	}
	return nil
}

// RemoteRemove removes the origin remote.
func (s *Service) RemoteRemove(dir string) error {
	mu := s.lock(dir)
	mu.Lock()
	defer mu.Unlock()
	_, err := run(dir, "remote", "remove", "origin")
	return err
}

// Push pushes the current branch and tags to origin.
func (s *Service) Push(dir string) error {
	mu := s.lock(dir)
	mu.Lock()
	defer mu.Unlock()
	_, err := run(dir, "push", "origin", "HEAD", "--tags")
	return err
}

// Pull pulls from origin.
func (s *Service) Pull(dir string) error {
	mu := s.lock(dir)
	mu.Lock()
	defer mu.Unlock()
	_, err := run(dir, "pull", "origin")
	return err
}

// CurrentHash returns the current HEAD commit hash, or empty string if repo has no commits.
func (s *Service) CurrentHash(dir string) string {
	out, err := run(dir, "rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	return out
}

// FileDiff returns the diff of a single file between fromRef and HEAD.
// If fromRef is empty, diffs against the index (uncommitted changes).
// Returns original and modified content.
func (s *Service) FileDiff(dir, filePath, fromRef string) (original, modified string, err error) {
	// Read current (modified) content
	modBytes, readErr := os.ReadFile(filepath.Join(dir, filePath))
	if readErr != nil {
		if os.IsNotExist(readErr) {
			modified = ""
		} else {
			return "", "", readErr
		}
	} else {
		modified = string(modBytes)
	}

	if fromRef == "" {
		// No ref: original is HEAD version
		fromRef = "HEAD"
	}

	origOut, origErr := run(dir, "show", fromRef+":"+filePath)
	if origErr != nil {
		// File didn't exist at that ref
		original = ""
	} else {
		original = origOut
	}

	return original, modified, nil
}
