package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/michelroberge/paulette/backend/internal/config"
)

var dependencies = []Dependency{
	{
		Name:       "beads",
		Binary:     "bd",
		VersionArg: "--version",
		Required:   true,
		URL:        "https://github.com/steveyegge/beads",
		InstallOpts: []InstallOption{
			{Label: "curl script", Command: "sh", Args: []string{"-c", "curl -fsSL https://raw.githubusercontent.com/steveyegge/beads/main/scripts/install.sh | bash"}},
			{Label: "npm install -g @beads/bd", Command: "npm", Args: []string{"install", "-g", "@beads/bd"}},
			{Label: "homebrew", Command: "brew", Args: []string{"install", "beads"}, Env: []string{"HOMEBREW_NO_AUTO_UPDATE=1"}},
			{Label: "go install", Command: "go", Args: []string{"install", "github.com/steveyegge/beads/cmd/bd@latest"}},
		},
	},
	{
		Name:       "claude",
		Binary:     "claude",
		VersionArg: "--version",
		Required:   true,
		URL:        "https://docs.anthropic.com/en/docs/claude-code",
		InstallOpts: []InstallOption{
			{Label: "npm install -g @anthropic-ai/claude-code", Command: "npm", Args: []string{"install", "-g", "@anthropic-ai/claude-code"}},
		},
	},
	{
		Name:       "rtk",
		Binary:     "rtk",
		VersionArg: "--version",
		Required:   false,
		URL:        "https://github.com/rtk-ai/rtk",
		InstallOpts: []InstallOption{
			{Label: "homebrew", Command: "brew", Args: []string{"install", "rtk"}, Env: []string{"HOMEBREW_NO_AUTO_UPDATE=1"}},
			{Label: "curl script", Command: "sh", Args: []string{"-c", "curl -fsSL https://raw.githubusercontent.com/rtk-ai/rtk/refs/heads/master/install.sh | sh"}},
			{Label: "cargo install", Command: "cargo", Args: []string{"install", "--git", "https://github.com/rtk-ai/rtk"}},
		},
	},
}

// parseFlag extracts --name=<value> from os.Args (if present).
func parseFlag(name string) string {
	prefix := "--" + name + "="
	for _, arg := range os.Args[2:] {
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimPrefix(arg, prefix)
		}
	}
	return ""
}

// RunInit checks all dependencies and helps install missing ones.
// Accepts an optional --repos-path=<path> flag to configure where projects are stored.
// Returns an exit code: 0 for success, 1 for failure.
func RunInit() int {
	fmt.Println("paulette init - checking dependencies...")
	fmt.Println()

	// Handle optional --repos-path flag
	if rp := parseFlag("repos-path"); rp != "" {
		absPath, err := filepath.Abs(rp)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid repos path: %v\n", err)
			return 1
		}
		settings := config.LoadSettings()
		settings.ReposPath = absPath
		if err := config.SaveSettings(settings); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to save settings: %v\n", err)
			return 1
		}
		fmt.Printf("Repos path set to: %s\n\n", absPath)
	}

	// Handle optional --claude-path flag
	if cp := parseFlag("claude-path"); cp != "" {
		absPath, err := filepath.Abs(cp)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid claude path: %v\n", err)
			return 1
		}
		settings := config.LoadSettings()
		settings.ClaudePath = absPath
		if err := config.SaveSettings(settings); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to save settings: %v\n", err)
			return 1
		}
		fmt.Printf("Claude path set to: %s\n\n", absPath)
	}

	reader := bufio.NewReader(os.Stdin)
	allOK := true

	for i, dep := range dependencies {
		label := "optional"
		if dep.Required {
			label = "REQUIRED"
		}

		fmt.Printf("[%d/%d] Checking %s (%s)...", i+1, len(dependencies), dep.Name, dep.Binary)

		version, found := checkInstalled(dep.Binary, dep.VersionArg)
		if found {
			fmt.Printf(" found (%s)\n", version)
			continue
		}

		fmt.Println(" not found")

		if dep.Required {
			fmt.Printf("  %s is %s for paulette.\n", dep.Name, label)
		} else {
			fmt.Printf("  %s is %s but recommended.\n", dep.Name, label)
		}
		fmt.Printf("  For manual install, see: %s\n", dep.URL)
		fmt.Println("  Install options:")

		choice := promptChoice(reader, dep.InstallOpts)
		if choice < 0 {
			if dep.Required {
				fmt.Printf("  %s is required. paulette cannot run without it.\n\n", dep.Name)
				allOK = false
			} else {
				fmt.Printf("  Skipping %s (optional).\n\n", dep.Name)
			}
			continue
		}

		err := attemptInstall(dep.InstallOpts[choice])
		if err != nil {
			fmt.Printf("  Install failed: %v\n", err)
		}

		// Re-verify after install attempt
		version, found = checkInstalled(dep.Binary, dep.VersionArg)
		if found {
			fmt.Printf("  %s installed successfully (%s)\n\n", dep.Name, version)
		} else {
			fmt.Printf("  %s is still not found in PATH.\n", dep.Binary)
			fmt.Println("  You may need to restart your shell or add it to your PATH.")
			if dep.Required {
				fmt.Printf("  %s is required. paulette cannot run without it.\n\n", dep.Name)
				allOK = false
			} else {
				fmt.Printf("  Skipping %s (optional).\n\n", dep.Name)
			}
		}
	}

	// Summary
	fmt.Println("--- Summary ---")
	for _, dep := range dependencies {
		version, found := checkInstalled(dep.Binary, dep.VersionArg)
		status := "OK (" + version + ")"
		if !found {
			if dep.Required {
				status = "MISSING (required)"
			} else {
				status = "NOT INSTALLED (optional)"
			}
		}
		fmt.Printf("  %-10s %s\n", dep.Name+":", status)
	}
	fmt.Println()

	if !allOK {
		fmt.Println("Some required dependencies are missing. Please install them and run 'paulette init' again.")
		return 1
	}

	cfg := config.Load()
	fmt.Println("All dependencies satisfied. You're ready to run paulette!")
	fmt.Printf("  Repos path: %s\n", cfg.ReposPath)
	return 0
}
