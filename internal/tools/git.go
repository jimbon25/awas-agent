package tools

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func GitOps(workDir string, action string, message string, branch string) string {
	action = strings.ToLower(strings.TrimSpace(action))
	var args []string

	switch action {
	case "status":
		args = []string{"status"}
	case "diff":
		args = []string{"diff"}
	case "commit":
		if message == "" {
			return "[Error] commit message is required"
		}
		args = []string{"commit", "-am", message}
	case "push":
		args = []string{"push", "origin"}
		if branch != "" {
			args = append(args, branch)
		}
	case "pull":
		args = []string{"pull", "origin"}
		if branch != "" {
			args = append(args, branch)
		}
	case "checkout":
		if branch == "" {
			return "[Error] branch name is required for checkout"
		}
		args = []string{"checkout", branch}
	case "log":
		args = []string{"log", "-n", "10", "--oneline"}
	default:
		return fmt.Sprintf("[Error] unknown git action: %q", action)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = workDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	errStr := stderr.String()

	if err != nil {
		return fmt.Sprintf("[Error] Git command failed: %v\nStderr: %s\nStdout: %s", err, errStr, output)
	}

	if output == "" && errStr == "" {
		return "Command executed successfully (no output)."
	}

	if output == "" && errStr != "" {
		return errStr
	}

	return output
}
