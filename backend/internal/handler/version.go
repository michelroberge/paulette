package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/repository"
	fsrepo "github.com/michelroberge/paulette/backend/internal/repository/fs"
)

var semverRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

const (
	errProjectNotFound = "project not found"
	errInvalidVersion  = "invalid version"
)

type VersionHandler struct {
	registry repository.RegistryRepo
}

func NewVersionHandler(registry repository.RegistryRepo) *VersionHandler {
	return &VersionHandler{registry: registry}
}

// VersionListEntry is a summary of one versioned iteration returned by ListVersions.
type VersionListEntry struct {
	Version    string `json:"version"`
	Iteration  int    `json:"iteration,omitempty"`
	CommitHash string `json:"commitHash,omitempty"`
	TagName    string `json:"tagName,omitempty"`
	ApprovedAt string `json:"approvedAt,omitempty"`
	HasMeta    bool   `json:"hasMeta"`
}

// VersionStageSnapshot holds all archived data for one stage of a historical version.
type VersionStageSnapshot struct {
	Artifact    string              `json:"artifact"`
	ChatHistory *model.ChatHistory  `json:"chatHistory"`
	MockHtml    string              `json:"mockHtml,omitempty"`
	BeadsGraph  json.RawMessage     `json:"beadsGraph,omitempty"`
}

// VersionSnapshotResponse is the full payload returned by GetVersionSnapshot.
type VersionSnapshotResponse struct {
	Version string                                   `json:"version"`
	Meta    *model.VersionMeta                       `json:"meta"`
	Summary string                                   `json:"summary"`
	Stages  map[string]*VersionStageSnapshot         `json:"stages"`
}

// ListVersions returns all versioned iterations found in the project's docs/ directory.
// GET /api/projects/{id}/versions
func (h *VersionHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, errProjectNotFound, http.StatusNotFound)
		return
	}

	docsDir := filepath.Join(project.HostDir, "docs")
	entries, err := os.ReadDir(docsDir)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, []VersionListEntry{})
			return
		}
		http.Error(w, "failed to read docs directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var versions []VersionListEntry
	for _, e := range entries {
		if !e.IsDir() || !semverRe.MatchString(e.Name()) {
			continue
		}
		entry := VersionListEntry{Version: e.Name()}
		meta, _ := fsrepo.ReadVersionMeta(project.HostDir, e.Name())
		if meta != nil {
			entry.HasMeta = true
			entry.Iteration = meta.Iteration
			entry.CommitHash = meta.CommitHash
			entry.TagName = meta.TagName
			entry.ApprovedAt = meta.ApprovedAt.Format("2006-01-02T15:04:05Z")
		}
		versions = append(versions, entry)
	}

	// Sort descending by version string (semver lexicographic is sufficient for display).
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Version > versions[j].Version
	})

	writeJSON(w, versions)
}

// GetVersionSnapshot bundles all small artifacts for a historical version into one response.
// GET /api/projects/{id}/versions/{version}
func (h *VersionHandler) GetVersionSnapshot(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	version := chi.URLParam(r, "version")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, errProjectNotFound, http.StatusNotFound)
		return
	}
	if !semverRe.MatchString(version) {
		http.Error(w, errInvalidVersion, http.StatusBadRequest)
		return
	}

	resp := VersionSnapshotResponse{
		Version: version,
		Stages:  make(map[string]*VersionStageSnapshot),
	}

	resp.Meta, _ = fsrepo.ReadVersionMeta(project.HostDir, version)
	resp.Summary, _ = fsrepo.ReadSummaryDoc(project.HostDir, version)

	stages := []model.StageName{model.StageVision, model.StageUX, model.StageArchitecture, model.StageBuild}
	for _, stage := range stages {
		snap := &VersionStageSnapshot{}
		snap.Artifact, _ = fsrepo.ReadStageDoc(project.HostDir, version, stage)
		snap.ChatHistory, _ = fsrepo.ReadStageChatHistory(project.HostDir, version, stage)

		if stage == model.StageUX {
			if mockBytes, err := fsrepo.ReadMockDoc(project.HostDir, version); err == nil && len(mockBytes) > 0 {
				snap.MockHtml = string(mockBytes)
			}
		}

		if stage == model.StageBuild {
			p := filepath.Join(project.HostDir, "docs", version, "build", "beads-graph.json")
			if data, err := os.ReadFile(p); err == nil {
				snap.BeadsGraph = json.RawMessage(data)
			}
		}

		resp.Stages[string(stage)] = snap
	}

	writeJSON(w, resp)
}

// GetVersionBeadExecution returns execution log and chat for a single bead of a historical version.
// GET /api/projects/{id}/versions/{version}/beads/{beadId}
func (h *VersionHandler) GetVersionBeadExecution(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	version := chi.URLParam(r, "version")
	beadId := chi.URLParam(r, "beadId")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, errProjectNotFound, http.StatusNotFound)
		return
	}
	if !semverRe.MatchString(version) {
		http.Error(w, errInvalidVersion, http.StatusBadRequest)
		return
	}

	execution, _ := fsrepo.ReadBeadDoc(project.HostDir, version, beadId)
	msgs, _ := fsrepo.ReadBeadChatHistory(project.HostDir, version, beadId)

	writeJSON(w, map[string]any{
		"beadId":           beadId,
		"executionContent": execution,
		"chatMessages":     msgs,
	})
}

// GetVersionMock serves the UX mock HTML for a historical version.
// GET /api/projects/{id}/versions/{version}/mock
func (h *VersionHandler) GetVersionMock(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	version := chi.URLParam(r, "version")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, errProjectNotFound, http.StatusNotFound)
		return
	}
	if !semverRe.MatchString(version) {
		http.Error(w, errInvalidVersion, http.StatusBadRequest)
		return
	}

	data, err := fsrepo.ReadMockDoc(project.HostDir, version)
	if err != nil {
		http.Error(w, "failed to read mock: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if len(data) == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data) //nolint:errcheck
}

