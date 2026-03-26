package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/paulette/backend/internal/agent"
	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/provider"
	"github.com/michelroberge/paulette/backend/internal/repository"
	fsrepo "github.com/michelroberge/paulette/backend/internal/repository/fs"
	"github.com/michelroberge/paulette/backend/internal/stream"
)

const mockRelPath = ".paulette/ux/mock.html"

type MockHandler struct {
	registry         repository.RegistryRepo
	artifactRepo     repository.ArtifactRepo
	activityRepo     repository.ActivityRepo
	runs             *stream.Manager
	providerRegistry *provider.Registry
	stageConfig      *provider.StageConfigStore
	connStore        *provider.ConnectionStore
}

// NewMockHandler creates a MockHandler. providerRegistry, stageConfig, and
// connStore may be nil — in that case mock generation falls back to the Claude
// CLI provider, preserving v0.1.0 behaviour exactly.
func NewMockHandler(
	registry repository.RegistryRepo,
	artifactRepo repository.ArtifactRepo,
	activityRepo repository.ActivityRepo,
	runs *stream.Manager,
	providerRegistry *provider.Registry,
	stageConfig *provider.StageConfigStore,
	connStore *provider.ConnectionStore,
) *MockHandler {
	return &MockHandler{
		registry:         registry,
		artifactRepo:     artifactRepo,
		activityRepo:     activityRepo,
		runs:             runs,
		providerRegistry: providerRegistry,
		stageConfig:      stageConfig,
		connStore:        connStore,
	}
}

// resolveProvider returns the Provider and model ID to use for the UX stage.
// It delegates to the Registry when one is available; otherwise it falls back
// to the Claude CLI provider (v0.1.0 behaviour).
func (h *MockHandler) resolveProvider(projectID, hostDir string, stage model.StageName) (provider.Provider, string, error) {
	if h.providerRegistry != nil {
		return h.providerRegistry.ResolveForStage(projectID, stage, h.stageConfig, hostDir)
	}
	return provider.NewClaudeCLIProvider(), provider.FallbackModel(stage), nil
}

// buildProviderErrorEvent builds a StreamEvent{Type: "error"} whose Content is
// a JSON-encoded connectionErrorPayload. It attempts to look up the connection
// name for the given stage; on any lookup failure it falls back to safe defaults
// so the event is always emittable. Mirrors the same helper in ChatHandler.
func (h *MockHandler) buildProviderErrorEvent(hostDir string, stage model.StageName, err error) agent.StreamEvent {
	payload := connectionErrorPayload{
		IsConnectionError: true,
		ConnectionName:    "configured connection",
		Reason:            err.Error(),
	}

	if h.stageConfig != nil && h.connStore != nil {
		if assignment := h.stageConfig.ResolveStage(hostDir, stage); assignment != nil {
			payload.ConnectionID = assignment.ConnectionID
			if conn, lookupErr := h.connStore.Get(assignment.ConnectionID); lookupErr == nil {
				payload.ConnectionName = conn.Name
			}
		}
	}

	content, _ := json.Marshal(payload)
	return agent.StreamEvent{Type: "error", Content: string(content)}
}

type mockGetResponse struct {
	HTML   string `json:"html"`
	Exists bool   `json:"exists"`
}

