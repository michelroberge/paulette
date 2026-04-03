package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/michelroberge/paulette/backend/internal/agent"
	"github.com/michelroberge/paulette/backend/internal/llmparse"
	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/pipeline"
	"github.com/michelroberge/paulette/backend/internal/provider"
)

// codeFilePlan is a single file entry returned by the code file planner.
type codeFilePlan struct {
	Path    string `json:"path"`    // relative path from project root
	Purpose string `json:"purpose"` // what this file does for this task
	Outline string `json:"outline"` // key types/functions/structures to include
	IsNew   bool   `json:"isNew"`   // true = create new, false = modify existing
}

// dispatchExecuteBeadOrchestrated implements a multi-level non-agentic code generation
// path designed for Ollama and other small models that struggle with multi-step tool use.
//
// Level 1 – File Planner: given the bead task, produce a list of files to create/modify.
// Level 2 – Per-File Generator: for each file, generate its complete content.
// Level 3 – Deterministic writer: write generated content to disk.
//
// Returns a channel of StreamEvents matching the contract of dispatchExecuteBead so
// the caller (StartExecuteRun) can consume it identically.
func dispatchExecuteBeadOrchestrated(
	ctx context.Context,
	prov provider.Provider,
	modelID string,
	projectDir string,
	bead model.Bead,
	artifacts map[model.StageName]string,
	enhCtx *agent.EnhancementContext,
) (<-chan agent.StreamEvent, error) {
	ch := make(chan agent.StreamEvent, 32)
	go func() {
		defer close(ch)

		emit := func(ev agent.StreamEvent) {
			select {
			case ch <- ev:
			case <-ctx.Done():
			}
		}
		emitLog := func(msg string) { emit(agent.StreamEvent{Type: "log", Content: msg}) }

		// --- Level 1: File Planner ---
		emitLog(fmt.Sprintf("[%s] Planning files...", bead.ID))
		files, planTokens, err := runCodeFilePlannerCall(ctx, prov, modelID, bead, artifacts)
		if err != nil {
			emit(agent.StreamEvent{Type: "error", Content: fmt.Sprintf("[%s] file planner: %v", bead.ID, err)})
			return
		}
		if planTokens > 0 {
			emit(agent.StreamEvent{Type: "tokens", Tokens: planTokens})
		}
		if len(files) == 0 {
			emit(agent.StreamEvent{Type: "error", Content: fmt.Sprintf("[%s] file planner returned no files", bead.ID)})
			return
		}
		emitLog(fmt.Sprintf("[%s] %d file(s) planned", bead.ID, len(files)))

		// --- Level 2: Per-File Generator ---
		generated := make(map[string]string, len(files)) // path → content
		var totalGenTokens int
		for i, fp := range files {
			if ctx.Err() != nil {
				return
			}
			emitLog(fmt.Sprintf("[%s] [%d/%d] Generating %s...", bead.ID, i+1, len(files), fp.Path))

			// Read existing content when modifying a file.
			var existing string
			if !fp.IsNew {
				absPath := filepath.Join(projectDir, fp.Path)
				if data, readErr := os.ReadFile(absPath); readErr == nil {
					existing = string(data)
				}
			}

			content, genTokens, genErr := runCodeFileGeneratorCall(ctx, prov, modelID, bead, fp, existing, generated)
			if genErr != nil {
				emitLog(fmt.Sprintf("[%s] warn: generator failed for %s: %v — skipping", bead.ID, fp.Path, genErr))
				continue
			}
			if genTokens > 0 {
				totalGenTokens += genTokens
				emit(agent.StreamEvent{Type: "tokens", Tokens: genTokens})
			}
			if content == "" {
				emitLog(fmt.Sprintf("[%s] warn: empty content for %s — skipping", bead.ID, fp.Path))
				continue
			}
			generated[fp.Path] = content
		}

		// --- Level 3: Write files to disk ---
		var written []string
		for _, fp := range files {
			content, ok := generated[fp.Path]
			if !ok {
				continue
			}
			absPath := filepath.Join(projectDir, fp.Path)
			if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
				emitLog(fmt.Sprintf("[%s] warn: mkdir failed for %s: %v", bead.ID, fp.Path, err))
				continue
			}
			if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
				emitLog(fmt.Sprintf("[%s] warn: write failed for %s: %v", bead.ID, fp.Path, err))
				continue
			}
			written = append(written, fp.Path)
		}

		// --- Level 4: Post-generation build validation ---
		// Run build commands from the architecture artifact to catch compile errors
		// before the review step. On failure, attempt a targeted correction.
		if arch, ok := artifacts[model.StageArchitecture]; ok && arch != "" {
			_, buildCmds, _ := pipeline.ParseValidationCommands(arch)
			if len(buildCmds) > 0 {
				emitLog(fmt.Sprintf("[%s] Running build validation (%d commands)...", bead.ID, len(buildCmds)))
				const maxBuildFixes = 2
				for attempt := 0; attempt < maxBuildFixes; attempt++ {
					if ctx.Err() != nil {
						break
					}
					allPassed := true
					var failOutput strings.Builder
					for _, cmdStr := range buildCmds {
						result := pipeline.RunBuildCommand(ctx, projectDir, cmdStr)
						if !result.Success {
							allPassed = false
							failOutput.WriteString(fmt.Sprintf("Command `%s` failed (exit %d):\n%s\n\n", result.Command, result.ExitCode, result.Output))
						}
					}
					if allPassed {
						emitLog(fmt.Sprintf("[%s] Build validation passed", bead.ID))
						break
					}
					if attempt == maxBuildFixes-1 {
						emitLog(fmt.Sprintf("[%s] Build validation still failing after %d fix attempts — proceeding to review", bead.ID, maxBuildFixes))
						break
					}
					emitLog(fmt.Sprintf("[%s] Build failed, attempting correction (attempt %d/%d)...", bead.ID, attempt+1, maxBuildFixes))

					// Create a correction bead with build errors appended to the description,
					// then re-generate each file with the current content as "existing".
					corrBead := bead
					corrBead.Description = bead.Description + "\n\nBUILD ERRORS — fix these:\n" + failOutput.String()
					for _, fp := range files {
						content, ok := generated[fp.Path]
						if !ok {
							continue
						}
						fixed, fixTokens, fixErr := runCodeFileGeneratorCall(ctx, prov, modelID, corrBead, fp, content, generated)
						if fixTokens > 0 {
							emit(agent.StreamEvent{Type: "tokens", Tokens: fixTokens})
						}
						if fixErr != nil || fixed == "" {
							continue
						}
						generated[fp.Path] = fixed
						absPath := filepath.Join(projectDir, fp.Path)
						os.WriteFile(absPath, []byte(fixed), 0o644) //nolint:errcheck
					}
				}
			}
		}

		summary := fmt.Sprintf("Orchestrated code generation complete for [%s] '%s'.\n\nFiles written (%d):\n%s",
			bead.ID, bead.Title, len(written), strings.Join(written, "\n"))
		emit(agent.StreamEvent{Type: "done", Content: summary})
	}()
	return ch, nil
}

