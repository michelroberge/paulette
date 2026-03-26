package provider

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/michelroberge/paulette/backend/internal/model"
)

// StageAssignment describes the connection and model to use for a pipeline stage.
type StageAssignment struct {
	ConnectionID string `json:"connectionId"`
	Model        string `json:"model"`
}

// GlobalConfig is the top-level structure persisted at ~/.paulette/config.json.
// It holds the global stage defaults that every project inherits unless overridden.
type GlobalConfig struct {
	StageDefaults map[model.StageName]StageAssignment `json:"stageDefaults"`
}

// ProjectStageConfig is persisted at <hostDir>/.paulette/stage_config.json.
// A nil pointer value in Overrides means the stage inherits the global default.
type ProjectStageConfig struct {
	Overrides map[model.StageName]*StageAssignment `json:"overrides"`
}

// StageConfigStore manages global stage defaults and per-project overrides.
// All writes are atomic (write to temp file, rename) to prevent partial writes.
type StageConfigStore struct {
	// globalPath is the path to ~/.paulette/config.json.
	globalPath string
	mu         sync.RWMutex
}

// NewStageConfigStore creates a StageConfigStore whose global config lives in
// registryPath/config.json (typically ~/.paulette/config.json).
func NewStageConfigStore(registryPath string) *StageConfigStore {
	return &StageConfigStore{
		globalPath: filepath.Join(registryPath, "config.json"),
	}
}

// GetGlobalDefaults returns the current global stage defaults.
// If the config file is missing or unreadable an empty GlobalConfig is returned
// so callers never need to handle a nil-map case.
func (s *StageConfigStore) GetGlobalDefaults() GlobalConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.readGlobal()
}

// SetGlobalStageDefault writes a stage assignment into the global defaults and
// persists the result atomically to ~/.paulette/config.json.
func (s *StageConfigStore) SetGlobalStageDefault(stage model.StageName, assignment StageAssignment) error {
	if err := validateStage(stage); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cfg := s.readGlobal()
	if cfg.StageDefaults == nil {
		cfg.StageDefaults = make(map[model.StageName]StageAssignment)
	}
	cfg.StageDefaults[stage] = assignment
	return s.writeGlobal(cfg)
}

// GetProjectOverrides returns the per-project stage overrides for the project
// rooted at hostDir.  Stages absent from the map (or explicitly set to nil) fall
// through to the global defaults.  An empty ProjectStageConfig is returned when
// the override file is missing or cannot be parsed.
func (s *StageConfigStore) GetProjectOverrides(hostDir string) ProjectStageConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.readProjectConfig(hostDir)
}

// SetProjectStageOverride writes a per-stage override for the project at hostDir.
// Passing a nil assignment clears the override (the stage will inherit the global
// default).  The parent directory is created if it does not yet exist.
func (s *StageConfigStore) SetProjectStageOverride(hostDir string, stage model.StageName, assignment *StageAssignment) error {
	if err := validateStage(stage); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cfg := s.readProjectConfig(hostDir)
	if cfg.Overrides == nil {
		cfg.Overrides = make(map[model.StageName]*StageAssignment)
	}

	if assignment == nil {
		// Explicit nil == inherit: keep the key present with a nil value so the
		// JSON file clearly expresses the intent, matching the schema in the arch doc.
		cfg.Overrides[stage] = nil
	} else {
		cfg.Overrides[stage] = assignment
	}

	return s.writeProjectConfig(hostDir, cfg)
}

// ResetProjectOverrides removes all per-project overrides for the project at
// hostDir by deleting the stage_config.json file.  After this call every stage
// reverts to the global default.
func (s *StageConfigStore) ResetProjectOverrides(hostDir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := projectConfigPath(hostDir)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reset project overrides: %w", err)
	}
	return nil
}

