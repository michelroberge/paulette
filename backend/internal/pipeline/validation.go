package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/michelroberge/paulette/backend/internal/agent"
	"github.com/michelroberge/paulette/backend/internal/model"
)

// Command category constants.
const (
	CmdDev   = "dev"
	CmdBuild = "build"
	CmdRun   = "run"
)

// Timeouts for each command category.
const (
	DevTimeout   = 30 * time.Second
	BuildTimeout = 5 * time.Minute
	RunTimeout   = 15 * time.Second
)

// MaxValidationRetries is the maximum number of fix-and-retry cycles.
const MaxValidationRetries = 3

// ValidationResult captures the outcome of running a single validation command.
type ValidationResult struct {
	Command  string        `json:"command"`
	Category string        `json:"category"`
	ExitCode int           `json:"exitCode"`
	Output   string        `json:"output"`
	Success  bool          `json:"success"`
	Duration time.Duration `json:"duration"`
}

// ValidationCommands groups the three command categories.
type ValidationCommands struct {
	Dev   []string
	Build []string
	Run   []string
}

// HasCommands returns true if any validation commands are configured.
func (vc ValidationCommands) HasCommands() bool {
	return len(vc.Dev) > 0 || len(vc.Build) > 0 || len(vc.Run) > 0
}

// CommandsFromProject extracts validation commands from a project.
func CommandsFromProject(p *model.Project) ValidationCommands {
	return ValidationCommands{
		Dev:   p.DevCommands,
		Build: p.BuildCommands,
		Run:   p.RunCommands,
	}
}

// RunValidation runs all validation commands in order: dev, build, run.
// It emits SSE events via the emit callback and returns the results plus an overall pass/fail.
func RunValidation(ctx context.Context, projectDir string, cmds ValidationCommands, emit func(agent.StreamEvent)) ([]ValidationResult, bool) {
	var results []ValidationResult
	allPassed := true

	emit(agent.StreamEvent{Type: "validation_start", Content: "Starting validation gate..."})

	// Run each category in order.
	for _, cat := range []struct {
		name     string
		commands []string
		timeout  time.Duration
		longRun  bool
	}{
		{CmdDev, cmds.Dev, DevTimeout, true},
		{CmdBuild, cmds.Build, BuildTimeout, false},
		{CmdRun, cmds.Run, RunTimeout, true},
	} {
		for _, cmdStr := range cat.commands {
			emit(agent.StreamEvent{
				Type:    "validation_cmd",
				Content: fmt.Sprintf(`{"command":%q,"category":%q,"status":"running"}`, cmdStr, cat.name),
			})

			var result ValidationResult
			if cat.longRun {
				result = runLongRunning(ctx, projectDir, cmdStr, cat.name, cat.timeout)
			} else {
				result = runToCompletion(ctx, projectDir, cmdStr, cat.name, cat.timeout)
			}
			results = append(results, result)

			status := "pass"
			if !result.Success {
				status = "fail"
				allPassed = false
			}
			emit(agent.StreamEvent{
				Type:    "validation_cmd",
				Content: fmt.Sprintf(`{"command":%q,"category":%q,"status":%q,"exitCode":%d,"duration":%q}`, cmdStr, cat.name, status, result.ExitCode, result.Duration),
			})
		}
	}

	passStr := "true"
	if !allPassed {
		passStr = "false"
	}
	emit(agent.StreamEvent{
		Type:    "validation_done",
		Content: fmt.Sprintf(`{"passed":%s,"total":%d}`, passStr, len(results)),
	})

	return results, allPassed
}

// runToCompletion runs a command to completion and checks exit code.
func runToCompletion(ctx context.Context, dir, cmdStr, category string, timeout time.Duration) ValidationResult {
	start := time.Now()
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(tctx, "sh", "-c", cmdStr)
	cmd.Dir = dir
	setSysProcAttr(cmd)

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	duration := time.Since(start)
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	output := buf.String()
	// Truncate output to avoid massive bead descriptions.
	if len(output) > 4000 {
		output = output[:2000] + "\n\n... (truncated) ...\n\n" + output[len(output)-2000:]
	}

	return ValidationResult{
		Command:  cmdStr,
		Category: category,
		ExitCode: exitCode,
		Output:   output,
		Success:  exitCode == 0,
		Duration: duration,
	}
}

