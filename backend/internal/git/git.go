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

// BranchList holds all local branches and the currently checked-out branch.
type BranchList struct {
	Branches []string `json:"branches"`
	Current  string   `json:"current"`
}

// Status represents the working tree and remote state.
type Status struct {
	Clean        bool   `json:"clean"`
	Dirty        int    `json:"dirty"`
	HasRemote    bool   `json:"hasRemote"`
	RemoteURL    string `json:"remoteUrl"`
	Branch       string `json:"branch"`
	RemoteBranch string `json:"remoteBranch"`
	GitUserName  string `json:"gitUserName"`
	GitUserEmail string `json:"gitUserEmail"`
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

// ListBranches returns all local branch names and the currently checked-out branch.
func (s *Service) ListBranches(dir string) (BranchList, error) {
	mu := s.lock(dir)
	mu.Lock()
	defer mu.Unlock()

	out, err := run(dir, "branch", "--format=%(refname:short)")
	if err != nil {
		return BranchList{}, err
	}
	current, _ := run(dir, "branch", "--show-current")
	var branches []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			branches = append(branches, line)
		}
	}
	return BranchList{Branches: branches, Current: strings.TrimSpace(current)}, nil
}

// Log returns commits with optional pagination and branch filter.
func (s *Service) Log(dir string, limit, offset int, branch string) ([]CommitEntry, error) {
	mu := s.lock(dir)
	mu.Lock()
	defer mu.Unlock()

	if limit <= 0 {
		limit = 10
	}
	// Format: hash<SEP>shortHash<SEP>author<SEP>date<SEP>message<SEP>refs
	sep := "<SEP>"
	format := fmt.Sprintf("%%H%s%%h%s%%an%s%%aI%s%%s%s%%D", sep, sep, sep, sep, sep)
	args := []string{"log", fmt.Sprintf("--max-count=%d", limit), fmt.Sprintf("--skip=%d", offset), fmt.Sprintf("--format=%s", format)}
	if branch != "" {
		args = append(args, branch)
	}
	out, err := run(dir, args...)
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

	branch := ""
	if b, err := run(dir, "branch", "--show-current"); err == nil {
		branch = b
	}

	remoteBranch := ""
	if hasRemote {
		if out, err := run(dir, "ls-remote", "--symref", "origin", "HEAD"); err == nil {
			// output: "ref: refs/heads/main\tHEAD\n..."
			for _, line := range strings.Split(out, "\n") {
				if strings.HasPrefix(line, "ref: refs/heads/") {
					// strip the trailing \tHEAD before trimming the prefix
					ref := strings.SplitN(line, "\t", 2)[0]
					remoteBranch = strings.TrimPrefix(ref, "ref: refs/heads/")
					break
				}
			}
		}
	}

	userName, _ := run(dir, "config", "user.name")
	userEmail, _ := run(dir, "config", "user.email")

	return Status{
		Clean:        dirty == 0,
		Dirty:        dirty,
		HasRemote:    hasRemote,
		RemoteURL:    remoteURL,
		Branch:       branch,
		RemoteBranch: remoteBranch,
		GitUserName:  userName,
		GitUserEmail: userEmail,
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

// RenameLocalBranch renames the current local branch.
func (s *Service) RenameLocalBranch(dir, newName string) error {
	mu := s.lock(dir)
	mu.Lock()
	defer mu.Unlock()
	_, err := run(dir, "branch", "-m", newName)
	return err
}

// SetIdentity sets user.name and user.email in the local repo config.
func (s *Service) SetIdentity(dir, name, email string) error {
	mu := s.lock(dir)
	mu.Lock()
	defer mu.Unlock()
	if _, err := run(dir, "config", "user.name", name); err != nil {
		return err
	}
	_, err := run(dir, "config", "user.email", email)
	return err
}

// Push pushes localBranch to remoteBranch on origin.
// If remoteBranch is empty it defaults to localBranch (or HEAD).
// Uses a refspec (local:remote) when the names differ.
func (s *Service) Push(dir string, localBranch, remoteBranch string, force bool) error {
	mu := s.lock(dir)
	mu.Lock()
	defer mu.Unlock()
	if localBranch == "" {
		localBranch = "HEAD"
	}
	if remoteBranch == "" {
		remoteBranch = localBranch
	}
	refspec := localBranch
	if localBranch != remoteBranch {
		refspec = localBranch + ":" + remoteBranch
	}
	args := []string{"push", "-u"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, "origin", refspec, "--tags")
	_, err := run(dir, args...)
	return err
}

// SSHPublicKey returns the container's SSH public key, generating an ed25519
// key pair if one does not already exist. It also ensures github.com is in
// known_hosts so that host key verification doesn't block pushes.
func (s *Service) SSHPublicKey() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	sshDir := filepath.Join(home, ".ssh")
	keyPath := filepath.Join(sshDir, "id_ed25519")
	pubKeyPath := keyPath + ".pub"
	knownHostsPath := filepath.Join(sshDir, "known_hosts")

	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return "", err
	}

	if _, err := os.Stat(pubKeyPath); os.IsNotExist(err) {
		out, err := exec.Command("ssh-keygen", "-t", "ed25519", "-C", "paulette", "-f", keyPath, "-N", "").CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("ssh-keygen: %w: %s", err, out)
		}
	}

	// Ensure github.com host key is trusted so pushes don't fail with
	// "Host key verification failed".
	if err := ensureKnownHost(knownHostsPath, "github.com"); err != nil {
		return "", fmt.Errorf("known_hosts: %w", err)
	}

	data, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// ensureKnownHost adds host's keys to knownHostsPath if not already present.