// runCodeFilePlannerCall asks the LLM to plan which files to create/modify for a bead.
// Returns the file plan, token count, and any error.
func runCodeFilePlannerCall(
	ctx context.Context,
	prov provider.Provider,
	modelID string,
	bead model.Bead,
	artifacts map[model.StageName]string,
) ([]codeFilePlan, int, error) {
	var userMsg strings.Builder
	userMsg.WriteString(fmt.Sprintf("Task: %s\n\n", bead.Title))
	if bead.Description != "" {
		userMsg.WriteString(fmt.Sprintf("Description:\n%s\n\n", bead.Description))
	}
	if len(bead.TargetFiles) > 0 {
		userMsg.WriteString("Target files (hint):\n")
		for _, f := range bead.TargetFiles {
			userMsg.WriteString(fmt.Sprintf("- %s\n", f))
		}
		userMsg.WriteString("\n")
	}
	// Inject architecture context to help the planner infer file structure.
	if arch, ok := artifacts[model.StageArchitecture]; ok && arch != "" {
		userMsg.WriteString("Architecture:\n---\n")
		userMsg.WriteString(arch)
		userMsg.WriteString("\n---\n\n")
	}
	// Inject UX context so the planner considers UI requirements when
	// deciding which files to create (e.g. components, pages, styles).
	if ux, ok := artifacts[model.StageUX]; ok && ux != "" {
		const maxUXChars = 2000
		truncated := ux
		if len(truncated) > maxUXChars {
			truncated = truncated[:maxUXChars] + "\n...(truncated)"
		}
		userMsg.WriteString("UX Design (for UI-related files):\n---\n")
		userMsg.WriteString(truncated)
		userMsg.WriteString("\n---\n\n")
	}
	userMsg.WriteString("List all files to create or modify for this task.")

	events, err := prov.Chat(ctx, provider.ChatRequest{
		Model:        modelID,
		SystemPrompt: agent.BuildCodeFilePlannerSystemPrompt(),
		UserMessage:  userMsg.String(),
	})
	if err != nil {
		return nil, 0, err
	}

	var fullResponse strings.Builder
	var tokens int
	for ev := range events {
		switch ev.Type {
		case "chunk":
			fullResponse.WriteString(ev.Content)
		case "tokens":
			tokens += ev.Tokens
		case "error":
			return nil, tokens, fmt.Errorf("file planner: %s", ev.Content)
		}
	}

	raw := fullResponse.String()
	debugLogAgentOutput("code_file_planner", raw)

	// Primary: extract <jsonplan> JSON array.
	parsed := agent.ParseResponse(raw)
	if len(parsed.JSON) > 0 {
		var files []codeFilePlan
		if err := json.Unmarshal(parsed.JSON, &files); err == nil && len(files) > 0 {
			return files, tokens, nil
		}
	}

	// Fallback: llmparse ListStrategy — convert plain list to file plans using target files as paths.
	engine := llmparse.NewEngine(
		[]llmparse.Strategy{llmparse.ListStrategy{}},
		nil,
	)
	var titles []string
	if err := engine.Parse(raw, &titles); err == nil && len(titles) > 0 {
		files := make([]codeFilePlan, 0, len(titles))
		for _, t := range titles {
			// Treat each list item as a file path if it looks like one, otherwise skip.
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			files = append(files, codeFilePlan{
				Path:    t,
				Purpose: fmt.Sprintf("Implementation for: %s", bead.Title),
				Outline: bead.Description,
				IsNew:   true,
			})
		}
		if len(files) > 0 {
			return files, tokens, nil
		}
	}

	// Last resort: synthesize from bead.TargetFiles if the LLM failed completely.
	if len(bead.TargetFiles) > 0 {
		files := make([]codeFilePlan, len(bead.TargetFiles))
		for i, tf := range bead.TargetFiles {
			files[i] = codeFilePlan{
				Path:    tf,
				Purpose: fmt.Sprintf("Implementation for: %s", bead.Title),
				Outline: bead.Description,
				IsNew:   true,
			}
		}
		return files, tokens, nil
	}

	return nil, tokens, fmt.Errorf("file planner returned no parseable file list")
}

