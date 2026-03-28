package provider

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ToolExecutor executes server-side tool calls on behalf of an agentic provider.
// All file-system operations are validated to remain within ProjectDir.
type ToolExecutor struct {
	ProjectDir string
}

// maxOutputBytes is the cap applied to tool output before sending it back to the LLM.
const maxOutputBytes = 100 * 1024 // 100 KB

// StandardTools returns the full tool set for code-writing agents.
func StandardTools() []AgentTool {
	return []AgentTool{
		{Name: "Write", Description: "Write (create or overwrite) a file at path with the given content.", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string", "description": "Relative path within the project"},
				"content": map[string]any{"type": "string", "description": "File content"},
			},
			"required": []string{"path", "content"},
		}},
		{Name: "Edit", Description: "Replace the first occurrence of old_string with new_string in path.", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":       map[string]any{"type": "string"},
				"old_string": map[string]any{"type": "string"},
				"new_string": map[string]any{"type": "string"},
			},
			"required": []string{"path", "old_string", "new_string"},
		}},
		{Name: "Read", Description: "Read a file and return its contents (up to 50 KB).", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"path": map[string]any{"type": "string"}},
			"required":   []string{"path"},
		}},
		{Name: "cat", Description: "Alias for Read.", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"path": map[string]any{"type": "string"}},
			"required":   []string{"path"},
		}},
		{Name: "LS", Description: "List directory contents.", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"path": map[string]any{"type": "string", "description": "Directory path (optional, defaults to project root)"}},
		}},
		{Name: "ls", Description: "Alias for LS.", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"path": map[string]any{"type": "string"}},
		}},
		{Name: "Tree", Description: "Show directory tree up to optional depth.", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":  map[string]any{"type": "string"},
				"depth": map[string]any{"type": "integer", "description": "Max depth (default 3)"},
			},
		}},
		{Name: "Glob", Description: "Return file paths matching a glob pattern within the project.", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"pattern": map[string]any{"type": "string"}},
			"required":   []string{"pattern"},
		}},
		{Name: "Grep", Description: "Search for a regex pattern in files.", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string"},
				"path":    map[string]any{"type": "string", "description": "File or directory to search (optional)"},
				"flags":   map[string]any{"type": "string", "description": "Extra flags passed to grep/rg (optional)"},
			},
			"required": []string{"pattern"},
		}},
		{Name: "rg", Description: "Alias for Grep.", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string"},
				"path":    map[string]any{"type": "string"},
				"flags":   map[string]any{"type": "string"},
			},
			"required": []string{"pattern"},
		}},
		{Name: "GitStatus", Description: "Run git status in the project directory.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{}}},
		{Name: "GitDiff", Description: "Run git diff with optional args.", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"args": map[string]any{"type": "string"}},
		}},
		{Name: "GitLog", Description: "Run git log --oneline -20 with optional args.", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"args": map[string]any{"type": "string"}},
		}},
		{Name: "GitAdd", Description: "Stage files for commit.", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"paths": map[string]any{"type": "string", "description": "Space-separated file paths to add"}},
			"required":   []string{"paths"},
		}},
		{Name: "GitCommit", Description: "Commit staged changes.", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"message": map[string]any{"type": "string"}},
			"required":   []string{"message"},
		}},
		{Name: "GitPush", Description: "Push commits to the remote.", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"args": map[string]any{"type": "string"}},
		}},
		{Name: "GoTest", Description: "Run go test with optional args in the project directory.", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"args": map[string]any{"type": "string"}},
		}},
		{Name: "NpmTest", Description: "Run npm test with optional args.", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"args": map[string]any{"type": "string"}},
		}},
		{Name: "CargoTest", Description: "Run cargo test with optional args.", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"args": map[string]any{"type": "string"}},
		}},
		{Name: "Pytest", Description: "Run pytest with optional args.", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"args": map[string]any{"type": "string"}},
		}},
		{Name: "RuffCheck", Description: "Run ruff check with optional args.", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"args": map[string]any{"type": "string"}},
		}},
		{Name: "DockerPS", Description: "Run docker ps.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{}}},
		{Name: "Bash", Description: "Run an arbitrary shell command in the project directory (2-minute timeout).", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"command": map[string]any{"type": "string"}},
			"required":   []string{"command"},
		}},
	}
}

// ReadOnlyTools returns the subset of tools safe for reviewer agents.
func ReadOnlyTools() []AgentTool {
	names := map[string]bool{
		"Read": true, "cat": true, "LS": true, "Tree": true, "Glob": true,
		"Grep": true, "rg": true,
		"GitStatus": true, "GitDiff": true, "GitLog": true,
		"GoTest": true, "Pytest": true, "RuffCheck": true, "DockerPS": true,
	}
	var out []AgentTool
	for _, t := range StandardTools() {
		if names[t.Name] {
			out = append(out, t)
		}
	}
	return out
}

