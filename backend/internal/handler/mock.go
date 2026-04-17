package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/paulette/backend/internal/agent"
	"github.com/michelroberge/paulette/backend/internal/llmparse"
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

// plannerComponent is a single component entry decoded from the component planner's JSON output.
type plannerComponent struct {
	ID          string `json:"id"`
	Type        string `json:"type"`        // nav|sidebar|card|form|content|footer|header|table|modal|hero
	LayoutRole  string `json:"layout_role"` // top|left|main|right|bottom|full
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
	logBase          string
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
	logBase string,
) *MockHandler {
	return &MockHandler{
		registry:         registry,
		artifactRepo:     artifactRepo,
		activityRepo:     activityRepo,
		runs:             runs,
		providerRegistry: providerRegistry,
		stageConfig:      stageConfig,
		connStore:        connStore,
		logBase:          logBase,
	}
}

// resolveProvider returns the Provider and model ID to use for the UX stage.
// It delegates to the Registry when one is available; otherwise it falls back
// to the Claude CLI provider (v0.1.0 behaviour).
func (h *MockHandler) resolveProvider(projectID, hostDir string, stage model.StageName) (provider.Provider, string, *provider.StageAssignment, error) {
	if h.providerRegistry != nil {
		return h.providerRegistry.ResolveForStageWithSettings(projectID, stage, h.stageConfig, hostDir)
	}
	return provider.NewClaudeCLIProvider(), provider.FallbackModel(stage), nil, nil
}

