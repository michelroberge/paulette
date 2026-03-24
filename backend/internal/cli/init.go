package cli

import (
	"bufio"
	"fmt"
	"os"
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

// RunInit checks all dependencies and helps install missing ones.
// Returns an exit code: 0 for success, 1 for failure.
func RunInit() int {
	fmt.Println("Claudine init - checking dependencies...")
	fmt.Println()

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
			fmt.Printf("  %s is %s for Claudine.\n", dep.Name, label)
		} else {
			fmt.Printf("  %s is %s but recommended.\n", dep.Name, label)
		}
		fmt.Printf("  For manual install, see: %s\n", dep.URL)
		fmt.Println("  Install options:")

		choice := promptChoice(reader, dep.InstallOpts)
		if choice < 0 {
			if dep.Required {
				fmt.Printf("  %s is required. Claudine cannot run without it.\n\n", dep.Name)
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
				fmt.Printf("  %s is required. Claudine cannot run without it.\n\n", dep.Name)
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
		fmt.Println("Some required dependencies are missing. Please install them and run 'Claudine init' again.")
		return 1
	}

	fmt.Println("All dependencies satisfied. You're ready to run Claudine!")
	return 0
}
