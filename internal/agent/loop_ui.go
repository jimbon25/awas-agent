package agent

import (
	"awas/internal/safety"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type DefaultUI struct{}

func (d DefaultUI) SendFile(filePath string, caption string) error {
	fmt.Printf("\n⬥ [File Output] Path: %s | Caption: %s\n", filePath, caption)
	return nil
}

func (d DefaultUI) AskUser(ctx context.Context, question string) (string, error) {
	fmt.Printf("\n❯  %s\nInput: ", question)
	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

func (d DefaultUI) PrintThinking(model string) {
	fmt.Printf("⧗ Thinking (model: %s)... \r", model)
}

func (d DefaultUI) PrintMessage(role string, content string) {
	if role == "assistant" && content != "" {
		fmt.Printf("\n⬥ %s\n", content)
	} else if role == "system" {
		fmt.Printf("\nℹ  %s\n", content)
	}
}

func (d DefaultUI) PrintMessageDelta(content string) {
	fmt.Print(content)
}

func (d DefaultUI) PrintCompression(turns int) {
	fmt.Printf("\n⤓  Earlier conversation compressed (%d turns)\n", turns)
}

func (d DefaultUI) PrintToolCall(name string, args string) {
	fmt.Printf("\n⛭  Running tool: %s(%s)\n", name, args)
}

func (d DefaultUI) PrintToolResult(name string, result string) {
	displayRes := result
	if len(displayRes) > 500 {
		displayRes = displayRes[:500] + "... (truncated)"
	}
	fmt.Printf("  ↳ Result: %s\n", displayRes)
}

func (d DefaultUI) PrintTokenUsage(count int) {
}

func (d DefaultUI) RequestApproval(ctx context.Context, toolName string, args string, mode string) bool {
	return safety.CheckApproval(toolName, args, mode)
}

func (d DefaultUI) RequestChainContinue(ctx context.Context) bool {
	fmt.Print("\n⚠  Reached consecutive tool calls limit. Continue? (y/N): ")
	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	text = strings.TrimSpace(strings.ToLower(text))
	return text == "y" || text == "yes"
}

func (d DefaultUI) PrintPlan(goal string, steps []string) {
	fmt.Printf("\n⎔  Plan: %s\n", goal)
	for i, stepDesc := range steps {
		fmt.Printf("  [ ] Step %d: %s\n", i+1, stepDesc)
	}
	fmt.Println()
}

func (d DefaultUI) PrintStepStart(stepID string) {
	fmt.Printf("  ❯ Running step: %s...\n", stepID)
}

func (d DefaultUI) PrintStepFinish(stepID string, success bool, result string) {
	icon := "✔"
	statusStr := "completed"
	if !success {
		icon = "✘"
		statusStr = "failed"
	}
	fmt.Printf("  %s Finished step: %s (%s)\n", icon, stepID, statusStr)
}

type SilentUI struct {
	SubagentID string
}

func (s SilentUI) SendFile(filePath string, caption string) error                       { return nil }
func (s SilentUI) AskUser(ctx context.Context, question string) (string, error)         { return "", fmt.Errorf("ask_user not supported in subagent") }
func (s SilentUI) PrintThinking(model string)                                           {}
func (s SilentUI) PrintMessage(role string, content string)                              {}
func (s SilentUI) PrintMessageDelta(content string)                                      {}
func (s SilentUI) PrintCompression(turns int)                                           {}
func (s SilentUI) PrintToolCall(name string, args string) {
	if s.SubagentID != "" {
		GetSubagentRegistry().UpdateStep(s.SubagentID, formatSubagentStep(name, args))
	}
}
func (s SilentUI) PrintToolResult(name string, result string)                            {}
func (s SilentUI) PrintTokenUsage(count int)                                            {}
func (s SilentUI) RequestApproval(ctx context.Context, toolName string, args string, mode string) bool {
	level := safety.GetDangerLevel(toolName)
	return level != safety.LevelDangerous
}
func (s SilentUI) RequestChainContinue(ctx context.Context) bool                       { return true }
func (s SilentUI) PrintPlan(goal string, steps []string)                                {}
func (s SilentUI) PrintStepStart(stepID string)                                         {}
func (s SilentUI) PrintStepFinish(stepID string, success bool, result string)          {}

func formatSubagentStep(name string, args string) string {
	var params map[string]any
	_ = json.Unmarshal([]byte(args), &params)

	switch name {
	case "read_file":
		if p, ok := params["path"].(string); ok {
			return "Reading " + filepath.Base(p)
		}
	case "search_code":
		if q, ok := params["query"].(string); ok {
			return "Searching " + q
		}
	case "execute_command":
		if c, ok := params["command"].(string); ok {
			if len(c) > 25 {
				c = c[:22] + "..."
			}
			return "Exec " + c
		}
	case "edit_file":
		if p, ok := params["file_path"].(string); ok {
			return "Editing " + filepath.Base(p)
		}
	case "write_file":
		if p, ok := params["path"].(string); ok {
			return "Writing " + filepath.Base(p)
		}
	case "list_directory":
		return "Listing dir"
	case "find_files":
		return "Finding files"
	}
	return "Tool " + name
}