// resolveProviderOp resolves the provider for a specific UX sub-step operation
// (e.g. "ux.chat", "ux.mock") using the five-level fallback hierarchy.
func (h *MockHandler) resolveProviderOp(projectID, hostDir string, stage model.StageName, operation provider.OperationKey) (provider.Provider, string, *provider.StageAssignment, error) {
	if h.providerRegistry != nil {
		return h.providerRegistry.ResolveForStageOperation(projectID, stage, operation, h.stageConfig, hostDir)
	}
	return provider.NewClaudeCLIProvider(), provider.FallbackModel(stage), nil, nil
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
	prov, _, _, _ := h.resolveProviderOp(project.ID, project.HostDir, model.StageUX, provider.OperationUXMock)

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
	runLogID := startRunLog(h.logBase, project, model.StageUX, "mock")

	// Resolve the LLM provider for mock generation (ux.mock → ux → Claude CLI fallback).
	prov, modelID, sa, provErr := h.resolveProviderOp(project.ID, project.HostDir, model.StageUX, provider.OperationUXMock)
	if provErr != nil {
		failRunLog(h.logBase, project.Name, runLogID, provErr.Error())
		run.Emit(h.buildProviderErrorEvent(project.HostDir, model.StageUX, provErr))
		clearActivity(h.activityRepo, project.HostDir, model.StageUX)
		run.Finish(h.runs)
		return run, nil
	}

	// Gather project context (vision, build plan) so the LLM knows what
	// the app is about, not just the UX layout.
	visionContent, _ := h.artifactRepo.ReadWithFallback(project.HostDir, project.Version, model.StageVision)
	buildContent, _ := h.artifactRepo.ReadWithFallback(project.HostDir, project.Version, model.StageBuild)

	// Build the initial user prompt and the shared mock system prompt.
	var userPromptBuf strings.Builder
	userPromptBuf.WriteString(agent.BuildMockContext(project.Name, visionContent, buildContent))
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
		var rlErr string
		runStart := time.Now()
		trace := newRunTrace(h.logBase, project.Name, runLogID)
		defer func() {
			trace.flush()
			if rlErr != "" {
				failRunLog(h.logBase, project.Name, runLogID, rlErr)
			} else {
				successRunLog(h.logBase, project.Name, runLogID, expectedArtifacts(project.HostDir, model.StageUX, "mock"), stageTokensAccum, "")
			}
		}()
		defer func() {
			if stageTokensAccum > 0 {
				project.AddStageTokens(model.StageUX, stageTokensAccum)
				h.registry.Update(project)
				recordSession(project.HostDir, model.StageUX, model.SessionMock, project.Iteration, runStart, stageTokensAccum)
			}
		}()

		userPrompt := initialUserPrompt
		saTemp, saNumCtx, saStream := sa.Fields()

		for attempt := 0; attempt <= agent.MaxMockRetries; attempt++ {
			if run.Context().Err() != nil {
				return
			}

			events, chatErr := prov.Chat(run.Context(), provider.ChatRequest{
				Model:        modelID,
				SystemPrompt: systemPrompt,
				UserMessage:  userPrompt,
				ProjectDir:   project.HostDir,
				Stage:        string(model.StageUI),
				Temperature:  saTemp,
				NumCtx:       saNumCtx,
				Stream:       saStream,
			})
			if chatErr != nil {
				rlErr = chatErr.Error()
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
					rlErr = event.Content
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

			stepName := fmt.Sprintf("mock_attempt%d", attempt)

			if hadError {
				trace.logStep(stepName, "error", "LLM error event", fullResponse, 0)
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
				trace.logStep(stepName, "ok",
					fmt.Sprintf("HTML extracted (%d bytes)", len(html)),
					fullResponse, 0)
				run.Emit(agent.StreamEvent{Type: "done", Content: html})
				return
			}

			trace.logStep(stepName, "error",
				fmt.Sprintf("no HTML envelope found; response length: %d bytes", len(fullResponse)),
				fullResponse, 0)

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
	sa *provider.StageAssignment,
	run *stream.Run,
	uxContent, refinement string,
	tokensAccum *int,
	trace *runTrace,
) ([]plannerScreen, error) {
	var userMsg strings.Builder
	userMsg.WriteString("UX Design Document:\n---\n")
	userMsg.WriteString(uxContent)
	userMsg.WriteString("\n---\n\nList every distinct screen described above.")
	if refinement != "" {
		userMsg.WriteString("\n\nContext: ")
		userMsg.WriteString(refinement)
	}

	stepStart := time.Now()
	saTemp, saNumCtx, saStream := sa.Fields()
	events, err := prov.Chat(run.Context(), provider.ChatRequest{
		Model:        modelID,
		SystemPrompt: agent.BuildMockPlannerSystemPrompt(),
		UserMessage:  userMsg.String(),
		Stage:        string(model.StageUI),
		Temperature:  saTemp,
		NumCtx:       saNumCtx,
		Stream:       saStream,
	})
	if err != nil {
		return nil, err
	}

	var fullResponse strings.Builder
	for ev := range events {
		switch ev.Type {
		case "chunk":
			fullResponse.WriteString(ev.Content)
		case "tokens":
			var n int
			fmt.Sscanf(ev.Content, "%d", &n)
			*tokensAccum += n
		case "error":
			return nil, fmt.Errorf("planner: %s", ev.Content)
		}
	}

	raw := agent.StripThinkBlocks(fullResponse.String())
	debugLogAgentOutput("screen_planner", raw)

	// Primary path: extract JSON (handles <jsonplan> tags and raw JSON arrays).
	parsed := agent.ParseResponse(raw)
	if len(parsed.JSON) > 0 {
		var screens []plannerScreen
		if err := json.Unmarshal(parsed.JSON, &screens); err == nil && len(screens) > 0 {
			trace.logStep("screen_planner", "ok",
				fmt.Sprintf("parsed %d screen(s) via JSON", len(screens)),
				raw, time.Since(stepStart))
			return screens, nil
		}
	}

	// Fallback: use llmparse to handle list-format responses from Ollama.
	engine := llmparse.NewEngine(
		[]llmparse.Strategy{llmparse.ListStrategy{}},
		nil,
	)
	var titles []string
	if err := engine.Parse(raw, &titles); err == nil && len(titles) > 0 {
		screens := make([]plannerScreen, len(titles))
		for i, title := range titles {
			screens[i] = plannerScreen{
				ID:          slugifyTitle(title),
				Title:       title,
				Description: title,
			}
		}
		trace.logStep("screen_planner", "ok",
			fmt.Sprintf("parsed %d screen(s) via list fallback", len(screens)),
			raw, time.Since(stepStart))
		return screens, nil
	}

	// Both strategies failed — log the raw response for debugging.
	var diag strings.Builder
	diag.WriteString("JSON extraction: ")
	if len(parsed.JSON) == 0 {
		diag.WriteString("no JSON found")
	} else {
		var tmp []plannerScreen
		if err := json.Unmarshal(parsed.JSON, &tmp); err != nil {
			diag.WriteString("unmarshal error: " + err.Error())
		} else {
			diag.WriteString("empty array")
		}
	}
	diag.WriteString("; list extraction: no items found")
	diag.WriteString(fmt.Sprintf("; response length: %d bytes", len(raw)))
	detail := diag.String()

	trace.logStep("screen_planner", "error", detail, raw, time.Since(stepStart))

	return nil, fmt.Errorf("planner returned no parseable screen list (%s)", detail)
}

// debugLogAgentOutput prints the raw LLM response for an agent step to stdout and,
// when PAULETTE_DEBUG_DIR is set, writes it to <dir>/<step>_<timestamp>.txt.
// The step name (e.g. "screen_planner", "component_planner", "component_gen") is
// included in both the console header and the filename so outputs can be correlated
// and reused in unit tests.
func debugLogAgentOutput(step, raw string) {
	fmt.Printf("=== %s RAW OUTPUT START ===\n%s\n=== %s RAW OUTPUT END ===\n", step, raw, step)
	if debugDir := os.Getenv("PAULETTE_DEBUG_DIR"); debugDir != "" {
		path := fmt.Sprintf("%s/%s_%d.txt", debugDir, step, time.Now().UnixNano())
		if f, err := os.Create(path); err == nil {
			f.WriteString(raw)
			_ = f.Close()
			fmt.Printf("%s output written to %s\n", step, path)
		}
	}
}

var slugifyRe = regexp.MustCompile(`[^a-z0-9]+`)

// slugifyTitle converts a screen title into a snake_case id.
func slugifyTitle(s string) string {
	s = strings.ToLower(s)
	s = slugifyRe.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if s == "" {
		return "screen"
	}
	return s
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
	var lastRaw string

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

		lastRaw = fullResponse.String()
		if agent.HasHTMLBlock(lastRaw) {
			html := agent.ExtractHTML(lastRaw)
			return html, nil
		}

		if attempt < agent.MaxMockRetries {
			snippet := lastRaw
			if len(snippet) > 500 {
				snippet = snippet[:500] + "..."
			}
			userMsg = fmt.Sprintf(agent.MockRetryPrompt, snippet)
		}
	}

	// Retries exhausted — return whatever can be extracted from the last response.
	return agent.ExtractHTML(lastRaw), nil
}

// runComponentPlannerCall asks the LLM to decompose a single screen into sub-components.
// It is silent — no chunk events are forwarded to run. Tokens are accumulated into the
// provided pointer via tokensMu.
func runComponentPlannerCall(
	prov provider.Provider,
	modelID string,
	sa *provider.StageAssignment,
	run *stream.Run,
	frameworkCfg *model.FrameworkConfig,
	screen plannerScreen,
	tokensAccum *int,
	tokensMu *sync.Mutex,
	trace *runTrace,
) ([]plannerComponent, error) {
	userMsg := "Screen: " + screen.Title + "\n\n" + screen.Description

	stepStart := time.Now()
	saTemp, saNumCtx, saStream := sa.Fields()
	events, err := prov.Chat(run.Context(), provider.ChatRequest{
		Model:        modelID,
		SystemPrompt: agent.BuildMockComponentPlannerSystemPrompt(frameworkCfg),
		UserMessage:  userMsg,
		Stage:        string(model.StageUI),
		Temperature:  saTemp,
		NumCtx:       saNumCtx,
		Stream:       saStream,
	})
	if err != nil {
		return nil, err
	}

	var fullResponse strings.Builder
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
			return nil, fmt.Errorf("component planner: %s", ev.Content)
		}
	}

	raw := agent.StripThinkBlocks(fullResponse.String())
	stepName := "component_planner_" + screen.ID
	debugLogAgentOutput(stepName, raw)

	// Primary: extract <jsonplan> JSON and unmarshal into []plannerComponent.
	parsed := agent.ParseResponse(raw)
	if len(parsed.JSON) > 0 {
		var components []plannerComponent
		if err := json.Unmarshal(parsed.JSON, &components); err == nil && len(components) > 0 {
			trace.logStep(stepName, "ok",
				fmt.Sprintf("parsed %d component(s) via JSON", len(components)),
				raw, time.Since(stepStart))
			return components, nil
		}
	}

	// Fallback: use llmparse ListStrategy to handle plain list responses.
	engine := llmparse.NewEngine([]llmparse.Strategy{llmparse.ListStrategy{}}, nil)
	var titles []string
	if err := engine.Parse(raw, &titles); err == nil && len(titles) > 0 {
		components := make([]plannerComponent, len(titles))
		for i, title := range titles {
			components[i] = plannerComponent{
				ID:          slugifyTitle(title),
				Type:        "content",
				LayoutRole:  "full",
				Description: title,
			}
		}
		trace.logStep(stepName, "ok",
			fmt.Sprintf("parsed %d component(s) via list fallback", len(components)),
			raw, time.Since(stepStart))
		return components, nil
	}

	detail := fmt.Sprintf("JSON: %d bytes extracted, list: no items; response length: %d bytes", len(parsed.JSON), len(raw))
	trace.logStep(stepName, "error", detail, raw, time.Since(stepStart))
	return nil, fmt.Errorf("component planner returned no parseable component list (%s)", detail)
}