// runCodeFileGeneratorCall asks the LLM to generate the complete content of one file.
// existing is the current file content (empty string for new files).
// priorFiles contains content of files already generated in this bead, keyed by path.
// Relevant prior files are injected so the model can reference types/interfaces from
// sibling files in the same bead.
// Returns generated content (extracted from code fence), token count, and any error.
func runCodeFileGeneratorCall(
	ctx context.Context,
	prov provider.Provider,
	modelID string,
	bead model.Bead,
	fp codeFilePlan,
	existing string,
	priorFiles map[string]string,
) (string, int, error) {
	var userMsg strings.Builder
	userMsg.WriteString(fmt.Sprintf("File: %s\n", fp.Path))
	userMsg.WriteString(fmt.Sprintf("Purpose: %s\n", fp.Purpose))
	if fp.Outline != "" {
		userMsg.WriteString(fmt.Sprintf("Must include: %s\n", fp.Outline))
	}
	userMsg.WriteString(fmt.Sprintf("\nTask: %s\n", bead.Title))
	if bead.Description != "" {
		userMsg.WriteString(fmt.Sprintf("Description:\n%s\n", bead.Description))
	}
	if existing != "" {
		userMsg.WriteString("\nExisting file content to modify:\n```\n")
		userMsg.WriteString(existing)
		userMsg.WriteString("\n```\n")
	}
	// Inject relevant prior-generated files so the model can reference
	// types, interfaces, and imports from sibling files in this bead.
	if len(priorFiles) > 0 {
		const maxPriorChars = 6000
		var priorBuf strings.Builder
		for path, content := range priorFiles {
			entry := fmt.Sprintf("\n### %s\n```\n%s\n```\n", path, content)
			if priorBuf.Len()+len(entry) > maxPriorChars {
				break
			}
			priorBuf.WriteString(entry)
		}
		if priorBuf.Len() > 0 {
			userMsg.WriteString("\nOther files already generated for this task (use for imports/types):\n")
			userMsg.WriteString(priorBuf.String())
		}
	}
	userMsg.WriteString("\nGenerate the complete file content now.")

	events, err := prov.Chat(ctx, provider.ChatRequest{
		Model:        modelID,
		SystemPrompt: agent.BuildCodeFileGeneratorSystemPrompt(),
		UserMessage:  userMsg.String(),
	})
	if err != nil {
		return "", 0, err
	}

	var fullResponse strings.Builder
	var tokens int
	for ev := range events {
		switch ev.Type {
		case "chunk":
			fullResponse.WriteString(ev.Content)
		case "tokens":
			tokens += ev.Tokens
		case "error":
			return "", tokens, fmt.Errorf("file generator: %s", ev.Content)
		}
	}

	raw := fullResponse.String()
	debugLogAgentOutput("code_file_generator_"+fp.Path, raw)

	// Extract code block via llmparse CodeStrategy.
	engine := llmparse.NewEngine(
		[]llmparse.Strategy{llmparse.CodeStrategy{}},
		nil,
	)
	var blocks []string
	if err := engine.Parse(raw, &blocks); err == nil && len(blocks) > 0 {
		return blocks[0], tokens, nil
	}

	// Fallback: return the trimmed raw response if no fences found.
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" {
		return trimmed, tokens, nil
	}

	return "", tokens, fmt.Errorf("file generator returned empty content for %s", fp.Path)
}
