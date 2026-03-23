package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Dependency describes an external CLI tool that claudette depends on.
type Dependency struct {
	Name        string
	Binary      string
	VersionArg  string
	Required    bool
	URL         string // project URL for manual install reference
	InstallOpts []InstallOption
}

// InstallOption describes one way to install a dependency.
type InstallOption struct {
	Label   string
	Command string
	Args    []string
	Env     []string // additional environment variables
}

// checkInstalled returns the version string and true if the binary is found and runnable.
func checkInstalled(binary, versionArg string) (string, bool) {
	_, err := exec.LookPath(binary)
	if err != nil {
		return "", false
	}
	cmd := exec.Command(binary, versionArg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", false
	}
	version := strings.TrimSpace(string(out))
	// Take first line only
	if idx := strings.IndexByte(version, '\n'); idx >= 0 {
		version = version[:idx]
	}
	return version, true
}

// promptChoice displays a numbered menu and reads the user's selection.
// Returns the 0-based index into options, or -1 if the user chose to skip.
func promptChoice(reader *bufio.Reader, options []InstallOption) int {
	for i, opt := range options {
		fmt.Printf("    %d) %s\n", i+1, opt.Label)
	}
	fmt.Println("    0) Skip")
	fmt.Print("  Choice: ")

	line, err := reader.ReadString('\n')
	if err != nil {
		return -1
	}
	line = strings.TrimSpace(line)
	n, err := strconv.Atoi(line)
	if err != nil || n < 0 || n > len(options) {
		fmt.Println("  Invalid choice.")
		return -1
	}
	if n == 0 {
		return -1
	}
	return n - 1
}

// attemptInstall runs the install command, piping output to stdout/stderr.
func attemptInstall(opt InstallOption) error {
	fmt.Printf("  Running: %s %s\n", opt.Command, strings.Join(opt.Args, " "))
	cmd := exec.Command(opt.Command, opt.Args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(opt.Env) > 0 {
		cmd.Env = append(os.Environ(), opt.Env...)
	}
	return cmd.Run()
}
