package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/michelroberge/Claudine/backend/internal/model"
)

// importStep describes one stage of the import artifact generation.
type importStep struct {
	label string
	stage model.StageName
}

var importSteps = []importStep{
	{"Generating architecture artifact...", model.StageArchitecture},
	{"Generating UX artifact...", model.StageUX},
	{"Updating architecture with journey cross-references...", model.StageArchitecture},
	{"Generating vision document...", model.StageVision},
}

// ImportResult holds the generated artifacts from an import analysis.
type ImportResult struct {
	Artifacts map[model.StageName]string
}

// StreamImport analyzes an existing codebase and generates all pipeline artifacts.
// It emits progress events via the emit callback and returns the generated artifacts.
func StreamImport(ctx context.Context, hostDir, version, projectName string, emit func(StreamEvent)) (*ImportResult, error) {
	digest, err := buildCodebaseDigest(hostDir)
	if err != nil {
		return nil, fmt.Errorf("build codebase digest: %w", err)
	}

	artifacts := make(map[model.StageName]string)

	// Step 1: Architecture
	emit(StreamEvent{Type: "log", Content: importSteps[0].label})
	archPrompt := buildImportArchitecturePrompt(version, digest)
	archContent, err := runImportAgent(ctx, archPrompt, projectName, hostDir)
	if err != nil {
		return nil, fmt.Errorf("architecture agent: %w", err)
	}
	if extracted, ok := ExtractArtifact(archContent); ok {
		archContent = extracted
	}
	artifacts[model.StageArchitecture] = archContent
	emit(StreamEvent{Type: "artifact", Content: "architecture"})

	// Step 2: UX
	emit(StreamEvent{Type: "log", Content: importSteps[1].label})
	uxPrompt := buildImportUXPrompt(version, digest, archContent)
	uxContent, err := runImportAgent(ctx, uxPrompt, projectName, hostDir)
	if err != nil {
		return nil, fmt.Errorf("ux agent: %w", err)
	}
	if extracted, ok := ExtractArtifact(uxContent); ok {
		uxContent = extracted
	}
	artifacts[model.StageUX] = uxContent
	emit(StreamEvent{Type: "artifact", Content: "ux"})

	// Step 3: Update architecture with JRN cross-references
	emit(StreamEvent{Type: "log", Content: importSteps[2].label})
	archUpdatePrompt := buildImportArchCrossRefPrompt(version, archContent, uxContent)
	updatedArch, err := runImportAgent(ctx, archUpdatePrompt, projectName, hostDir)
	if err != nil {
		return nil, fmt.Errorf("architecture cross-ref agent: %w", err)
	}
	if extracted, ok := ExtractArtifact(updatedArch); ok {
		updatedArch = extracted
	}
	artifacts[model.StageArchitecture] = updatedArch
	emit(StreamEvent{Type: "artifact", Content: "architecture"})

	// Step 4: Vision
	emit(StreamEvent{Type: "log", Content: importSteps[3].label})
	visionPrompt := buildImportVisionPrompt(projectName, updatedArch, uxContent)
	visionContent, err := runImportAgent(ctx, visionPrompt, projectName, hostDir)
	if err != nil {
		return nil, fmt.Errorf("vision agent: %w", err)
	}
	if extracted, ok := ExtractArtifact(visionContent); ok {
		visionContent = extracted
	}
	artifacts[model.StageVision] = visionContent
	emit(StreamEvent{Type: "artifact", Content: "vision"})

	// Step 5: Build (programmatic — single imported milestone)
	emit(StreamEvent{Type: "log", Content: "Generating build artifact..."})
	artifacts[model.StageBuild] = buildImportBuildArtifact(projectName, version)
	emit(StreamEvent{Type: "artifact", Content: "build"})

	return &ImportResult{Artifacts: artifacts}, nil
}

// runImportAgent calls Claude with a system prompt and collects the full response.
func runImportAgent(ctx context.Context, systemPrompt, projectName, projectDir string) (string, error) {
	userMessage := fmt.Sprintf("Analyze this codebase for project '%s' and produce the requested artifact.", projectName)

	events, err := Chat(ctx, "claude-sonnet-4-6", systemPrompt, nil, userMessage, projectDir)
	if err != nil {
		return "", err
	}

	var result strings.Builder
	for ev := range events {
		if ev.Type == "chunk" {
			result.WriteString(ev.Content)
		}
	}
	return result.String(), nil
}

