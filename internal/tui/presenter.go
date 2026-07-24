package tui

import (
	"context"
	"strings"

	"awas/internal/agent"
	tea "charm.land/bubbletea/v2"
)

type TUIPresenter struct {
	program *tea.Program
}

func NewTUIPresenter(p *tea.Program) *TUIPresenter {
	return &TUIPresenter{program: p}
}

var _ agent.UI = (*TUIPresenter)(nil)

func (tp *TUIPresenter) PrintThinking(model string) {
	tp.program.Send(AgentThinkingMsg{Model: model})
}

func (tp *TUIPresenter) PrintMessage(role string, content string) {
	tp.program.Send(AgentMessageMsg{Role: role, Content: content})
}

func (tp *TUIPresenter) PrintMessageDelta(content string) {
	tp.program.Send(AgentMessageDeltaMsg{Content: content})
}

func (tp *TUIPresenter) PrintToolCall(name string, args string) {
	tp.program.Send(AgentToolCallMsg{Name: name, Args: args})
}

func (tp *TUIPresenter) PrintToolResult(name string, result string) {
	success := !strings.HasPrefix(result, "[Error]")
	tp.program.Send(AgentToolResultMsg{Name: name, Result: result, Success: success})
}

func (tp *TUIPresenter) PrintTokenUsage(count int) {
	tp.program.Send(AgentTokenUsageMsg{Count: count})
}

func (tp *TUIPresenter) PrintCompression(turns int) {
	tp.program.Send(AgentCompressionMsg{Turns: turns})
}

func (tp *TUIPresenter) SendFile(filePath string, caption string) error {
	msg := "ℹ️ File output generated: " + filePath
	if caption != "" {
		msg += " | " + caption
	}
	tp.program.Send(AgentMessageMsg{Role: "system", Content: msg})
	return nil
}

func (tp *TUIPresenter) RequestApproval(ctx context.Context, toolName string, args string, mode string) bool {
	if toolName == "read_file" || toolName == "search_code" || toolName == "list_directory" {
		return true
	}
	if (toolName == "write_file" || toolName == "edit_file") && mode == "autonomous" {
		tp.program.Send(AgentMessageMsg{
			Role:    "system",
			Content: "[Auto-Approved] Running medium tool: " + toolName,
		})
		return true
	}

	respChan := make(chan bool)
	tp.program.Send(AgentApprovalRequestMsg{
		ToolName: toolName,
		Args:     args,
		RespChan: respChan,
	})

	select {
	case approved := <-respChan:
		return approved
	case <-ctx.Done():
		return false
	}
}

func (tp *TUIPresenter) RequestChainContinue(ctx context.Context) bool {
	respChan := make(chan bool)
	tp.program.Send(AgentChainLimitRequestMsg{RespChan: respChan})

	select {
	case approved := <-respChan:
		return approved
	case <-ctx.Done():
		return false
	}
}

func (tp *TUIPresenter) AskUser(ctx context.Context, question string) (string, error) {
	respChan := make(chan string)
	tp.program.Send(AgentAskUserRequestMsg{
		Question: question,
		RespChan: respChan,
	})

	select {
	case answer := <-respChan:
		return answer, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

type AgentPlanCreatedMsg struct {
	Goal  string
	Steps []string
}

type AgentPlanStepStartMsg struct {
	StepID string
}

type AgentPlanStepFinishMsg struct {
	StepID  string
	Success bool
	Result  string
}

func (tp *TUIPresenter) PrintPlan(goal string, steps []string) {
	tp.program.Send(AgentPlanCreatedMsg{Goal: goal, Steps: steps})
}

func (tp *TUIPresenter) PrintStepStart(stepID string) {
	tp.program.Send(AgentPlanStepStartMsg{StepID: stepID})
}

func (tp *TUIPresenter) PrintStepFinish(stepID string, success bool, result string) {
	tp.program.Send(AgentPlanStepFinishMsg{StepID: stepID, Success: success, Result: result})
}
