package provider

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/michelroberge/paulette/backend/internal/model"
)

// OperationKey is a dotted string identifying a specific sub-step within a stage,
// e.g. "ux.chat", "ux.mock", "build.generate", "build.review", "build.execute".
// Operation-level assignments override stage-level assignments in the resolution order.
type OperationKey = string

const (
	OperationUXChat       OperationKey = "ux.chat"
	OperationUXMock       OperationKey = "ux.mock"
	OperationBuildGenerate OperationKey = "build.generate"
	OperationBuildReview  OperationKey = "build.review"
	OperationBuildExecute OperationKey = "build.execute"
)

// StageAssignment describes the connection and model to use for a pipeline stage,
// along with optional per-stage inference settings that override provider defaults.
type StageAssignment struct {
	ConnectionID string   `json:"connectionId"`
	Model        string   `json:"model"`
	Temperature  *float64 `json:"temperature,omitempty"`
	NumCtx       *int     `json:"numCtx,omitempty"`
	Stream       *bool    `json:"stream,omitempty"`
}

// Fields returns the optional inference settings, safe to call on a nil receiver.
// All returned pointers are nil when the receiver is nil or the field is unset.
func (a *StageAssignment) Fields() (temperature *float64, numCtx *int, stream *bool) {
	if a == nil {
		return nil, nil, nil
	}
	return a.Temperature, a.NumCtx, a.Stream
}

// GlobalConfig is the top-level structure persisted at ~/.paulette/config.json.
// It holds the global stage defaults that every project inherits unless overridden.
type GlobalConfig struct {
	StageDefaults     map[model.StageName]StageAssignment `json:"stageDefaults"`
	// OperationDefaults maps operation keys (e.g. "ux.mock", "build.generate") to
	// connection+model assignments. These override stage-level defaults for specific
	// sub-operations. Omitted when empty for backward compatibility.
	OperationDefaults map[OperationKey]StageAssignment    `json:"operationDefaults,omitempty"`
	// BuildPool configures multi-connection parallel bead execution.
	// When non-nil, bead execution uses pool slots instead of the per-project
	// build.execute assignment. Omitted when not configured.
	BuildPool *BuildPoolConfig `json:"buildPool,omitempty"`
}

// BuildPoolConfig defines a set of connection slots for parallel bead execution.
type BuildPoolConfig struct {
	Slots []BuildPoolSlot `json:"slots"`
}

// BuildPoolSlot is a single connection entry in the build pool.
type BuildPoolSlot struct {
	ConnectionID string `json:"connectionId"`
	Model        string `json:"model"`
	MaxParallel  int    `json:"maxParallel"`
}