func ensureKnownHost(knownHostsPath, host string) error {
	// Check if host is already in known_hosts
	existing, _ := os.ReadFile(knownHostsPath)
	if strings.Contains(string(existing), host) {
		return nil
	}
	out, err := exec.Command("ssh-keyscan", "-H", host).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ssh-keyscan %s: %w: %s", host, err, out)
	}
	f, err := os.OpenFile(knownHostsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(out)
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

// FileDiff returns the diff of a single file between fromRef and the working tree.
// If fromRef is empty, diffs against HEAD.
// Returns original (at fromRef) and modified (current on disk) content.
func (s *Service) FileDiff(dir, filePath, fromRef string) (original, modified string, err error) {
	// Read current (modified) content from disk
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
		fromRef = "HEAD"
	}

	origOut, origErr := run(dir, "show", fromRef+":"+filePath)
	if origErr != nil {
		original = ""
	} else {
		original = origOut
	}

	return original, modified, nil
}

// DiffEntry holds the diff of a single file against HEAD.
type DiffEntry struct {
	Path     string `json:"path"`
	Original string `json:"original"`
	Modified string `json:"modified"`
}

// WorkingDiff returns all uncommitted changes (staged + unstaged + untracked)
// as a slice of DiffEntry, each containing HEAD content vs current disk content.
func (s *Service) WorkingDiff(dir string) ([]DiffEntry, error) {
	mu := s.lock(dir)
	mu.Lock()
	defer mu.Unlock()

	out, err := run(dir, "status", "--porcelain")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return []DiffEntry{}, nil
	}

	var entries []DiffEntry
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}
		xy := line[:2]
		filePath := strings.TrimSpace(line[3:])
		// Handle renames: "R old -> new" or "R old\tnew"
		if idx := strings.Index(filePath, " -> "); idx >= 0 {
			filePath = filePath[idx+4:]
		} else if idx := strings.Index(filePath, "\t"); idx >= 0 {
			filePath = filePath[idx+1:]
		}

		isDeleted := strings.Contains(xy, "D")
		isNew := xy == "??" || xy == "A " || xy == " A"

		var original, modified string
		if isNew {
			// Untracked / newly added: no HEAD version
			original = ""
			modBytes, readErr := os.ReadFile(filepath.Join(dir, filePath))
			if readErr == nil {
				modified = string(modBytes)
			}
		} else if isDeleted {
			// Deleted: no disk version
			modified = ""
			origOut, _ := run(dir, "show", "HEAD:"+filePath)
			original = origOut
		} else {
			// Modified: read HEAD and disk
			origOut, _ := run(dir, "show", "HEAD:"+filePath)
			original = origOut
			modBytes, readErr := os.ReadFile(filepath.Join(dir, filePath))
			if readErr == nil {
				modified = string(modBytes)
			}
		}

		entries = append(entries, DiffEntry{
			Path:     filePath,
			Original: original,
			Modified: modified,
		})
	}
	return entries, nil
}

// FileDiffBetweenRefs returns the content of a file at two git refs.
// Use this for stable diffs when both commits are known (e.g. pre/post bead execution).
func (s *Service) FileDiffBetweenRefs(dir, filePath, fromRef, toRef string) (original, modified string, err error) {
	origOut, origErr := run(dir, "show", fromRef+":"+filePath)
	if origErr != nil {
		original = ""
	} else {
		original = origOut
	}

	modOut, modErr := run(dir, "show", toRef+":"+filePath)
	if modErr != nil {
		modified = ""
	} else {
		modified = modOut
	}

	return original, modified, nil
}