// runComponentCall asks the LLM to generate an HTML fragment for a single component.
// It is silent — no chunk events are forwarded to run. HTML is extracted via llmparse
// HTMLStrategy, which handles the XML envelope, code fences, and raw HTML fallback.
func runComponentCall(
	prov provider.Provider,
	modelID string,
	sa *provider.StageAssignment,
	run *stream.Run,
	frameworkCfg *model.FrameworkConfig,
	comp plannerComponent,
	tokensAccum *int,
	tokensMu *sync.Mutex,
	trace *runTrace,
) (string, error) {
	systemPrompt := agent.BuildMockComponentSystemPrompt(frameworkCfg)
	userMsg := fmt.Sprintf("Component type: %s\nLayout role: %s\n\n%s",
		comp.Type, comp.LayoutRole, comp.Description)

	htmlEngine := llmparse.NewEngine([]llmparse.Strategy{llmparse.HTMLStrategy{}}, nil)
	saTemp, saNumCtx, saStream := sa.Fields()

	for attempt := 0; attempt <= agent.MaxMockRetries; attempt++ {
		stepStart := time.Now()
		if run.Context().Err() != nil {
			return "", run.Context().Err()
		}

		events, err := prov.Chat(run.Context(), provider.ChatRequest{
			Model:        modelID,
			SystemPrompt: systemPrompt,
			UserMessage:  userMsg,
			Stage:        string(model.StageUI),
			Temperature:  saTemp,
			NumCtx:       saNumCtx,
			Stream:       saStream,
		})
		if err != nil {
			return "", err
		}

		var fullResponse strings.Builder
		var hadError bool
		var errContent string
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
				errContent = ev.Content
			}
		}
		if hadError {
			return "", fmt.Errorf("component %q: LLM error: %s", comp.ID, errContent)
		}

		original := fullResponse.String()
		raw := agent.StripThinkBlocks(original)
		stepName := fmt.Sprintf("component_gen_%s_attempt%d", comp.ID, attempt)
		debugLogAgentOutput(stepName, raw)
		var html string
		if err := htmlEngine.Parse(raw, &html); err == nil && html != "" {
			trace.logStep(stepName, "ok",
				fmt.Sprintf("extracted HTML (%d bytes)", len(html)),
				raw, time.Since(stepStart))
			return html, nil
		}

		// Fallback: if think-block stripping removed useful content,
		// try to extract HTML from the original (pre-stripped) response.
		// Reasoning models sometimes put the HTML inside <think> blocks
		// or leave them unclosed, causing StripThinkBlocks to drop everything.
		if len(raw) < len(original) {
			if err := htmlEngine.Parse(original, &html); err == nil && html != "" {
				trace.logStep(stepName, "ok",
					fmt.Sprintf("extracted HTML from pre-stripped response (%d bytes)", len(html)),
					original, time.Since(stepStart))
				return html, nil
			}
		}

		trace.logStep(stepName, "error",
			fmt.Sprintf("no HTML extracted; raw length: %d bytes, original length: %d bytes", len(raw), len(original)),
			original, time.Since(stepStart))

		if attempt < agent.MaxMockRetries {
			snippet := original
			if snippet == "" {
				snippet = raw
			}
			if len(snippet) > 500 {
				snippet = snippet[:500] + "..."
			}
			userMsg = fmt.Sprintf(agent.MockRetryPrompt, snippet)
		}
	}

	return "", fmt.Errorf("component %q: retries exhausted, no HTML extracted", comp.ID)
}

