//go:build windows

package pipeline

import "os/exec"

func setSysProcAttr(cmd *exec.Cmd) {
	// Windows does not support process groups via SysProcAttr.Setpgid
}

// killProcessGroup kills the process directly on Windows (no process group support).
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		cmd.Process.Kill()
	}
}