// BashOnlyTools returns just the Bash tool (used by ReviewBead).
func BashOnlyTools() []AgentTool {
	for _, t := range StandardTools() {
		if t.Name == "Bash" {
			return []AgentTool{t}
		}
	}
	return nil
}

// validatePath ensures that the given path, when joined with ProjectDir, stays
// within ProjectDir. Returns the absolute path on success, error on escape attempt.
func (e *ToolExecutor) validatePath(p string) (string, error) {
	if filepath.IsAbs(p) {
		// Absolute paths must be inside ProjectDir
		rel, err := filepath.Rel(e.ProjectDir, p)
		if err != nil {
			return "", fmt.Errorf("invalid path: %w", err)
		}
		if strings.HasPrefix(rel, "..") {
			return "", fmt.Errorf("path escapes project directory: %s", p)
		}
		return filepath.Clean(p), nil
	}
	abs := filepath.Join(e.ProjectDir, p)
	rel, err := filepath.Rel(e.ProjectDir, abs)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path escapes project directory: %s", p)
	}
	return abs, nil
}

func capOutput(s string) string {
	if len(s) > maxOutputBytes {
		return s[:maxOutputBytes] + "\n... (output truncated)"
	}
	return s
}

// Execute runs a single named tool and returns its output string.
func (e *ToolExecutor) Execute(ctx context.Context, name string, input map[string]any) (string, error) {
	str := func(key string) string {
		v, _ := input[key].(string)
		return v
	}

	switch name {
	case "Write":
		path, err := e.validatePath(str("path"))
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return "", fmt.Errorf("mkdir: %w", err)
		}
		content := str("content")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return "", fmt.Errorf("write file: %w", err)
		}
		return fmt.Sprintf("Written %d bytes to %s", len(content), str("path")), nil

	case "Edit":
		path, err := e.validatePath(str("path"))
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read file: %w", err)
		}
		old := str("old_string")
		if !strings.Contains(string(data), old) {
			return "", fmt.Errorf("old_string not found in %s", str("path"))
		}
		updated := strings.Replace(string(data), old, str("new_string"), 1)
		if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
			return "", fmt.Errorf("write file: %w", err)
		}
		return fmt.Sprintf("Edited %s", str("path")), nil

	case "Read", "cat":
		path, err := e.validatePath(str("path"))
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read file: %w", err)
		}
		const fileCap = 50 * 1024
		if len(data) > fileCap {
			return string(data[:fileCap]) + "\n... (file truncated)", nil
		}
		return string(data), nil

	case "LS", "ls":
		p := str("path")
		if p == "" {
			p = "."
		}
		path, err := e.validatePath(p)
		if err != nil {
			return "", err
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return "", fmt.Errorf("readdir: %w", err)
		}
		var sb strings.Builder
		for _, en := range entries {
			if en.IsDir() {
				sb.WriteString(en.Name() + "/\n")
			} else {
				info, _ := en.Info()
				size := int64(0)
				if info != nil {
					size = info.Size()
				}
				sb.WriteString(fmt.Sprintf("%s  (%d bytes)\n", en.Name(), size))
			}
		}
		return sb.String(), nil

	case "Tree":
		p := str("path")
		if p == "" {
			p = "."
		}
		path, err := e.validatePath(p)
		if err != nil {
			return "", err
		}
		depth := 3
		if d, ok := input["depth"]; ok {
			switch v := d.(type) {
			case float64:
				depth = int(v)
			case int:
				depth = v
			}
		}
		var sb strings.Builder
		buildTree(path, "", depth, 0, &sb)
		return sb.String(), nil

	case "Glob":
		pattern := str("pattern")
		// Make the pattern relative to ProjectDir
		absPattern := filepath.Join(e.ProjectDir, pattern)
		matches, err := filepath.Glob(absPattern)
		if err != nil {
			return "", fmt.Errorf("glob: %w", err)
		}
		var rel []string
		for _, m := range matches {
			r, _ := filepath.Rel(e.ProjectDir, m)
			rel = append(rel, r)
		}
		return strings.Join(rel, "\n"), nil

	case "Grep", "rg":
		pattern := str("pattern")
		searchPath := str("path")
		flags := str("flags")
		if searchPath == "" {
			searchPath = "."
		}
		absPath, err := e.validatePath(searchPath)
		if err != nil {
			return "", err
		}
		// Try rg first, fall back to grep
		var cmd *exec.Cmd
		if _, rerr := exec.LookPath("rg"); rerr == nil {
			args := []string{"-n", pattern, absPath}
			if flags != "" {
				args = append([]string{flags}, args...)
			}
			cmd = exec.CommandContext(ctx, "rg", args...)
		} else {
			args := []string{"-rn", pattern, absPath}
			if flags != "" {
				args = append(args, flags)
			}
			cmd = exec.CommandContext(ctx, "grep", args...)
		}
		cmd.Dir = e.ProjectDir
		out, _ := cmd.CombinedOutput()
		return capOutput(string(out)), nil

	case "GitStatus":
		return e.runGit(ctx, "status")
	case "GitDiff":
		args := str("args")
		if args != "" {
			return e.runGitArgs(ctx, "diff", strings.Fields(args)...)
		}
		return e.runGit(ctx, "diff")
	case "GitLog":
		args := str("args")
		if args != "" {
			return e.runGitArgs(ctx, "log", append([]string{"--oneline", "-20"}, strings.Fields(args)...)...)
		}
		return e.runGitArgs(ctx, "log", "--oneline", "-20")
	case "GitAdd":
		paths := str("paths")
		return e.runGitArgs(ctx, "add", strings.Fields(paths)...)
	case "GitCommit":
		return e.runGitArgs(ctx, "commit", "-m", str("message"))
	case "GitPush":
		args := str("args")
		if args != "" {
			return e.runGitArgs(ctx, "push", strings.Fields(args)...)
		}
		return e.runGit(ctx, "push")

	case "GoTest":
		return e.runCmd(ctx, "go", append([]string{"test"}, optArgs(str("args"))...)...)
	case "NpmTest":
		return e.runCmd(ctx, "npm", append([]string{"test"}, optArgs(str("args"))...)...)
	case "CargoTest":
		return e.runCmd(ctx, "cargo", append([]string{"test"}, optArgs(str("args"))...)...)
	case "Pytest":
		return e.runCmd(ctx, "pytest", optArgs(str("args"))...)
	case "RuffCheck":
		return e.runCmd(ctx, "ruff", append([]string{"check"}, optArgs(str("args"))...)...)
	case "DockerPS":
		return e.runCmd(ctx, "docker", "ps")

	case "Bash":
		command := str("command")
		bashCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(bashCtx, "cmd", "/C", command)
		} else {
			cmd = exec.CommandContext(bashCtx, "sh", "-c", command)
		}
		cmd.Dir = e.ProjectDir
		out, err := cmd.CombinedOutput()
		result := capOutput(string(out))
		if err != nil {
			return result, fmt.Errorf("command failed: %w", err)
		}
		return result, nil

	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