// ClearConnectionReferences removes any stage assignments (global or
// per-project-scoped) that reference the given connectionID.  Only global
// defaults are cleared here; per-project overrides must be cleaned up via the
// handler layer (or re-resolved at runtime).  Returns the names of affected
// global-default stages.
//
// The architecture doc specifies that Delete returns affected stage assignments
// so the handler can report them to the caller.  This method handles only the
// global scope; callers that need project-level cleanup can call
// GetProjectOverrides / SetProjectStageOverride themselves.
func (s *StageConfigStore) ClearConnectionReferences(connectionID string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg := s.readGlobal()
	if len(cfg.StageDefaults) == 0 {
		return nil
	}

	var affected []string
	for stage, assignment := range cfg.StageDefaults {
		if assignment.ConnectionID == connectionID {
			affected = append(affected, string(stage))
			delete(cfg.StageDefaults, stage)
		}
	}

	if len(affected) > 0 {
		// Best-effort write — log errors but don't block the delete operation.
		_ = s.writeGlobal(cfg)
	}

	return affected
}

// ---------------------------------------------------------------------------
// Internal helpers (must be called with the mutex already held).
// ---------------------------------------------------------------------------

// readGlobal reads and unmarshals the global config file.
// Returns an empty GlobalConfig on any error so callers always get a usable value.
func (s *StageConfigStore) readGlobal() GlobalConfig {
	var cfg GlobalConfig
	data, err := os.ReadFile(s.globalPath)
	if err != nil {
		// Missing file is expected on first run — return empty defaults.
		return cfg
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		// Corrupted file — return empty defaults rather than hard-failing.
		return GlobalConfig{}
	}
	if cfg.StageDefaults == nil {
		cfg.StageDefaults = make(map[model.StageName]StageAssignment)
	}
	return cfg
}

// writeGlobal atomically persists the GlobalConfig to the global config file.
func (s *StageConfigStore) writeGlobal(cfg GlobalConfig) error {
	return writeJSONAtomic(s.globalPath, cfg, 0644)
}

// readProjectConfig reads the per-project override file.
// Returns an empty ProjectStageConfig when the file is missing or invalid.
func (s *StageConfigStore) readProjectConfig(hostDir string) ProjectStageConfig {
	var cfg ProjectStageConfig
	data, err := os.ReadFile(projectConfigPath(hostDir))
	if err != nil {
		return cfg
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ProjectStageConfig{}
	}
	if cfg.Overrides == nil {
		cfg.Overrides = make(map[model.StageName]*StageAssignment)
	}
	return cfg
}

// writeProjectConfig atomically persists the ProjectStageConfig to
// <hostDir>/.paulette/stage_config.json, creating parent directories as needed.
func (s *StageConfigStore) writeProjectConfig(hostDir string, cfg ProjectStageConfig) error {
	path := projectConfigPath(hostDir)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create project config dir: %w", err)
	}
	return writeJSONAtomic(path, cfg, 0644)
}

// projectConfigPath returns the canonical path for a project's stage_config.json.
func projectConfigPath(hostDir string) string {
	return filepath.Join(hostDir, ".paulette", "stage_config.json")
}

// ---------------------------------------------------------------------------
// Shared atomic write utility.
// ---------------------------------------------------------------------------

// writeJSONAtomic marshals v to indented JSON and writes it to path using the
// write-to-temp-then-rename pattern to guarantee atomicity.  perm sets the
// file mode on creation.
func writeJSONAtomic(path string, v any, perm os.FileMode) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	// Write to a temp file in the same directory so the rename is on the same
	// filesystem and therefore atomic.
	tmp, err := os.CreateTemp(dir, ".tmp-stage-config-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	// Ensure we clean up on any error path.
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

	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename to %s: %w", path, err)
	}

	committed = true
	return nil
}

// ---------------------------------------------------------------------------
// Validation helpers.
// ---------------------------------------------------------------------------

// validStages is the canonical set of pipeline stage names.
var validStages = map[model.StageName]struct{}{
	model.StageVision:       {},
	model.StageUX:           {},
	model.StageArchitecture: {},
	model.StageBuild:        {},
	model.StageComplete:     {},
}

// validateStage returns an error when stage is not a recognised pipeline stage.
func validateStage(stage model.StageName) error {
	if _, ok := validStages[stage]; !ok {
		return fmt.Errorf("unknown pipeline stage %q; must be one of vision, ux, architecture, build, complete", stage)
	}
	return nil
}