// sanitizeMockClasses performs a deterministic pass over assembled HTML, stripping
// CSS class names that belong to the wrong framework. This replaces the LLM-based
// styler which cannot faithfully reproduce large documents with small models.
func sanitizeMockClasses(html string, cfg *model.FrameworkConfig) string {
	if cfg == nil {
		return html
	}
	filter := classForbiddenFilter(cfg.Framework)
	if filter == nil {
		return html
	}
	return classAttrRe.ReplaceAllStringFunc(html, func(match string) string {
		// Extract the class value between quotes.
		inner := match[len(`class="`) : len(match)-1]
		classes := strings.Fields(inner)
		kept := classes[:0]
		for _, cls := range classes {
			if !filter(cls) {
				kept = append(kept, cls)
			}
		}
		if len(kept) == 0 {
			return `class=""`
		}
		return `class="` + strings.Join(kept, " ") + `"`
	})
}

// classAttrRe matches class="..." attributes (non-greedy, double quotes only).
var classAttrRe = regexp.MustCompile(`class="[^"]*"`)

// Bootstrap class patterns that should be stripped when targeting Tailwind.
var bootstrapClassRe = regexp.MustCompile(
	`^(btn|btn-.+|card|card-.+|container|container-.+|row|col-.+|d-.+|navbar|nav-.+|form-.+|badge|list-group.*|table-.+|alert.*|modal.*|dropdown.*)$`,
)

