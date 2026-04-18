package handler

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/michelroberge/paulette/backend/internal/agent"
	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/promptfiles"
	"github.com/michelroberge/paulette/backend/internal/provider"
)

// synthesizeSpecs is a reusable helper that condenses multiple artifacts into
// a single specs document via an LLM call. It caches the result on disk.
// Returns the synthesized content, or "" if synthesis was skipped (larger than originals).
// When trace is non-nil, the full LLM exchange is recorded for tuning/inspection.
func synthesizeSpecs(
	ctx context.Context,
	prov provider.Provider,
	modelID string,
	sa *provider.StageAssignment,
	store *promptfiles.PromptStore,
	promptName string, // e.g. "arch-specs-synthesize.md.tmpl"
	cachePath string, // full path for caching
	inputs map[string]string, // labeled inputs for the user message
	trace *runTrace, // optional: records the LLM exchange
) string {
	// Cache check: if file exists, reuse.
	if data, err := os.ReadFile(cachePath); err == nil && len(data) > 0 {
		specs := string(data)
		totalInput := totalInputSize(inputs)
		if len(specs) >= totalInput {
			log.Printf("synthesis: cached %s larger than originals, using originals", filepath.Base(cachePath))
			return ""
		}
		return specs
	}

	// Load the synthesis system prompt.
	sysPrompt := promptfiles.Defaults[promptName].Content
	if store != nil {
		if loaded, err := store.Load(promptName); err == nil {
			sysPrompt = loaded
		}
	}

	// Build the user message from labeled inputs.
	var userMsg strings.Builder
	for label, content := range inputs {
		userMsg.WriteString(fmt.Sprintf("## %s\n---\n%s\n---\n\n", label, content))
	}
	userMsg.WriteString("Synthesize these into a concise specs document.")

	saTemp, saNumCtx, saStream := sa.Fields()
	events, err := prov.Chat(ctx, provider.ChatRequest{
		Model:        modelID,
		SystemPrompt: sysPrompt,
		UserMessage:  userMsg.String(),
		Stage:        "synthesis",
		Temperature:  saTemp,
		NumCtx:       saNumCtx,
		Stream:       saStream,
	})
	if err != nil {
		log.Printf("synthesis: LLM call failed for %s: %v", promptName, err)
		return ""
	}

	var fullResponse strings.Builder
	runStart := time.Now()
	for ev := range events {
		if ev.Type == "chunk" {
			fullResponse.WriteString(ev.Content)
		}
	}

	// Record the LLM exchange for tuning/inspection.
	if trace != nil {
		dur := time.Since(runStart)
		trace.logStep("synthesis_"+promptName, "ok",
			fmt.Sprintf("resp=%d chars", fullResponse.Len()),
			fmt.Sprintf("## System Prompt\n%s\n\n## User Message\n%s\n\n## Raw Response\n%s",
				sysPrompt, userMsg.String(), fullResponse.String()),
			dur)
	}

	// Extract artifact from XML envelope.
	parsed := agent.ParseResponse(fullResponse.String())
	specs := parsed.Artifact
	if specs == "" {
		specs = strings.TrimSpace(fullResponse.String())
	}

	// Size gate.
	totalInput := totalInputSize(inputs)
	if len(specs) >= totalInput {
		log.Printf("synthesis: %s (%d bytes) >= originals (%d bytes), skipping", filepath.Base(cachePath), len(specs), totalInput)
		return ""
	}

	// Persist.
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err == nil {
		os.WriteFile(cachePath, []byte(specs), 0644)
	}

	log.Printf("synthesis: %s produced %d bytes (%.0f%% of originals)", filepath.Base(cachePath), len(specs), float64(len(specs))/float64(totalInput)*100)
	return specs
}

func totalInputSize(inputs map[string]string) int {
	total := 0
	for _, v := range inputs {
		total += len(v)
	}
	return total
}

// applySynthesis checks if synthesis should run for the current stage and
// replaces previousArtifacts entries with synthesized versions when smaller.
// Requires a running context and resolved provider.
// When trace is non-nil, the full LLM exchange is recorded for tuning/inspection.
func applySynthesis(
	ctx context.Context,
	stage model.StageName,
	project *model.Project,
	previousArtifacts map[model.StageName]string,
	prov provider.Provider,
	modelID string,
	sa *provider.StageAssignment,
	store *promptfiles.PromptStore,
	trace ...*runTrace,
) {
	if prov == nil || sa == nil {
		return
	}

	var t *runTrace
	if len(trace) > 0 {
		t = trace[0]
	}

	switch stage {
	case model.StageArchitecture:
		vision := previousArtifacts[model.StageVision]
		ux := previousArtifacts[model.StageUX]
		if vision == "" || ux == "" {
			return
		}
		cachePath := filepath.Join(project.DataDir, "architecture", "arch-specs.md")
		specs := synthesizeSpecs(ctx, prov, modelID, sa, store,
			"arch-specs-synthesize.md.tmpl", cachePath,
			map[string]string{"Product Vision": vision, "UX Design": ux},
			t,
		)
		if specs != "" {
			// Replace both artifacts with the single synthesis.
			previousArtifacts[model.StageVision] = specs
			previousArtifacts[model.StageUX] = "" // clear — specs contains both
		}

	case model.StageBuild:
		// Try to use arch-specs if it exists, otherwise vision+UX.
		archSpecsPath := filepath.Join(project.DataDir, "architecture", "arch-specs.md")
		archSpecs, _ := os.ReadFile(archSpecsPath)
		arch := previousArtifacts[model.StageArchitecture]
		if arch == "" {
			return
		}

		// Load mock-specs for a 360° view of what to build (screens + interactions).
		mockSpecsPath := filepath.Join(project.DataDir, "ux", "mock-specs.md")
		mockSpecs, _ := os.ReadFile(mockSpecsPath)

		var inputs map[string]string
		if len(archSpecs) > 0 {
			inputs = map[string]string{
				"Architecture Input Specs": string(archSpecs),
				"Architecture":             arch,
			}
		} else {
			vision := previousArtifacts[model.StageVision]
			ux := previousArtifacts[model.StageUX]
			inputs = map[string]string{
				"Product Vision": vision,
				"UX Design":     ux,
				"Architecture":  arch,
			}
		}
		if len(mockSpecs) > 0 {
			inputs["Mock Specs (screens & interactions)"] = string(mockSpecs)
		}

		cachePath := filepath.Join(project.DataDir, "build", "build-specs.md")
		specs := synthesizeSpecs(ctx, prov, modelID, sa, store,
			"build-specs-synthesize.md.tmpl", cachePath, inputs,
			t,
		)
		if specs != "" {
			// Replace all upstream artifacts with the single synthesis.
			previousArtifacts[model.StageVision] = ""
			previousArtifacts[model.StageUX] = ""
			previousArtifacts[model.StageArchitecture] = specs
		}
	}
}
