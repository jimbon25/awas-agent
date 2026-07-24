package agent

import (
	"awas/internal/safety"
	"bufio"
	"context"
	"fmt"
	"os"
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