// Tailwind utility patterns that should be stripped when targeting Bootstrap.
var tailwindClassRe = regexp.MustCompile(
	`^(bg-.+|text-(slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose|white|black|transparent).*|rounded-.+|shadow-.+|border-.+|hover:.+|focus:.+|w-.+|h-.+|min-.+|max-.+|gap-.+|items-.+|justify-.+|space-.+|overflow-.+|opacity-.+|font-.+|leading-.+|tracking-.+|flex-.+|grid-.+|grow|shrink|basis-.+|self-.+|place-.+|inset-.+|top-.+|right-.+|left-.+|bottom-.+|z-.+|aspect-.+|transition-.+|duration-.+|ease-.+|animate-.+)$`,
)

// classForbiddenFilter returns a function that returns true for class names
// that should be stripped for the given target framework. Returns nil if no
// sanitization is needed.
func classForbiddenFilter(fw model.UXFramework) func(string) bool {
	// Protected: never strip mock-* infrastructure classes.
	protected := func(cls string) bool {
		return strings.HasPrefix(cls, "mock-")
	}

	switch fw {
	case model.FrameworkTailwind:
		// Strip Bootstrap class names; keep Tailwind utilities.
		return func(cls string) bool {
			if protected(cls) {
				return false
			}
			return bootstrapClassRe.MatchString(cls)
		}
	case model.FrameworkBootstrap:
		// Strip Tailwind utility names; keep Bootstrap components.
		return func(cls string) bool {
			if protected(cls) {
				return false
			}
			return tailwindClassRe.MatchString(cls)
		}
	case model.FrameworkMUI, model.FrameworkShadcn, model.FrameworkVanilla:
		// These frameworks use inline styles / CSS variables only.
		// Strip any Tailwind or Bootstrap class names.
		return func(cls string) bool {
			if protected(cls) {
				return false
			}
			return bootstrapClassRe.MatchString(cls) || tailwindClassRe.MatchString(cls)
		}
	default:
		return nil
	}
}

