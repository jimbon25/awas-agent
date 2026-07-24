package cron

import (
	"context"
	"strings"
)

type CronUI struct {
	accumulated []string
}

func NewCronUI() *CronUI {
	return &CronUI{
		accumulated: make([]string, 0),
	}
}

func (c *CronUI) GetOutput() string {
	return strings.Join(c.accumulated, "\n\n")
}

func (c *CronUI) PrintMessage(role string, content string) {
	if content == "" {
		return
	}
	if role == "assistant" {
		c.accumulated = append(c.accumulated, content)
	} else if role == "system" {
		c.accumulated = append(c.accumulated, "ℹ "+content)
	}
}

func (c *CronUI) PrintMessageDelta(content string)                   {}
func (c *CronUI) PrintThinking(model string)                         {}
func (c *CronUI) PrintTokenUsage(tokens int)                         {}
func (c *CronUI) PrintToolCall(name string, args string)             {}
func (c *CronUI) PrintToolResult(name string, result string)         {}
func (c *CronUI) PrintCompression(turns int)                        {}

func (c *CronUI) RequestApproval(ctx context.Context, toolName string, args string, mode string) bool {
	return true
}

func (c *CronUI) RequestChainContinue(ctx context.Context) bool {
	return true
}

func (c *CronUI) AskUser(ctx context.Context, question string) (string, error) {
	return "[Error] ask_user is not supported in non-interactive background cron jobs.", nil
}

func (c *CronUI) PrintPlan(goal string, steps []string)                               {}
func (c *CronUI) PrintStepStart(stepID string)                                         {}
func (c *CronUI) PrintStepFinish(stepID string, success bool, result string)           {}

func (c *CronUI) SendFile(filePath string, caption string) error {
	msg := "✦ File output generated: " + filePath
	if caption != "" {
		msg += " | " + caption
	}
	c.accumulated = append(c.accumulated, msg)
	return nil
}
