package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

type ExecuteResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

type RunningTask struct {
	ID        string
	Command   string
	StartTime time.Time
	Cmd       *exec.Cmd
	Cancel    context.CancelFunc
}

var (
	RunningTasks  = make(map[string]*RunningTask)
	TasksMu       sync.Mutex
	TaskEventChan chan TaskEvent 
)

type TaskEvent struct {
	Type      string 
	ID        string
	Command   string
	StartTime time.Time
	Status    string
	ExitCode  int
	Output    string
}

func ExecuteCommand(workDir string, command string) string {
	command = sanitizeCommand(command)
	id := fmt.Sprintf("task-%d", time.Now().UnixNano())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		_, err := exec.LookPath("bash")
		if err != nil {
			res := ExecuteResult{
				ExitCode: -1,
				Error:    "bash not found in PATH. Install bash or use a different shell.",
			}
			resBytes, _ := json.Marshal(res)
			return string(resBytes)
		}
		cmd = exec.CommandContext(ctx, "bash", "-c", command)
	}
	cmd.Dir = workDir
	setSysProcAttr(cmd)

	t := &RunningTask{
		ID:        id,
		Command:   command,
		StartTime: time.Now(),
		Cmd:       cmd,
		Cancel:    cancel,
	}
	TasksMu.Lock()
	RunningTasks[id] = t
	TasksMu.Unlock()

	sendTaskEvent(TaskEvent{
		Type:      "started",
		ID:        id,
		Command:   command,
		StartTime: t.StartTime,
		Status:    "running",
	})

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		res := ExecuteResult{
			ExitCode: -1,
			Error:    fmt.Sprintf("stdout pipe failed: %v", err),
		}
		resBytes, _ := json.Marshal(res)
		return string(resBytes)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		res := ExecuteResult{
			ExitCode: -1,
			Error:    fmt.Sprintf("stderr pipe failed: %v", err),
		}
		resBytes, _ := json.Marshal(res)
		return string(resBytes)
	}

	if err := cmd.Start(); err != nil {
		TasksMu.Lock()
		delete(RunningTasks, id)
		TasksMu.Unlock()
		res := ExecuteResult{
			ExitCode: -1,
			Error:    err.Error(),
		}
		resBytes, _ := json.Marshal(res)
		return string(resBytes)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)

	scanPipe := func(pipe io.ReadCloser, buf *bytes.Buffer) {
		defer wg.Done()
		scanner := bufio.NewScanner(pipe)
		for scanner.Scan() {
			line := scanner.Text()
			buf.WriteString(line + "\n")
			sendTaskEvent(TaskEvent{
				Type:   "output",
				ID:     id,
				Output: line,
			})
		}
	}

	go scanPipe(stdoutPipe, &stdoutBuf)
	go scanPipe(stderrPipe, &stderrBuf)

	wg.Wait()
	err = cmd.Wait()

	// Unregister task
	TasksMu.Lock()
	delete(RunningTasks, id)
	TasksMu.Unlock()

	res := ExecuteResult{
		Stdout: stdoutBuf.String(),
		Stderr: stderrBuf.String(),
	}

	status := "completed (exit 0)"
	exitCode := 0

	if err != nil {
		if ctx.Err() == context.Canceled {
			res.ExitCode = -1
			res.Error = "command was cancelled/killed"
			status = "cancelled"
			exitCode = -1
		} else if ctx.Err() == context.DeadlineExceeded {
			res.ExitCode = -1
			res.Error = "command timed out"
			status = "timed out"
			exitCode = -1
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			if statusVal, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				res.ExitCode = statusVal.ExitStatus()
			} else {
				res.ExitCode = exitErr.ExitCode()
			}
			res.Error = exitErr.Error()
			exitCode = res.ExitCode
			status = fmt.Sprintf("failed (exit %d)", exitCode)
		} else {
			res.ExitCode = -1
			res.Error = err.Error()
			exitCode = -1
			status = "failed"
		}
	}

	// Send finished event
	sendTaskEvent(TaskEvent{
		Type:     "finished",
		ID:       id,
		Status:   status,
		ExitCode: exitCode,
	})

	resBytes, _ := json.Marshal(res)
	return string(resBytes)
}

func KillTask(id string) bool {
	TasksMu.Lock()
	t, ok := RunningTasks[id]
	TasksMu.Unlock()
	if ok && t.Cancel != nil {
		t.Cancel() // Trigger context cancellation
		if t.Cmd != nil && t.Cmd.Process != nil {
			t.Cmd.Process.Kill() // Force kill process
		}
		return true
	}
	return false
}

func RegisterTaskEventChan(ch chan TaskEvent) {
	TaskEventChan = ch
}

func sendTaskEvent(ev TaskEvent) {
	if TaskEventChan != nil {
		select {
		case TaskEventChan <- ev:
		default:
		}
	}
}

func sanitizeCommand(command string) string {
	if strings.Contains(command, "sudo") {
		words := strings.Fields(command)
		for i, w := range words {
			if w == "sudo" {
				hasNextOption := false
				if i+1 < len(words) {
					next := words[i+1]
					if strings.HasPrefix(next, "-n") || strings.HasPrefix(next, "-S") || strings.HasPrefix(next, "-A") {
						hasNextOption = true
					}
				}
				if !hasNextOption {
					words[i] = "sudo -n"
				}
			}
		}
		return strings.Join(words, " ")
	}
	return command
}
