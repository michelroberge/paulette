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

			content, genTokens, genErr := runCodeFileGeneratorCall(ctx, prov, modelID, bead, fp, existing)
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
// Returns generated content (extracted from code fence), token count, and any error.
func runCodeFileGeneratorCall(
	ctx context.Context,
	prov provider.Provider,
	modelID string,
	bead model.Bead,
	fp codeFilePlan,
	existing string,
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