// buildCodebaseDigest reads the project directory and produces a structured
// summary of the codebase for injection into import prompts.
func buildCodebaseDigest(hostDir string) (string, error) {
	var sb strings.Builder
	sb.WriteString("# Codebase File Tree\n\n")

	// Collect all files, excluding common non-source directories
	excludeDirs := map[string]bool{
		".git": true, "node_modules": true, "vendor": true, "__pycache__": true,
		".next": true, "dist": true, "build": true, ".claudine": true,
		".venv": true, "venv": true, "target": true, "bin": true, "obj": true,
		".idea": true, ".vscode": true, "coverage": true, ".cache": true,
	}

	var files []string
	filepath.Walk(hostDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(hostDir, path)
		if rel == "." {
			return nil
		}

		// Skip excluded directories
		if info.IsDir() {
			if excludeDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		files = append(files, rel)
		return nil
	})

	// Print file tree
	for _, f := range files {
		sb.WriteString(f)
		sb.WriteString("\n")
	}

	// Prioritize reading certain files
	priorityPatterns := []string{
		"package.json", "go.mod", "Cargo.toml", "requirements.txt", "pyproject.toml",
		"Dockerfile", "docker-compose.yml", "docker-compose.yaml",
		"Makefile", "CMakeLists.txt",
		".env.example", "README.md",
	}

	// Categorize files for smart reading
	var configFiles, entryFiles, routeFiles, modelFiles, sourceFiles []string

	for _, f := range files {
		base := filepath.Base(f)
		lower := strings.ToLower(base)

		// Config/manifest files
		for _, pat := range priorityPatterns {
			if strings.EqualFold(base, pat) {
				configFiles = append(configFiles, f)
				break
			}
		}

		// Entry points
		if strings.HasPrefix(lower, "main.") || strings.HasPrefix(lower, "app.") ||
			strings.HasPrefix(lower, "index.") || strings.HasPrefix(lower, "server.") {
			entryFiles = append(entryFiles, f)
		}

		// Route files
		if strings.Contains(lower, "route") || strings.Contains(lower, "router") ||
			strings.Contains(lower, "handler") || strings.Contains(lower, "controller") {
			routeFiles = append(routeFiles, f)
		}

		// Model/schema files
		if strings.Contains(lower, "model") || strings.Contains(lower, "schema") ||
			strings.Contains(lower, "types") || strings.Contains(lower, "entity") {
			modelFiles = append(modelFiles, f)
		}

		// General source files
		ext := filepath.Ext(lower)
		if isSourceExt(ext) {
			sourceFiles = append(sourceFiles, f)
		}
	}

	// Read files in priority order, up to ~100K characters
	const maxChars = 100_000
	currentChars := sb.Len()

	readFile := func(rel string) {
		if currentChars >= maxChars {
			return
		}
		content, err := os.ReadFile(filepath.Join(hostDir, rel))
		if err != nil {
			return
		}
		// Skip binary or very large files
		if len(content) > 50_000 || isBinary(content) {
			return
		}
		sb.WriteString(fmt.Sprintf("\n---\n## File: %s\n```\n%s\n```\n", rel, string(content)))
		currentChars += len(content) + 50
	}

	// Deduplicate and read in order
	seen := make(map[string]bool)
	readAll := func(list []string) {
		for _, f := range list {
			if !seen[f] {
				seen[f] = true
				readFile(f)
			}
		}
	}

	readAll(configFiles)
	readAll(entryFiles)
	readAll(routeFiles)
	readAll(modelFiles)

	// Sort remaining source files by path for determinism
	sort.Strings(sourceFiles)
	readAll(sourceFiles)

	return sb.String(), nil
}

func isSourceExt(ext string) bool {
	switch ext {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".rs", ".java", ".kt",
		".rb", ".php", ".cs", ".swift", ".vue", ".svelte", ".html", ".css",
		".scss", ".sql", ".graphql", ".proto", ".yaml", ".yml", ".toml", ".json":
		return true
	}
	return false
}

func isBinary(data []byte) bool {
	// Check first 512 bytes for null bytes
	check := data
	if len(check) > 512 {
		check = check[:512]
	}
	for _, b := range check {
		if b == 0 {
			return true
		}
	}
	return false
}