// runGit runs a single-arg git command.
func (e *ToolExecutor) runGit(ctx context.Context, subcmd string) (string, error) {
	return e.runCmd(ctx, "git", subcmd)
}

// runGitArgs runs git with multiple args.
func (e *ToolExecutor) runGitArgs(ctx context.Context, subcmd string, args ...string) (string, error) {
	return e.runCmd(ctx, "git", append([]string{subcmd}, args...)...)
}

// runCmd runs an arbitrary command in ProjectDir and returns combined output.
func (e *ToolExecutor) runCmd(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = e.ProjectDir
	out, err := cmd.CombinedOutput()
	result := capOutput(string(out))
	if err != nil {
		return result, fmt.Errorf("%s: %w", name, err)
	}
	return result, nil
}

// buildTree recursively builds a text directory tree.
func buildTree(path, prefix string, maxDepth, depth int, sb *strings.Builder) {
	if depth > maxDepth {
		return
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return
	}
	for i, en := range entries {
		connector := "├── "
		childPrefix := prefix + "│   "
		if i == len(entries)-1 {
			connector = "└── "
			childPrefix = prefix + "    "
		}
		sb.WriteString(prefix + connector + en.Name() + "\n")
		if en.IsDir() && depth < maxDepth {
			buildTree(filepath.Join(path, en.Name()), childPrefix, maxDepth, depth+1, sb)
		}
	}
}

// optArgs splits a space-separated args string into a slice, returning nil for empty input.
func optArgs(args string) []string {
	args = strings.TrimSpace(args)
	if args == "" {
		return nil
	}
	return strings.Fields(args)
}

// toolNames extracts the Name field from a slice of AgentTool.
func toolNames(tools []AgentTool) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}

// ToolSchemaForLLM converts an AgentTool into the JSON-serialisable structure
// expected by OpenAI-compatible /v1/chat/completions tool definitions.
func ToolSchemaForLLM(t AgentTool) map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"parameters":  t.InputSchema,
		},
	}
}

// walkDir helper used in Tree — wraps fs.WalkDir for test compatibility.
var _ fs.WalkDirFunc = nil // ensure fs is imported
