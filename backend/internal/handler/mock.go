package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/paulette/backend/internal/agent"
	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/provider"
	"github.com/michelroberge/paulette/backend/internal/repository"
	fsrepo "github.com/michelroberge/paulette/backend/internal/repository/fs"
	"github.com/michelroberge/paulette/backend/internal/stream"
)

// orchestratedArtifactThreshold is the UX artifact byte length above which the
// orchestrated multi-agent path is used even for non-Ollama providers, to keep
// each individual LLM call within a manageable context window.
const orchestratedArtifactThreshold = 6000

// plannerScreen is a single screen entry decoded from the planner agent's JSON output.
type plannerScreen struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

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

	// Resolve provider early so we can pick the right generation strategy.
	uxContent, _ := h.artifactRepo.ReadWithFallback(project.HostDir, project.Version, model.StageUX)
	if uxContent == "" {
		uxContent, _ = h.artifactRepo.ReadWithFallback(project.HostDir, project.Version, model.StageVision)
	}
	prov, _, _ := h.resolveProvider(project.ID, project.HostDir, model.StageUX)

	var run *stream.Run
	if prov != nil && useOrchestrated(prov, uxContent) {
		run, err = h.StartMockRunOrchestrated(project, req.Refinement)
	} else {
		run, err = h.StartMockRun(project, req.Refinement)
	}
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
			var accumulated strings.Builder
			var hadError bool
			for event := range events {
				switch event.Type {
				case "tokens":
					var n int
					fmt.Sscanf(event.Content, "%d", &n)
					stageTokensAccum += n
				case "done":
					// "done" Content is the full accumulated response text for some
					// providers (e.g. Claude CLI). Others (e.g. Ollama) emit "done"
					// with empty Content - fall back to the accumulated chunks.
					fullResponse = event.Content
					if fullResponse == "" {
						fullResponse = accumulated.String()
					}
				case "error":
					run.Emit(event)
					hadError = true
				default:
					if event.Type == "chunk" {
						accumulated.WriteString(event.Content)
					}
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
				// Exhausted retries — save and emit whatever we got so the
				// result persists even without a connected client.
				html := agent.ExtractHTML(fullResponse)
				if html != "" {
					p := filepath.Join(project.HostDir, mockRelPath)
					if err := os.MkdirAll(filepath.Dir(p), 0755); err == nil {
						os.WriteFile(p, []byte(html), 0644)
					}
				}
				run.Emit(agent.StreamEvent{Type: "done", Content: html})
			}
		}
	}()

	return run, nil
}

// useOrchestrated reports whether the orchestrated multi-agent path should be
// used for mock generation. It returns true when:
//   - the provider is Ollama (small context window, weaker instruction following), or
//   - the UX artifact exceeds orchestratedArtifactThreshold bytes (keeps any provider's
//     individual calls within a manageable context window).
func useOrchestrated(prov provider.Provider, uxContent string) bool {
	if _, ok := prov.(*provider.OllamaProvider); ok {
		return true
	}
	return len(uxContent) > orchestratedArtifactThreshold
}

// runPlannerCall sends the UX artifact to the LLM and returns the parsed screen list.
// Chunk events are forwarded to run so the user sees activity. Tokens are accumulated
// into the provided pointer.
func runPlannerCall(
	prov provider.Provider,
	modelID string,
	run *stream.Run,
	uxContent, refinement string,
	tokensAccum *int,
) ([]plannerScreen, error) {
	var userMsg strings.Builder
	userMsg.WriteString("UX Design Document:\n---\n")
	userMsg.WriteString(uxContent)
	userMsg.WriteString("\n---\n\nList every distinct screen described above.")
	if refinement != "" {
		userMsg.WriteString("\n\nContext: ")
		userMsg.WriteString(refinement)
	}

	events, err := prov.Chat(run.Context(), provider.ChatRequest{
		Model:        modelID,
		SystemPrompt: agent.BuildMockPlannerSystemPrompt(uxContent),
		UserMessage:  userMsg.String(),
	})
	if err != nil {
		return nil, err
	}

	var fullResponse strings.Builder
	for ev := range events {
		switch ev.Type {
		case "chunk":
			fullResponse.WriteString(ev.Content)
			run.Emit(agent.StreamEvent{Type: "chunk", Content: ev.Content})
		case "tokens":
			var n int
			fmt.Sscanf(ev.Content, "%d", &n)
			*tokensAccum += n
		case "error":
			return nil, fmt.Errorf("planner: %s", ev.Content)
		}
	}

	raw := fullResponse.String()

	// Debug: save the planner response to a file for inspection
	debugFile := fmt.Sprintf("debug_planner_%d.txt", time.Now().UnixNano())
	if f, err := os.Create(debugFile); err == nil {
		defer f.Close()
		f.WriteString(raw)
		fmt.Printf("Planner output written to %s\n", debugFile)
	}

	parsed := agent.ParseResponse(raw)
	if len(parsed.JSON) == 0 {
		return nil, fmt.Errorf("planner returned no JSON screen list")
	}

	var screens []plannerScreen
	if err := json.Unmarshal(parsed.JSON, &screens); err != nil {
		return nil, fmt.Errorf("planner JSON parse error: %w", err)
	}
	if len(screens) == 0 {
		return nil, fmt.Errorf("planner returned empty screen list")
	}
	return screens, nil
}

