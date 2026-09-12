//go:build !windows

package tools

import (
	"os/exec"
	"syscall"
)

func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil && cmd.Process.Pid > 0 {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
