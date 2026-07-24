package tui

import (
	"awas/internal/agent"
	"awas/internal/tools"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func handleAgentMessage(m *Model, msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case WorkspaceFilesMsg:
		m.WorkspaceFiles = msg
		return nil, true

	case AgentCancelInitMsg:
		m.AgentCancel = msg.Cancel
		return nil, true

	case AgentThinkingMsg:
		wasThinking := m.State == StateThinking
		m.State = StateThinking
		m.ThinkingModel = msg.Model
		m.ThinkingTicks = 0
		m.IsStreaming = true
		recalculateViewportHeight(m)
		if wasThinking {
			return nil, true
		}
		return tickThinking(), true

	case ThinkingTickMsg:
		if m.State == StateThinking {
			m.ThinkingTicks++
			return tickThinking(), true
		}
		return nil, true

	case AgentPlanCreatedMsg:
		m.ActivePlanGoal = msg.Goal
		m.ActivePlanSteps = msg.Steps
		m.ActivePlanStepStatuses = make(map[string]string)
		for _, rawStep := range msg.Steps {
			parts := strings.SplitN(rawStep, "|", 2)
			if len(parts) == 2 {
				m.ActivePlanStepStatuses[parts[0]] = "pending"
			}
		}
		m.TypewriterRunes = nil
		m.TypewriterIndex = 0
		m.TypewriterMsgIndex = -1
		updateViewportContent(m)
		recalculateViewportHeight(m)
		return nil, true

	case AgentPlanStepStartMsg:
		if m.ActivePlanStepStatuses != nil {
			m.ActivePlanStepStatuses[msg.StepID] = "running"
		}
		updateViewportContent(m)
		recalculateViewportHeight(m)
		return nil, true

	case AgentPlanStepFinishMsg:
		if m.ActivePlanStepStatuses != nil {
			if msg.Success {
				m.ActivePlanStepStatuses[msg.StepID] = "completed"
			} else {
				m.ActivePlanStepStatuses[msg.StepID] = "failed"
			}
		}
		updateViewportContent(m)
		recalculateViewportHeight(m)
		return nil, true

	case AgentMessageMsg:
		if msg.Role == "assistant" && msg.Content != "" {
			m.TypewriterRunes = []rune(msg.Content)
			m.TypewriterIndex = 0
			m.Messages = append(m.Messages, UIMessage{
				Role:    msg.Role,
				Content: "",
			})
			m.TypewriterMsgIndex = len(m.Messages) - 1
			updateViewportContent(m)
			recalculateViewportHeight(m)
			return tickTypewriter(), true
		}

		m.Messages = append(m.Messages, UIMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
		updateViewportContent(m)
		recalculateViewportHeight(m)
		return nil, true

	case AgentMessageDeltaMsg:
		if len(m.Messages) > 0 && m.Messages[len(m.Messages)-1].Role == "assistant" {
			lastIdx := len(m.Messages) - 1
			m.Messages[lastIdx].Content += msg.Content
			delete(m.RenderedLines, lastIdx) 
		} else {
			m.Messages = append(m.Messages, UIMessage{
				Role:    "assistant",
				Content: msg.Content,
			})
		}
		updateViewportContent(m)
		return nil, true

	case TypewriterTickMsg:
		if m.TypewriterRunes != nil && m.TypewriterIndex < len(m.TypewriterRunes) {
			chunkSize := 20 
			nextIdx := m.TypewriterIndex + chunkSize
			if nextIdx > len(m.TypewriterRunes) {
				nextIdx = len(m.TypewriterRunes)
			}

			chunk := string(m.TypewriterRunes[m.TypewriterIndex:nextIdx])
			if m.TypewriterMsgIndex >= 0 && m.TypewriterMsgIndex < len(m.Messages) {
				m.Messages[m.TypewriterMsgIndex].Content += chunk
				delete(m.RenderedLines, m.TypewriterMsgIndex)
			}
			m.TypewriterIndex = nextIdx
			updateViewportContent(m)
			return tickTypewriter(), true
		}
		m.TypewriterRunes = nil
		m.TypewriterIndex = 0
		m.TypewriterMsgIndex = -1
		return nil, true

	case AgentToolCallMsg:
		m.Messages = append(m.Messages, UIMessage{
			Role:    "tool_call",
			Name:    msg.Name,
			Content: msg.Args,
		})
		updateViewportContent(m)
		recalculateViewportHeight(m)
		return nil, true

	case tools.TaskEvent:
		if msg.Type == "output" {
			m.LastTaskOutput = msg.Output
			updateViewportContent(m)
			return nil, true
		}
		if msg.Type == "started" {
			m.ActiveTaskCount++
			found := false
			for i, t := range m.Tasks {
				if t.ID == msg.ID {
					m.Tasks[i].Status = msg.Status
					m.Tasks[i].StartTime = msg.StartTime
					found = true
					break
				}
			}
			if !found {
				m.Tasks = append(m.Tasks, Task{
					ID:        msg.ID,
					Command:   msg.Command,
					StartTime: msg.StartTime,
					Status:    msg.Status,
				})
			}
		} else if msg.Type == "finished" {
			m.ActiveTaskCount--
			if m.ActiveTaskCount < 0 {
				m.ActiveTaskCount = 0
			}
			for i, t := range m.Tasks {
				if t.ID == msg.ID {
					m.Tasks[i].Status = msg.Status
					m.Tasks[i].ExitCode = msg.ExitCode
					break
				}
			}
		}
		return nil, true

	case AgentToolResultMsg:
		m.Messages = append(m.Messages, UIMessage{
			Role:    "tool_result",
			Name:    msg.Name,
			Content: msg.Result,
			Success: msg.Success,
		})
		updateViewportContent(m)
		recalculateViewportHeight(m)
		return nil, true

	case AgentApprovalRequestMsg:
		var isAutoApproved bool
		if msg.ToolName == "execute_command" {
			var args map[string]any
			if json.Unmarshal([]byte(msg.Args), &args) == nil {
				cmd, _ := args["command"].(string)
				fields := strings.Fields(cmd)
				if len(fields) > 0 {
					prefix := fields[0]
					for _, p := range m.AutoApproveCommands {
						if p == prefix {
							isAutoApproved = true
							break
						}
					}
				}
			}
		}
		if m.AutoApproveTools[msg.ToolName] {
			isAutoApproved = true
		}

		if isAutoApproved {
			go func(ch chan<- bool) {
				ch <- true
			}(msg.RespChan)
			return nil, true
		}

		m.State = StateApprovalPending
		m.PendingTool = msg.ToolName
		m.PendingArgs = msg.Args
		m.ApprovalChan = msg.RespChan
		m.ApprovalCursor = 0 
		recalculateViewportHeight(m)
		return nil, true

	case AgentChainLimitRequestMsg:
		m.State = StateChainLimitPending
		m.ApprovalChan = msg.RespChan
		recalculateViewportHeight(m)
		return nil, true

	case AgentAskUserRequestMsg:
		m.State = StateAskUserPending
		m.PendingQuestion = msg.Question
		m.AskUserChan = msg.RespChan
		m.Messages = append(m.Messages, UIMessage{
			Role:    "system",
			Content: "❯ " + msg.Question,
		})
		updateViewportContent(m)
		recalculateViewportHeight(m)
		return nil, true

	case AgentCompressionMsg:
		m.CompressedTurns += msg.Turns
		if m.Loop != nil {

			var newUIMessages []UIMessage
			for _, hMsg := range m.Loop.GetHistory() {
				if hMsg.Role == "system" {
					if strings.Contains(hMsg.Content, "[COMPRESSED") {
						newUIMessages = append(newUIMessages, UIMessage{
							Role:    "system",
							Content: "⤓ Earlier conversation compressed",
						})
					}
					continue
				}

				uiMsg := UIMessage{
					Role:    hMsg.Role,
					Content: hMsg.Content,
				}
				if hMsg.Role == "assistant" && len(hMsg.ToolCalls) > 0 {
					var calls []string
					for _, tc := range hMsg.ToolCalls {
						action, detail := formatToolCallAgyStyle(tc.Function.Name, tc.Function.Arguments)
						if len(detail) > 60 {
							detail = detail[:57] + "..."
						}
						detail = strings.ReplaceAll(detail, "\n", " ")
						calls = append(calls, fmt.Sprintf("● %s(%s)", action, detail))
					}
					if uiMsg.Content != "" {
						uiMsg.Content += "\n\n" + strings.Join(calls, "\n")
					} else {
						uiMsg.Content = strings.Join(calls, "\n")
					}
				}
				newUIMessages = append(newUIMessages, uiMsg)
			}
			m.Messages = newUIMessages
			m.TokenCount = agent.EstimateTotalTokens(m.Loop.GetHistory())
		}
		updateViewportContent(m)
		recalculateViewportHeight(m)
		return nil, true

	case AgentTokenUsageMsg:
		m.TokenCount = msg.Count
		recalculateViewportHeight(m)
		return nil, true

	case AgentFinishedMsg:
		m.AgentCancel = nil 
		m.IsStreaming = false

		if len(m.QueryQueue) > 0 {
			nextQuery := m.QueryQueue[0]
			m.QueryQueue = m.QueryQueue[1:]

			for i := len(m.Messages) - 1; i >= 0; i-- {
				if m.Messages[i].Role == "user" && strings.HasSuffix(m.Messages[i].Content, " (queued)") {
					prefix := strings.TrimSuffix(m.Messages[i].Content, " (queued)")
					if prefix == nextQuery {
						m.Messages[i].Content = nextQuery
						break
					}
				}
			}
			updateViewportContent(m)

			m.State = StateThinking
			m.ThinkingTicks = 0
			recalculateViewportHeight(m)

			ctx, cancel := context.WithCancel(context.Background())
			m.AgentCancel = cancel

			go func(input string, c context.Context) {
				m.PromptChan <- AgentPrompt{Prompt: input, Ctx: c}
			}(nextQuery, ctx)

			return tea.Batch(
				tickThinking(),
			), true
		}

		if m.State == StateThinking {
			m.State = StateIdle
		} else {
			m.PreviousState = StateIdle
		}
		recalculateViewportHeight(m)
		generateSessionTitle(m) 
		return nil, true
	}

	return nil, false
}