// assembleScreenFragment combines generated component HTML fragments into a single
// screen fragment using deterministic layout rules based on each component's layout_role.
// Inline styles are used so the result is framework-agnostic.
func assembleScreenFragment(components []plannerComponent, htmlFragments map[string]string) string {
	if len(components) == 0 {
		return `<div style="max-width:1024px;margin:0 auto;padding:24px"><p>No components generated.</p></div>`
	}

	var tops, lefts, mains, rights, bottoms, fulls []plannerComponent
	for _, c := range components {
		switch c.LayoutRole {
		case "top":
			tops = append(tops, c)
		case "left":
			lefts = append(lefts, c)
		case "main":
			mains = append(mains, c)
		case "right":
			rights = append(rights, c)
		case "bottom":
			bottoms = append(bottoms, c)
		default: // "full" and unknown
			fulls = append(fulls, c)
		}
	}

	var b strings.Builder
	b.WriteString(`<div style="max-width:1024px;margin:0 auto;padding:24px">`)

	// Top components — full width, stacked
	for _, c := range tops {
		b.WriteString(`<div style="width:100%">`)
		b.WriteString(htmlFragments[c.ID])
		b.WriteString("</div>\n")
	}

	// Middle row — flex if sidebars present, otherwise full-width
	hasSidebar := len(lefts) > 0 || len(rights) > 0
	hasMain := len(mains) > 0
	if hasSidebar && hasMain {
		b.WriteString(`<div style="display:flex;gap:0;align-items:stretch">`)
		for _, c := range lefts {
			b.WriteString(`<div style="flex:0 0 220px;min-width:180px">`)
			b.WriteString(htmlFragments[c.ID])
			b.WriteString("</div>\n")
		}
		// First main component in the flex row
		b.WriteString(`<div style="flex:1;min-width:0">`)
		b.WriteString(htmlFragments[mains[0].ID])
		b.WriteString("</div>\n")
		for _, c := range rights {
			b.WriteString(`<div style="flex:0 0 220px;min-width:180px">`)
			b.WriteString(htmlFragments[c.ID])
			b.WriteString("</div>\n")
		}
		b.WriteString("</div>\n")
		// Extra main components (beyond the first) rendered as full-width after the row
		for _, c := range mains[1:] {
			b.WriteString(`<div style="width:100%">`)
			b.WriteString(htmlFragments[c.ID])
			b.WriteString("</div>\n")
		}
	} else {
		// No sidebars — render mains and fulls stacked
		for _, c := range mains {
			b.WriteString(`<div style="width:100%">`)
			b.WriteString(htmlFragments[c.ID])
			b.WriteString("</div>\n")
		}
		// Orphaned left/right (no matching main) also rendered stacked
		for _, c := range append(lefts, rights...) {
			b.WriteString(`<div style="width:100%">`)
			b.WriteString(htmlFragments[c.ID])
			b.WriteString("</div>\n")
		}
	}

	// Full-width components (explicit "full" + unknowns)
	for _, c := range fulls {
		b.WriteString(`<div style="width:100%">`)
		b.WriteString(htmlFragments[c.ID])
		b.WriteString("</div>\n")
	}

	// Bottom components — full width, stacked
	for _, c := range bottoms {
		b.WriteString(`<div style="width:100%">`)
		b.WriteString(htmlFragments[c.ID])
		b.WriteString("</div>\n")
	}

	b.WriteString("</div>")
	return b.String()
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

	// Gather project context for the planner call.
	visionContent, _ := h.artifactRepo.ReadWithFallback(project.HostDir, project.Version, model.StageVision)
	buildContent, _ := h.artifactRepo.ReadWithFallback(project.HostDir, project.Version, model.StageBuild)
	projectContext := agent.BuildMockContext(project.Name, visionContent, buildContent)

	frameworkCfg, _ := fsrepo.ReadFramework(project.HostDir)

	run := h.runs.Start(project.ID, "ux", "mock")
	if run == nil {
		return nil, nil // race: already started
	}
	writeActivity(h.activityRepo, project.HostDir, model.StageUX, "mock")
	runLogID := startRunLog(h.logBase, project, model.StageUX, "mock")

	prov, modelID, sa, provErr := h.resolveProviderOp(project.ID, project.HostDir, model.StageUX, provider.OperationUXMock)
	if provErr != nil {
		failRunLog(h.logBase, project.Name, runLogID, provErr.Error())
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
		var rlErr string
		runStart := time.Now()
		trace := newRunTrace(h.logBase, project.Name, runLogID)
		defer func() {
			trace.flush()
			if rlErr != "" {
				failRunLog(h.logBase, project.Name, runLogID, rlErr)
			} else {
				successRunLog(h.logBase, project.Name, runLogID, expectedArtifacts(project.HostDir, model.StageUX, "mock"), tokensAccum, "")
			}
		}()
		defer func() {
			if tokensAccum > 0 {
				project.AddStageTokens(model.StageUX, tokensAccum)
				h.registry.Update(project)
				recordSession(project.HostDir, model.StageUX, model.SessionMock, project.Iteration, runStart, tokensAccum)
			}
		}()

		// Step 1: Planner
		run.Emit(agent.StreamEvent{Type: "chunk", Content: "Planning screens from UX artifact...\n"})
		screens, err := runPlannerCall(prov, modelID, sa, run, projectContext+uxContent, refinement, &tokensAccum, trace)
		if err != nil {
			rlErr = err.Error()
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

				// 2a. Component planner
				run.Emit(agent.StreamEvent{Type: "chunk", Content: fmt.Sprintf("[%s] Launching component planner...\n", sc.Title)})
				components, compPlanErr := runComponentPlannerCall(prov, modelID, sa, run, frameworkCfg, sc, &tokensAccum, &tokensMu, trace)
				if compPlanErr != nil {
					run.Emit(agent.StreamEvent{Type: "chunk", Content: fmt.Sprintf("[%s] Component planner failed — using fallback.\n", sc.Title)})
					components = []plannerComponent{{
						ID:          sc.ID + "_fallback",
						Type:        "content",
						LayoutRole:  "full",
						Description: sc.Description,
					}}
				}

				// 2b. Component generators (sequential)
				htmlFragments := make(map[string]string, len(components))
				for _, comp := range components {
					run.Emit(agent.StreamEvent{Type: "chunk", Content: fmt.Sprintf("[%s > %s] Launching component agent (%s, %s)...\n", sc.Title, comp.ID, comp.Type, comp.LayoutRole)})
					compHTML, compErr := runComponentCall(prov, modelID, sa, run, frameworkCfg, comp, &tokensAccum, &tokensMu, trace)
					if compErr != nil {
						run.Emit(agent.StreamEvent{Type: "chunk", Content: fmt.Sprintf("[%s > %s] Warning: %s\n", sc.Title, comp.ID, compErr.Error())})
						compHTML = fmt.Sprintf(`<div class="mock-component"><p style="color:#f87171">Component failed: %s</p></div>`, comp.ID)
					}
					htmlFragments[comp.ID] = compHTML
					run.Emit(agent.StreamEvent{Type: "chunk", Content: fmt.Sprintf("[%s > %s] Done.\n", sc.Title, comp.ID)})
				}

				// 2c. Deterministic screen assembly
				screenHTML := assembleScreenFragment(components, htmlFragments)
				views[idx] = agent.MockView{ID: sc.ID, Title: sc.Title, HTML: screenHTML}
				run.Emit(agent.StreamEvent{Type: "chunk", Content: fmt.Sprintf("[%s] Done (%d component(s)).\n", sc.Title, len(components))})
			}(i, screen)
		}
		wg.Wait()

		// Step 3: Deterministic assembly
		run.Emit(agent.StreamEvent{Type: "chunk", Content: "\nAssembling final HTML...\n"})
		finalHTML := agent.AssembleMockHTML(frameworkCfg, views)

		// Step 4: Deterministic CSS class sanitizer — strip wrong-framework classes.
		finalHTML = sanitizeMockClasses(finalHTML, frameworkCfg)

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