// ProjectStageConfig is persisted at <hostDir>/.paulette/stage_config.json.
// A nil pointer value in Overrides means the stage inherits the global default.
type ProjectStageConfig struct {
	Overrides map[model.StageName]*StageAssignment `json:"overrides"`
	// OperationOverrides maps operation keys to per-project overrides.
	// A nil pointer value means the operation inherits from the global operation
	// default or the stage-level default.
	OperationOverrides map[OperationKey]*StageAssignment `json:"operationOverrides,omitempty"`
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

// SetGlobalOperationDefault writes an operation-level assignment into the global
// config and persists it atomically. operation must be one of the OperationKey constants.
func (s *StageConfigStore) SetGlobalOperationDefault(operation OperationKey, assignment StageAssignment) error {
	if err := validateOperation(operation); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cfg := s.readGlobal()
	if cfg.OperationDefaults == nil {
		cfg.OperationDefaults = make(map[OperationKey]StageAssignment)
	}
	cfg.OperationDefaults[operation] = assignment
	return s.writeGlobal(cfg)
}

// DeleteGlobalOperationDefault removes an operation-level default from the global
// config. Subsequent resolution falls through to the stage-level default.
func (s *StageConfigStore) DeleteGlobalOperationDefault(operation OperationKey) error {
	if err := validateOperation(operation); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cfg := s.readGlobal()
	if cfg.OperationDefaults != nil {
		delete(cfg.OperationDefaults, operation)
	}
	return s.writeGlobal(cfg)
}

// SetProjectOperationOverride writes a per-project operation override for the
// project at hostDir. Passing a nil assignment clears the override.
func (s *StageConfigStore) SetProjectOperationOverride(hostDir string, operation OperationKey, assignment *StageAssignment) error {
	if err := validateOperation(operation); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cfg := s.readProjectConfig(hostDir)
	if cfg.OperationOverrides == nil {
		cfg.OperationOverrides = make(map[OperationKey]*StageAssignment)
	}
	cfg.OperationOverrides[operation] = assignment
	return s.writeProjectConfig(hostDir, cfg)
}

// DeleteProjectOperationOverride removes a per-project operation override,
// causing the operation to fall through to global/stage defaults.
func (s *StageConfigStore) DeleteProjectOperationOverride(hostDir string, operation OperationKey) error {
	if err := validateOperation(operation); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cfg := s.readProjectConfig(hostDir)
	if cfg.OperationOverrides != nil {
		delete(cfg.OperationOverrides, operation)
	}
	return s.writeProjectConfig(hostDir, cfg)
}

// ResolveOperation performs a five-level lookup for the given stage+operation
// under a single read-lock acquisition.
//
// Resolution order:
//  1. Per-project operation override (operationOverrides["build.execute"])
//  2. Global operation default       (operationDefaults["build.execute"])
//  3. Per-project stage override     (overrides["build"])
//  4. Global stage default           (stageDefaults["build"])
//  5. Return nil (caller falls back to hardcoded Claude CLI default)
func (s *StageConfigStore) ResolveOperation(hostDir string, stage model.StageName, operation OperationKey) *StageAssignment {
	s.mu.RLock()
	defer s.mu.RUnlock()

	projCfg := s.readProjectConfig(hostDir)
	globalCfg := s.readGlobal()

	// Level 1: per-project operation override.
	if hostDir != "" && projCfg.OperationOverrides != nil {
		if a, ok := projCfg.OperationOverrides[operation]; ok && a != nil && a.ConnectionID != "" {
			cp := *a
			return &cp
		}
	}

	// Level 2: global operation default.
	if globalCfg.OperationDefaults != nil {
		if a, ok := globalCfg.OperationDefaults[operation]; ok && a.ConnectionID != "" {
			cp := a
			return &cp
		}
	}

	// Level 3: per-project stage override.
	if hostDir != "" {
		if a, ok := projCfg.Overrides[stage]; ok && a != nil && a.ConnectionID != "" {
			cp := *a
			return &cp
		}
	}

	// Level 4: global stage default.
	if a, ok := globalCfg.StageDefaults[stage]; ok && a.ConnectionID != "" {
		cp := a
		return &cp
	}

	return nil
}

// ResolveStage performs a two-level lookup for the given stage under a single
// read-lock acquisition, avoiding the double-lock overhead of calling
// GetProjectOverrides then GetGlobalDefaults sequentially.
//
// Resolution order:
//  1. Per-project override at <hostDir>/.paulette/stage_config.json
//  2. Global default at ~/.paulette/config.json
//
// Returns nil when neither level has a configured assignment (caller falls back
// to the hardcoded Claude CLI default).
func (s *StageConfigStore) ResolveStage(hostDir string, stage model.StageName) *StageAssignment {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Level 1: per-project overrides.
	if hostDir != "" {
		projCfg := s.readProjectConfig(hostDir)
		if assignment, ok := projCfg.Overrides[stage]; ok && assignment != nil && assignment.ConnectionID != "" {
			// Return a copy so the caller cannot mutate the parsed config.
			cp := *assignment
			return &cp
		}
	}

	// Level 2: global defaults.
	globalCfg := s.readGlobal()
	if assignment, ok := globalCfg.StageDefaults[stage]; ok && assignment.ConnectionID != "" {
		cp := assignment
		return &cp
	}

	return nil
}

// ClearConnectionReferences removes any stage assignments that reference
// connectionID from:
//   - the global defaults (~/.paulette/config.json), and
//   - each project's per-project overrides (<hostDir>/.paulette/stage_config.json)
//     for every hostDir provided in projectHostDirs.
//
// The method returns the names of all affected stages (may contain duplicates
// across global and project scopes) and the first write error encountered.
// On a write error the in-memory read state is still consistent because the
// store does not cache state; however the affected file may not have been
// updated on disk.
//
// The connection Delete handler is responsible for collecting projectHostDirs
// from the project registry before calling this method.
func (s *StageConfigStore) ClearConnectionReferences(connectionID string, projectHostDirs []string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var affected []string

	// --- Clear global defaults ---
	globalCfg := s.readGlobal()
	var globalDirty bool
	for stage, assignment := range globalCfg.StageDefaults {
		if assignment.ConnectionID == connectionID {
			affected = append(affected, string(stage))
			delete(globalCfg.StageDefaults, stage)
			globalDirty = true
		}
	}
	for op, assignment := range globalCfg.OperationDefaults {
		if assignment.ConnectionID == connectionID {
			affected = append(affected, op)
			delete(globalCfg.OperationDefaults, op)
			globalDirty = true
		}
	}
	if globalDirty {
		if err := s.writeGlobal(globalCfg); err != nil {
			return affected, fmt.Errorf("clear global connection references: %w", err)
		}
	}

	// --- Clear per-project overrides ---
	for _, hostDir := range projectHostDirs {
		projCfg := s.readProjectConfig(hostDir)
		if len(projCfg.Overrides) == 0 && len(projCfg.OperationOverrides) == 0 {
			continue
		}

		var projDirty bool
		for stage, assignment := range projCfg.Overrides {
			if assignment != nil && assignment.ConnectionID == connectionID {
				projCfg.Overrides[stage] = nil
				affected = append(affected, string(stage))
				projDirty = true
			}
		}
		for op, assignment := range projCfg.OperationOverrides {
			if assignment != nil && assignment.ConnectionID == connectionID {
				projCfg.OperationOverrides[op] = nil
				affected = append(affected, op)
				projDirty = true
			}
		}

		if projDirty {
			if err := s.writeProjectConfig(hostDir, projCfg); err != nil {
				return affected, fmt.Errorf("clear project connection references for %s: %w", hostDir, err)
			}
		}
	}

	return affected, nil
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
		cfg.StageDefaults = make(map[model.StageName]StageAssignment)
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
		cfg.Overrides = make(map[model.StageName]*StageAssignment)
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

// GetBuildPool returns the current build pool configuration, or nil if not configured.
func (s *StageConfigStore) GetBuildPool() *BuildPoolConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg := s.readGlobal()
	return cfg.BuildPool
}

// SetBuildPool sets or replaces the build pool configuration.
func (s *StageConfigStore) SetBuildPool(pool BuildPoolConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := s.readGlobal()
	cfg.BuildPool = &pool
	return s.writeGlobal(cfg)
}

// DeleteBuildPool removes the build pool configuration.
func (s *StageConfigStore) DeleteBuildPool() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := s.readGlobal()
	cfg.BuildPool = nil
	return s.writeGlobal(cfg)
}

