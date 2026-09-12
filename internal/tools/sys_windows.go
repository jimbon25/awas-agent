//go:build windows

package tools

import "os/exec"

func setSysProcAttr(cmd *exec.Cmd) {
	// No Setsid on Windows platforms
}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
