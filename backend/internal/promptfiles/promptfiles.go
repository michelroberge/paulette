package promptfiles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// PromptMeta describes a prompt template file.
type PromptMeta struct {
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Description string   `json:"description"`
	Variables   []string `json:"variables"`
}

// PromptStore manages externalized prompt templates stored in
// {hostDir}/.paulette/prompts/ for a single project.
// An optional fallback store is consulted when a file is not found locally.
type PromptStore struct {
	promptDir string
	fallback  *PromptStore // optional: checked when a custom file is not found locally
}

// New creates a PromptStore rooted at {hostDir}/.paulette/prompts/.
func New(hostDir string) *PromptStore {
	return &PromptStore{
		promptDir: filepath.Join(hostDir, ".paulette", "prompts"),
	}
}

// NewRaw creates a PromptStore rooted at the given directory path directly.
// Used for connection-level prompts stored outside a project repo.
func NewRaw(dir string) *PromptStore {
	return &PromptStore{promptDir: dir}
}

// NewForConnection creates a PromptStore for a connection's prompt overrides.
// Stored at {registryPath}/connection-prompts/{connectionID}/.
func NewForConnection(registryPath, connectionID string) *PromptStore {
	return &PromptStore{
		promptDir: filepath.Join(registryPath, "connection-prompts", connectionID),
	}
}

// WithFallback returns a copy of the store with a fallback store set.
// When Load cannot find a custom file locally, it checks the fallback
// before returning the hardcoded default.
func (ps *PromptStore) WithFallback(fb *PromptStore) *PromptStore {
	return &PromptStore{
		promptDir: ps.promptDir,
		fallback:  fb,
	}
}

// Exists returns true if the prompts directory has been initialised.
func (ps *PromptStore) Exists() bool {
	info, err := os.Stat(ps.promptDir)
	return err == nil && info.IsDir()
}

// Init creates the prompts directory and writes all default templates.
// Existing files are NOT overwritten so user customisations are preserved.
func (ps *PromptStore) Init() error {
	if err := os.MkdirAll(ps.promptDir, 0755); err != nil {
		return fmt.Errorf("create prompts dir: %w", err)
	}
	for name, def := range Defaults {
		path := filepath.Join(ps.promptDir, name)
		if _, err := os.Stat(path); err == nil {
			continue // already exists — don't overwrite
		}
		if err := os.WriteFile(path, []byte(def.Content), 0644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}

// Load reads a raw template file by name. Resolution order:
//  1. Custom file on disk in this store's directory
//  2. Fallback store (if set) — custom file in the fallback's directory
//  3. Hardcoded default from Defaults map
func (ps *PromptStore) Load(name string) (string, error) {
	// Try local custom file first.
	path := filepath.Join(ps.promptDir, name)
	data, err := os.ReadFile(path)
	if err == nil {
		return string(data), nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	// Try fallback store (connection-level prompts).
	if ps.fallback != nil {
		if content, ok := ps.fallback.LoadCustomOnly(name); ok {
			return content, nil
		}
	}
	// Fall back to hardcoded default.
	if def, ok := Defaults[name]; ok {
		return def.Content, nil
	}
	return "", fmt.Errorf("unknown prompt: %s", name)
}

// Save writes (or overwrites) a template file.
func (ps *PromptStore) Save(name, content string) error {
	if _, ok := Defaults[name]; !ok {
		return fmt.Errorf("unknown prompt: %s", name)
	}
	if err := os.MkdirAll(ps.promptDir, 0755); err != nil {
		return fmt.Errorf("create prompts dir: %w", err)
	}
	path := filepath.Join(ps.promptDir, name)
	return os.WriteFile(path, []byte(content), 0644)
}

// Render loads a template, parses it with text/template, and executes it
// with the given data. Falls back to defaults when a file is missing.
func (ps *PromptStore) Render(name string, data map[string]any) (string, error) {
	raw, err := ps.Load(name)
	if err != nil {
		return "", err
	}
	// If data is nil or template has no directives, return raw.
	if data == nil || !strings.Contains(raw, "{{") {
		return raw, nil
	}
	tmpl, err := template.New(name).Option("missingkey=zero").Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse template %s: %w", name, err)
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		return "", fmt.Errorf("execute template %s: %w", name, err)
	}
	return sb.String(), nil
}

// List returns metadata for all known prompts.
func (ps *PromptStore) List() []PromptMeta {
	metas := make([]PromptMeta, 0, len(Defaults))
	for name, def := range Defaults {
		metas = append(metas, PromptMeta{
			Name:        name,
			Category:    def.Category,
			Description: def.Description,
			Variables:   def.Variables,
		})
	}
	return metas
}

// LoadCustomOnly reads a template file from disk. Returns ("", false) if the
// file does not exist — it does NOT fall back to defaults. Used by PromptChain
// to check if a store has a customized version before trying the next store.
func (ps *PromptStore) LoadCustomOnly(name string) (string, bool) {
	path := filepath.Join(ps.promptDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(data), true
}

// PromptChain tries multiple PromptStores in order, falling back through
// the chain until one has a custom file, then finally to hardcoded defaults.
// Typical chain: project store → connection store.
type PromptChain struct {
	stores []*PromptStore
}

// NewChain creates a PromptChain from the given stores (nil entries are skipped).
func NewChain(stores ...*PromptStore) *PromptChain {
	var valid []*PromptStore
	for _, s := range stores {
		if s != nil {
			valid = append(valid, s)
		}
	}
	return &PromptChain{stores: valid}
}

// Load returns the first custom file found in the chain, falling back to defaults.
func (pc *PromptChain) Load(name string) (string, error) {
	for _, s := range pc.stores {
		if content, ok := s.LoadCustomOnly(name); ok {
			return content, nil
		}
	}
	// Fall back to hardcoded default.
	if def, ok := Defaults[name]; ok {
		return def.Content, nil
	}
	return "", fmt.Errorf("unknown prompt: %s", name)
}

// Render loads through the chain and renders with text/template.
func (pc *PromptChain) Render(name string, data map[string]any) (string, error) {
	raw, err := pc.Load(name)
	if err != nil {
		return "", err
	}
	if data == nil || !strings.Contains(raw, "{{") {
		return raw, nil
	}
	tmpl, err := template.New(name).Option("missingkey=zero").Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse template %s: %w", name, err)
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		return "", fmt.Errorf("execute template %s: %w", name, err)
	}
	return sb.String(), nil
}

// Reset overwrites a single template file with the built-in default.
func (ps *PromptStore) Reset(name string) error {
	def, ok := Defaults[name]
	if !ok {
		return fmt.Errorf("unknown prompt: %s", name)
	}
	if err := os.MkdirAll(ps.promptDir, 0755); err != nil {
		return fmt.Errorf("create prompts dir: %w", err)
	}
	path := filepath.Join(ps.promptDir, name)
	return os.WriteFile(path, []byte(def.Content), 0644)
}