// --- Import-specific system prompts ---

func buildImportArchitecturePrompt(version, digest string) string {
	return fmt.Sprintf(`You are analyzing an existing codebase to reverse-engineer its architecture documentation.

Here is the codebase structure and source code:
---
%s
---

Analyze the codebase and produce a System Architecture document describing what exists.

%s

When ready, produce the architecture document wrapped in these exact delimiters:

<!-- ARTIFACT:START -->
# System Architecture: {Product Name}

## Tech Stack
Identify all technologies, frameworks, and languages used.

## System Components
Describe each major component/service/module.

## API Design
Document existing API endpoints, their methods, and purposes.

## Data Models
Describe the data structures and their relationships.

## Infrastructure
Note any deployment, CI/CD, or infrastructure patterns observed.
<!-- ARTIFACT:END -->

Always wrap the document in exactly those delimiters. Be thorough — document what actually exists, not what should exist.`, digest, buildArchIDNote(version))
}

func buildImportUXPrompt(version, digest, archContent string) string {
	return fmt.Sprintf(`You are analyzing an existing codebase to reverse-engineer its UX documentation.

Here is the codebase structure and source code:
---
%s
---

Here is the architecture analysis of this codebase:
---
%s
---

Analyze the codebase (especially routes, pages, screens, UI components) and produce a UX document describing the existing user experience.

%s

When ready, produce the UX document wrapped in these exact delimiters:

<!-- ARTIFACT:START -->
# UX Design: {Product Name}

## User Journeys
Describe each user journey you can identify from the codebase.

## Screen Descriptions
Describe the screens/pages that exist.

## Navigation Flow
How do users navigate between screens?

## Interaction Patterns
What UI patterns and interactions are used?
<!-- ARTIFACT:END -->

Always wrap the document in exactly those delimiters. Document what actually exists in the code.`, digest, archContent, buildJourneyIDNote(version))
}

func buildImportArchCrossRefPrompt(version, archContent, uxContent string) string {
	return fmt.Sprintf(`You are updating an architecture document to add cross-references to user journey IDs.

Here is the current architecture document:
---
%s
---

Here is the UX document with journey IDs:
---
%s
---

Update the architecture document to add journey cross-references (JRN-v%s-NNN) to each architectural element, using the existing journey IDs from the UX document.

%s

Produce the COMPLETE updated architecture document wrapped in these exact delimiters:

<!-- ARTIFACT:START -->
{complete updated architecture document with journey cross-references}
<!-- ARTIFACT:END -->

Always wrap the document in exactly those delimiters.`, archContent, uxContent, version, buildArchIDNote(version))
}

func buildImportVisionPrompt(projectName, archContent, uxContent string) string {
	return fmt.Sprintf(`You are analyzing an existing codebase to reverse-engineer its product vision.

Here is the architecture analysis:
---
%s
---

Here is the UX analysis:
---
%s
---

Based on these analyses, synthesize a Product Vision document that captures what this product is, who it's for, and what it does.

When ready, produce the vision document wrapped in these exact delimiters:

<!-- ARTIFACT:START -->
# Product Vision: %s

## Problem Statement
What problem does this product solve? Why does it matter?

## Target Users
Who are the primary users? What are their needs?

## Core Features
The essential capabilities this product provides.

## User Experience
How users interact with this product. Key interaction patterns.

## Success Metrics
How would we know this product is working?

## Constraints & Assumptions
Technical, business, or other constraints observed.

## Out of Scope (V1)
What is explicitly not included in the current version.
<!-- ARTIFACT:END -->

Always wrap the document in exactly those delimiters. Infer the vision from what the code actually does.`, archContent, uxContent, projectName)
}

func buildImportBuildArtifact(projectName, version string) string {
	return fmt.Sprintf(`# Build Plan: %s

## Milestones

### Milestone 1: Initial Import
**Goal:** Baseline — the existing codebase as imported.
**Tasks:**
- [x] Import existing codebase (v%s)

## Dependencies
None — this is the initial import baseline.

## Risks & Unknowns
The codebase was imported as-is. A thorough review of the auto-generated artifacts is recommended before enhancement.

## Definition of Done
All pipeline artifacts reviewed and approved by the user.`, projectName, version)
}
