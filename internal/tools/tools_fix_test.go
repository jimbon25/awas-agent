package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestReindentToMatch(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		targetStyle string
		want        string
	}{
		{
			name:        "4 spaces preserved with 4 spaces target",
			input:       "    foo()",
			targetStyle: "    ",
			want:        "    foo()",
		},
		{
			name:        "8 spaces preserved with 4 spaces target",
			input:       "        bar()",
			targetStyle: "    ",
			want:        "        bar()",
		},
		{
			name:        "4 spaces converted to 1 tab",
			input:       "    foo()",
			targetStyle: "\t",
			want:        "\tfoo()",
		},
		{
			name:        "4 spaces converted to 2 spaces",
			input:       "    foo()",
			targetStyle: "  ",
			want:        "  foo()",
		},
		{
			name:        "2 spaces converted to 4 spaces",
			input:       "  foo()",
			targetStyle: "    ",
			want:        "    foo()",
		},
		{
			name:        "multi-line nested indent",
			input:       "    func test() {\n        return 1\n    }",
			targetStyle: "    ",
			want:        "    func test() {\n        return 1\n    }",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := reindentToMatch(tc.input, tc.targetStyle)
			if got != tc.want {
				t.Errorf("reindentToMatch(%q, %q) = %q; want %q", tc.input, tc.targetStyle, got, tc.want)
			}
		})
	}
}

func TestDownloadFileValidation(t *testing.T) {
	workDir := t.TempDir()

	// Relative and absolute path traversal attempts should fail
	traversalPaths := []string{
		"../escape.txt",
		"../../etc/passwd",
		"foo/../../escape.txt",
		"..",
		"/etc/passwd",
		"/tmp/some_absolute_escape.txt",
	}

	for _, p := range traversalPaths {
		res := DownloadFile(workDir, "http://example.com/test.zip", p)
		if !strings.HasPrefix(res, "[Error]") {
			t.Errorf("expected error for traversal path %q, got: %s", p, res)
		}
	}

	// Empty checks
	if res := DownloadFile(workDir, "", "valid.txt"); !strings.Contains(res, "url is required") {
		t.Errorf("expected url required error, got: %s", res)
	}
	if res := DownloadFile(workDir, "http://example.com/test.zip", ""); !strings.Contains(res, "path is required") {
		t.Errorf("expected path required error, got: %s", res)
	}
}

func TestKillTaskProcessGroup(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "child.pid")

	eventCh := make(chan TaskEvent, 10)
	RegisterTaskEventChan(eventCh)
	defer RegisterTaskEventChan(nil)

	// Command spawns a background sleep and writes its pid
	cmdStr := "sleep 100 & echo $! > " + pidFile + " && wait"

	done := make(chan string)
	go func() {
		res := ExecuteCommand(tmpDir, cmdStr)
		done <- res
	}()

	var taskID string
	select {
	case ev := <-eventCh:
		if ev.Type == "started" {
			taskID = ev.ID
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for task start event")
	}

	// Wait for child PID to be written
	var childPID int
	for i := 0; i < 50; i++ {
		data, err := os.ReadFile(pidFile)
		if err == nil && len(strings.TrimSpace(string(data))) > 0 {
			var p int
			if _, err := parsePID(strings.TrimSpace(string(data)), &p); err == nil && p > 0 {
				childPID = p
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	if childPID == 0 {
		t.Fatal("child pid file was not populated")
	}

	// Kill task (which should kill process group including the background sleep)
	killed := KillTask(taskID)
	if !killed {
		t.Fatalf("KillTask(%q) returned false", taskID)
	}

	// Wait for ExecuteCommand to return
	select {
	case res := <-done:
		var execRes ExecuteResult
		_ = json.Unmarshal([]byte(res), &execRes)
		t.Logf("ExecuteCommand finished with exit code %d, err: %s", execRes.ExitCode, execRes.Error)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for ExecuteCommand to exit after kill")
	}

	// Give OS brief moment to clean up process
	time.Sleep(100 * time.Millisecond)

	// Verify child process is dead
	// In unix, sending signal 0 checks if process exists
	err := syscall.Kill(childPID, 0)
	if err == nil {
		// Clean up leaked process if test failed
		_ = syscall.Kill(childPID, syscall.SIGKILL)
		t.Fatalf("child process %d is still alive after KillTask! Process group kill failed.", childPID)
	}
}

func parsePID(s string, out *int) (bool, error) {
	var val int
	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			val = val*10 + int(ch-'0')
		} else {
			break
		}
	}
	if val > 0 {
		*out = val
		return true, nil
	}
	return false, os.ErrInvalid
}