// runLongRunning starts a process and watches it for the given timeout.
// If the process exits with non-zero before the timeout, it's a failure.
// If the process is still alive at timeout (no error), it's a success — we kill it.
func runLongRunning(ctx context.Context, dir, cmdStr, category string, timeout time.Duration) ValidationResult {
	start := time.Now()

	cmd := exec.Command("sh", "-c", cmdStr)
	cmd.Dir = dir
	setSysProcAttr(cmd)

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Start(); err != nil {
		return ValidationResult{
			Command:  cmdStr,
			Category: category,
			ExitCode: -1,
			Output:   "Failed to start: " + err.Error(),
			Success:  false,
			Duration: time.Since(start),
		}
	}

	// Wait for process to finish or timeout.
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- cmd.Wait()
	}()

	select {
	case err := <-doneCh:
		// Process exited before timeout.
		duration := time.Since(start)
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}
		output := buf.String()
		if len(output) > 4000 {
			output = output[:2000] + "\n\n... (truncated) ...\n\n" + output[len(output)-2000:]
		}
		return ValidationResult{
			Command:  cmdStr,
			Category: category,
			ExitCode: exitCode,
			Output:   output,
			Success:  exitCode == 0,
			Duration: duration,
		}

	case <-time.After(timeout):
		// Process still running — that's success for long-running commands.
		// Kill the process group.
		killProcessGroup(cmd)
		return ValidationResult{
			Command:  cmdStr,
			Category: category,
			ExitCode: 0,
			Output:   buf.String(),
			Success:  true,
			Duration: time.Since(start),
		}

	case <-ctx.Done():
		killProcessGroup(cmd)
		return ValidationResult{
			Command:  cmdStr,
			Category: category,
			ExitCode: -1,
			Output:   "Cancelled: " + ctx.Err().Error(),
			Success:  false,
			Duration: time.Since(start),
		}
	}
}


// backtickCmdRe matches a backtick-wrapped command in a markdown bullet.
// e.g. "- `npm run build`" or "- `go build ./...` — compiles everything"
var backtickCmdRe = regexp.MustCompile("^\\s*[-*]\\s+`([^`]+)`")

// ParseValidationCommands extracts dev, build, and run commands from
// the "## Validation Commands" section of an architecture document.
func ParseValidationCommands(content string) (dev, build, run []string) {
	// Find the Validation Commands section.
	sectionIdx := strings.Index(content, "## Validation Commands")
	if sectionIdx < 0 {
		return nil, nil, nil
	}
	section := content[sectionIdx:]

	// Trim at the next ## heading (that's not a ### sub-heading).
	if nextH2 := findNextH2(section[len("## Validation Commands"):]); nextH2 >= 0 {
		section = section[:len("## Validation Commands")+nextH2]
	}

	// Split by subsections.
	var currentCategory *[]string
	for _, line := range strings.Split(section, "\n") {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "### Dev") {
			currentCategory = &dev
			continue
		}
		if strings.HasPrefix(trimmed, "### Build") {
			currentCategory = &build
			continue
		}
		if strings.HasPrefix(trimmed, "### Run") {
			currentCategory = &run
			continue
		}

		// Extract command from bullet.
		if currentCategory != nil {
			if m := backtickCmdRe.FindStringSubmatch(line); len(m) > 1 {
				cmd := strings.TrimSpace(m[1])
				if cmd != "" && cmd != "command here" {
					*currentCategory = append(*currentCategory, cmd)
				}
			}
		}
	}

	return dev, build, run
}

// findNextH2 finds the offset of the next "## " heading (but not "### ") in s.
func findNextH2(s string) int {
	for i := 0; i < len(s); i++ {
		if i == 0 || (i > 0 && s[i-1] == '\n') {
			rest := s[i:]
			if strings.HasPrefix(rest, "## ") && !strings.HasPrefix(rest, "### ") {
				return i
			}
		}
	}
	return -1
}

// FormatFailureSummary builds a description for a fix bead from failed validation results.
func FormatFailureSummary(results []ValidationResult) string {
	var sb strings.Builder
	sb.WriteString("The following validation commands failed after code generation. Fix the code so these commands succeed.\n\n")
	for _, r := range results {
		if !r.Success {
			sb.WriteString(fmt.Sprintf("## Command: `%s` (category: %s)\n", r.Command, r.Category))
			sb.WriteString(fmt.Sprintf("Exit code: %d\n", r.ExitCode))
			sb.WriteString(fmt.Sprintf("Output:\n```\n%s\n```\n\n", r.Output))
		}
	}
	return sb.String()
}
