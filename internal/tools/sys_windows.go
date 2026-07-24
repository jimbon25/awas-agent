//go:build windows

package tools

import "os/exec"

func setSysProcAttr(cmd *exec.Cmd) {
	// No Setsid on Windows platforms
}