func (h *MockHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	p := filepath.Join(project.HostDir, mockRelPath)
	b, err := os.ReadFile(p)
	if err != nil {
		if !os.IsNotExist(err) {
			http.Error(w, "failed to read mock: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// Not in .paulette — fall back to docs/{version}/ux/mock.html
		b, err = fsrepo.ReadMockDoc(project.HostDir, project.Version)
		if err != nil {
			http.Error(w, "failed to read mock: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if b == nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockGetResponse{Exists: false})
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mockGetResponse{HTML: string(b), Exists: true})
}

type generateMockRequest struct {
	Refinement string `json:"refinement"`
}

// Generate streams mock HTML generation via SSE.
func (h *MockHandler) Generate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	// Check for existing active run — reconnect to it
	if existing := h.runs.Active(id, "ux", "mock"); existing != nil {
		existing.StreamTo(w, r, 0)
		return
	}

	var req generateMockRequest
	json.NewDecoder(r.Body).Decode(&req) // optional body — ignore decode errors

	run, err := h.StartMockRun(project, req.Refinement)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if run == nil {
		http.Error(w, "mock generation is already running", http.StatusConflict)
		return
	}

	run.StreamTo(w, r, 0)
}

// StartMockRun starts a mock generation run without HTTP plumbing.
// Returns (nil, nil) on race condition.
//
// Mock generation routes through the provider layer just like chat (ARCH-v0.2.0-007).
// The HTML-envelope retry logic (up to agent.MaxMockRetries attempts) is
// implemented here rather than inside agent.GenerateMock so that any configured
// provider — not just the Claude CLI — benefits from the same retry mechanism.
func (h *MockHandler) StartMockRun(project *model.Project, refinement string) (*stream.Run, error) {
	// Use UX artifact if available, fall back to vision artifact
	uxContent, _ := h.artifactRepo.ReadWithFallback(project.HostDir, project.Version, model.StageUX)
	if uxContent == "" {
		uxContent, _ = h.artifactRepo.ReadWithFallback(project.HostDir, project.Version, model.StageVision)
	}
	if uxContent == "" {
		return nil, fmt.Errorf("no artifact available — chat with the agent to generate content first")
	}

	frameworkCfg, _ := fsrepo.ReadFramework(project.HostDir)

	run := h.runs.Start(project.ID, "ux", "mock")
	if run == nil {
		return nil, nil // race: already started
	}
	writeActivity(h.activityRepo, project.HostDir, model.StageUX, "mock")

	// Resolve the LLM provider for the UX stage (project override → global default → Claude CLI).
	prov, modelID, provErr := h.resolveProvider(project.ID, project.HostDir, model.StageUX)
	if provErr != nil {
		run.Emit(h.buildProviderErrorEvent(project.HostDir, model.StageUX, provErr))
		clearActivity(h.activityRepo, project.HostDir, model.StageUX)
		run.Finish(h.runs)
		return run, nil
	}

	// Build the initial user prompt and the shared mock system prompt.
	var userPromptBuf strings.Builder
	userPromptBuf.WriteString("UX Design Document:\n---\n")
	userPromptBuf.WriteString(uxContent)
	userPromptBuf.WriteString("\n---\n\nGenerate the HTML wireframe mockup for all screens described above.")
	if refinement != "" {
		userPromptBuf.WriteString("\n\nRefinement instruction: ")
		userPromptBuf.WriteString(refinement)
	}
	initialUserPrompt := userPromptBuf.String()
	systemPrompt := agent.BuildMockSystemPrompt(frameworkCfg)

	go func() {
		defer run.Finish(h.runs)
		defer clearActivity(h.activityRepo, project.HostDir, model.StageUX)

		var stageTokensAccum int
		runStart := time.Now()
		defer func() {
			if stageTokensAccum > 0 {
				project.AddStageTokens(model.StageUX, stageTokensAccum)
				h.registry.Update(project)
				recordSession(project.HostDir, model.StageUX, model.SessionMock, project.Iteration, runStart, stageTokensAccum)
			}
		}()

		userPrompt := initialUserPrompt

		for attempt := 0; attempt <= agent.MaxMockRetries; attempt++ {
			if run.Context().Err() != nil {
				return
			}

			events, chatErr := prov.Chat(run.Context(), provider.ChatRequest{
				Model:        modelID,
				SystemPrompt: systemPrompt,
				UserMessage:  userPrompt,
				ProjectDir:   project.HostDir,
			})
			if chatErr != nil {
				run.Emit(h.buildProviderErrorEvent(project.HostDir, model.StageUX, chatErr))
				return
			}

			// Drain the event stream: forward chunk/token events to the run and
			// capture the full response from the "done" event for HTML validation.
			var fullResponse string
			var hadError bool
			for event := range events {
				switch event.Type {
				case "tokens":
					var n int
					fmt.Sscanf(event.Content, "%d", &n)
					stageTokensAccum += n
				case "done":
					// "done" Content is the full accumulated response text.
					fullResponse = event.Content
				case "error":
					run.Emit(event)
					hadError = true
				default:
					// Forward chunk (and any other) events so the client sees
					// streaming text in real time.
					run.Emit(event)
				}
			}

			if hadError {
				return
			}

			// Check whether the response contains the expected HTML envelope.
			if agent.HasHTMLBlock(fullResponse) {
				html := agent.ExtractHTML(fullResponse)
				if html != "" {
					p := filepath.Join(project.HostDir, mockRelPath)
					if err := os.MkdirAll(filepath.Dir(p), 0755); err == nil {
						os.WriteFile(p, []byte(html), 0644)
					}
				}
				run.Emit(agent.StreamEvent{Type: "done", Content: html})
				return
			}

			// Response did not contain the HTML envelope — retry with a
			// correction prompt (up to agent.MaxMockRetries times).
			if attempt < agent.MaxMockRetries {
				snippet := fullResponse
				if len(snippet) > 500 {
					snippet = snippet[:500] + "..."
				}
				userPrompt = fmt.Sprintf(agent.MockRetryPrompt, snippet)
				run.Emit(agent.StreamEvent{Type: "chunk", Content: "\n\n[Response was not valid HTML — retrying...]\n\n"})
			} else {
				// Exhausted retries — emit whatever we got so the client
				// can see the raw response rather than hanging indefinitely.
				html := agent.ExtractHTML(fullResponse)
				run.Emit(agent.StreamEvent{Type: "done", Content: html})
			}
		}
	}()

	return run, nil
}

// --- Framework endpoints ---

type frameworkResponse struct {
	Framework  string `json:"framework"`
	CustomName string `json:"customName,omitempty"`
}

type frameworkSetRequest struct {
	Framework  string `json:"framework"`
	CustomName string `json:"customName,omitempty"`
}

// GetFramework returns the stored framework config for a project.
func (h *MockHandler) GetFramework(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	cfg, err := fsrepo.ReadFramework(project.HostDir)
	if err != nil {
		http.Error(w, "failed to read framework: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(frameworkResponse{
		Framework:  string(cfg.Framework),
		CustomName: cfg.CustomName,
	})
}

// SetFramework stores the framework selection for a project.
func (h *MockHandler) SetFramework(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	var req frameworkSetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	validFrameworks := map[string]bool{
		"tailwind": true, "bootstrap": true, "mui": true,
		"shadcn": true, "vanilla": true, "other": true,
	}
	if !validFrameworks[req.Framework] {
		http.Error(w, "invalid framework: "+req.Framework, http.StatusBadRequest)
		return
	}
	if req.Framework == "other" && req.CustomName == "" {
		http.Error(w, "customName is required when framework is 'other'", http.StatusBadRequest)
		return
	}

	cfg := &model.FrameworkConfig{
		Framework:  model.UXFramework(req.Framework),
		CustomName: req.CustomName,
	}
	if err := fsrepo.WriteFramework(project.HostDir, cfg); err != nil {
		http.Error(w, "failed to save framework: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(frameworkResponse{
		Framework:  req.Framework,
		CustomName: req.CustomName,
	})
}