// runViewCall asks the LLM to generate an HTML fragment for a single screen.
// It applies the same HTML-envelope retry logic as StartMockRun. Chunk events
// are NOT forwarded to run here (the caller handles per-view progress messages).
func runViewCall(
	prov provider.Provider,
	modelID string,
	run *stream.Run,
	frameworkCfg *model.FrameworkConfig,
	screen plannerScreen,
	tokensAccum *int,
	tokensMu *sync.Mutex,
) (string, error) {
	systemPrompt := agent.BuildMockViewSystemPrompt(frameworkCfg)
	userMsg := "Generate the HTML fragment for this screen:\n\n" + screen.Description

	for attempt := 0; attempt <= agent.MaxMockRetries; attempt++ {
		if run.Context().Err() != nil {
			return "", run.Context().Err()
		}

		events, err := prov.Chat(run.Context(), provider.ChatRequest{
			Model:        modelID,
			SystemPrompt: systemPrompt,
			UserMessage:  userMsg,
		})
		if err != nil {
			return "", err
		}

		var fullResponse strings.Builder
		var hadError bool
		for ev := range events {
			switch ev.Type {
			case "chunk":
				fullResponse.WriteString(ev.Content)
			case "tokens":
				var n int
				fmt.Sscanf(ev.Content, "%d", &n)
				tokensMu.Lock()
				*tokensAccum += n
				tokensMu.Unlock()
			case "error":
				hadError = true
			}
		}
		if hadError {
			return "", fmt.Errorf("view %q: LLM error", screen.Title)
		}

		raw := fullResponse.String()
		if agent.HasHTMLBlock(raw) {
			html := agent.ExtractHTML(raw)
			return html, nil
		}

		if attempt < agent.MaxMockRetries {
			snippet := raw
			if len(snippet) > 500 {
				snippet = snippet[:500] + "..."
			}
			userMsg = fmt.Sprintf(agent.MockRetryPrompt, snippet)
		}
	}

	// Retries exhausted — return whatever was extracted (may be empty)
	return agent.ExtractHTML(userMsg), nil
}

// StartMockRunOrchestrated starts an orchestrated multi-agent mock generation run:
//  1. Planner call  — decomposes UX artifact into a screen list (JSON)
//  2. Parallel view calls — one per screen, each generating a small HTML fragment
//  3. Deterministic assembly — wraps fragments in a shared shell with tab navigation
//
// Returns (nil, nil) on race condition (another run already started).
func (h *MockHandler) StartMockRunOrchestrated(project *model.Project, refinement string) (*stream.Run, error) {
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

	prov, modelID, provErr := h.resolveProvider(project.ID, project.HostDir, model.StageUX)
	if provErr != nil {
		run.Emit(h.buildProviderErrorEvent(project.HostDir, model.StageUX, provErr))
		clearActivity(h.activityRepo, project.HostDir, model.StageUX)
		run.Finish(h.runs)
		return run, nil
	}

	go func() {
		defer run.Finish(h.runs)
		defer clearActivity(h.activityRepo, project.HostDir, model.StageUX)

		var tokensAccum int
		var tokensMu sync.Mutex
		runStart := time.Now()
		defer func() {
			if tokensAccum > 0 {
				project.AddStageTokens(model.StageUX, tokensAccum)
				h.registry.Update(project)
				recordSession(project.HostDir, model.StageUX, model.SessionMock, project.Iteration, runStart, tokensAccum)
			}
		}()

		// Step 1: Planner
		run.Emit(agent.StreamEvent{Type: "chunk", Content: "Planning screens from UX artifact...\n"})
		screens, err := runPlannerCall(prov, modelID, run, uxContent, refinement, &tokensAccum)
		if err != nil {
			run.Emit(agent.StreamEvent{Type: "error", Content: err.Error()})
			return
		}
		run.Emit(agent.StreamEvent{Type: "chunk", Content: fmt.Sprintf("\nFound %d screen(s). Generating HTML fragments in parallel...\n\n", len(screens))})

		// Step 2: Parallel view generation (max 3 concurrent)
		const maxViewWorkers = 3
		views := make([]agent.MockView, len(screens))
		var wg sync.WaitGroup
		sem := make(chan struct{}, maxViewWorkers)

		for i, screen := range screens {
			wg.Add(1)
			go func(idx int, sc plannerScreen) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				run.Emit(agent.StreamEvent{Type: "chunk", Content: fmt.Sprintf("[%s] Generating...\n", sc.Title)})
				html, err := runViewCall(prov, modelID, run, frameworkCfg, sc, &tokensAccum, &tokensMu)
				if err != nil {
					run.Emit(agent.StreamEvent{Type: "chunk", Content: fmt.Sprintf("[%s] Warning: %s\n", sc.Title, err.Error())})
					html = "<p style='color:#f87171'>Failed to generate this screen.</p>"
				}
				views[idx] = agent.MockView{ID: sc.ID, Title: sc.Title, HTML: html}
				run.Emit(agent.StreamEvent{Type: "chunk", Content: fmt.Sprintf("[%s] Done.\n", sc.Title)})
			}(i, screen)
		}
		wg.Wait()

		// Step 3: Deterministic assembly
		run.Emit(agent.StreamEvent{Type: "chunk", Content: "\nAssembling final HTML...\n"})
		finalHTML := agent.AssembleMockHTML(frameworkCfg, views)

		// Persist + emit done
		p := filepath.Join(project.HostDir, mockRelPath)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err == nil {
			os.WriteFile(p, []byte(finalHTML), 0644)
		}
		run.Emit(agent.StreamEvent{Type: "done", Content: finalHTML})
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