// projectConfigPath returns the canonical path for a project's stage_config.json.
func projectConfigPath(hostDir string) string {
	return filepath.Join(hostDir, ".paulette", "stage_config.json")
}

// ---------------------------------------------------------------------------
// Validation helpers.
// ---------------------------------------------------------------------------

// validStages is the canonical set of pipeline stage names.
var validStages = map[model.StageName]struct{}{
	model.StageVision:       {},
	model.StageUX:           {},
	model.StageUI:           {},
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

// validOperations is the canonical set of sub-step operation keys.
var validOperations = map[OperationKey]model.StageName{
	OperationUXChat:        model.StageUX,
	OperationUXMock:        model.StageUX,
	OperationBuildGenerate: model.StageBuild,
	OperationBuildReview:   model.StageBuild,
	OperationBuildExecute:  model.StageBuild,
}

// StageForOperation returns the parent stage for a known operation key.
// Returns false if the operation key is not recognised.
func StageForOperation(operation OperationKey) (model.StageName, bool) {
	s, ok := validOperations[operation]
	return s, ok
}

// validateOperation returns an error when operation is not a recognised key.
func validateOperation(operation OperationKey) error {
	if _, ok := validOperations[operation]; !ok {
		return fmt.Errorf("unknown operation key %q; must be one of ux.chat, ux.mock, build.generate, build.review, build.execute", operation)
	}
	return nil
}
