package agent

import (
	"awas/internal/client"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func (l *Loop) RunAgentCycle(ctx context.Context, userInput string) {
	InvalidateSkillsCache()
	l.history = append(l.history, client.Message{
		Role:    "user",
		Content: userInput,
	})

	l.TurnCount++
	interval := l.cfg.NudgeInterval
	if interval <= 0 {
		interval = 10
	}
	if l.TurnCount > 0 && l.TurnCount%interval == 0 {
		l.history = append(l.history, client.Message{
			Role:    "system",
			Content: "[System Nudge: Please review the recent conversation turns. If the user provided new preferences, corrections, feedback, or there are new environment facts, use the 'manage_memory' tool to update USER.md or MEMORY.md before answering. If no updates are needed, proceed normally.]",
		})
	}

	if l.cfg.AgentMode == "planned" || l.cfg.AgentMode == "deep" {
		l.UI.PrintMessage("system", "⎔ Planning execution phase started...")
		plan, err := l.generatePlan(ctx, userInput)
		if err != nil {
			l.UI.PrintMessage("system", fmt.Sprintf("[Error] Failed to generate plan: %v", err))
			return
		}

		stepDescs := make([]string, len(plan.Steps))
		for idx, st := range plan.Steps {
			stepDescs[idx] = fmt.Sprintf("%s|%s", st.ID, st.Description)
		}
		l.UI.PrintPlan(plan.Goal, stepDescs)

		for idx := range plan.Steps {
			step := &plan.Steps[idx]
			step.Status = StepRunning
			l.UI.PrintStepStart(step.ID)

			argsBytes, _ := json.Marshal(step.Args)
			toolCall := client.ToolCall{
				ID:   fmt.Sprintf("call_%s", step.ID),
				Type: "function",
				Function: client.ToolFunction{
					Name:      step.Tool,
					Arguments: string(argsBytes),
				},
			}

			l.UI.PrintToolCall(toolCall.Function.Name, toolCall.Function.Arguments)
			approved := l.UI.RequestApproval(ctx, toolCall.Function.Name, toolCall.Function.Arguments, l.cfg.Mode)

			var result string
			if approved {
				if err := ctx.Err(); err != nil {
					l.printInterruptOrTimeout(ctx)
					return
				}
				result = l.executeTool(ctx, toolCall)
				l.UI.PrintToolResult(toolCall.Function.Name, result)
			} else {
				result = "[Error] Tool execution rejected by user."
				l.UI.PrintToolResult(toolCall.Function.Name, result)
			}

			isSuccess := approved && !strings.HasPrefix(result, "[Error]")

			if l.cfg.AgentMode == "deep" && !isSuccess && approved {
				retries := 0
				maxRetries := l.cfg.MaxRetries
				if maxRetries <= 0 {
					maxRetries = 3
				}

				review := l.reviewStepResult(ctx, plan.Goal, step, result)
				for review.ShouldRetry && retries < maxRetries {
					retries++
					l.UI.PrintMessage("system", fmt.Sprintf("⟳ Retrying step %s (Attempt %d/%d) using strategy: %s...", step.ID, retries, maxRetries, review.Strategy))

					if review.Strategy == StrategyModifyArgs && review.Args != nil {
						step.Args = review.Args
						argsBytes, _ := json.Marshal(step.Args)
						toolCall.Function.Arguments = string(argsBytes)
					} else if review.Strategy == StrategyAlternative && review.Tool != "" && review.Args != nil {
						step.Tool = review.Tool
						step.Args = review.Args
						toolCall.Function.Name = step.Tool
						argsBytes, _ := json.Marshal(step.Args)
						toolCall.Function.Arguments = string(argsBytes)
					}

					if err := ctx.Err(); err != nil {
						l.printInterruptOrTimeout(ctx)
						return
					}
					result = l.executeTool(ctx, toolCall)
					l.UI.PrintToolResult(toolCall.Function.Name, result)

					isSuccess = !strings.HasPrefix(result, "[Error]")
					if isSuccess {
						break
					}
					review = l.reviewStepResult(ctx, plan.Goal, step, result)
				}
			}

			if !isSuccess {
				step.Status = StepFailed
				l.UI.PrintStepFinish(step.ID, false, result)
				l.UI.PrintMessage("system", fmt.Sprintf("✘ Plan execution halted at step %s.", step.ID))
				return
			}

			step.Status = StepCompleted
			l.UI.PrintStepFinish(step.ID, true, result)

			compactedResult := truncateToolResult(result)
			l.history = append(l.history, client.Message{
				Role:    "assistant",
				Content: fmt.Sprintf("Executed planned step %s: %s", step.ID, step.Description),
			})
			l.history = append(l.history, client.Message{
				Role:       "tool",
				ToolCallID: toolCall.ID,
				Name:       toolCall.Function.Name,
				Content:    compactedResult,
			})
		}

		l.UI.PrintMessage("system", "✔ All planned steps completed successfully!")
		return
	}

	toolChainCount := 0

	for {
		if err := ctx.Err(); err != nil {
			l.printInterruptOrTimeout(ctx)
			return
		}

		l.UI.PrintThinking(l.cfg.Model)
		var toolsList []client.Tool
		if l.cfg.AgentMode == "chat" {
			toolsList = getChatTools()
		} else {
			toolsList = getTools()
		}
		choice, usage, err := l.cli.Send(ctx, l.history, toolsList)
		if err != nil {
			if ctx.Err() != nil {
				l.printInterruptOrTimeout(ctx)
			} else {
				l.UI.PrintMessage("system", fmt.Sprintf("[Error] from API: %v", err))
			}
			return
		}

		if usage != nil {
			l.UI.PrintTokenUsage(usage.TotalTokens)
		}

		if choice.Message.Content != "" {
			choice.Message.Content = cleanAssistantContent(choice.Message.Content)
			if choice.Message.Content != "" {
				l.UI.PrintMessage("assistant", choice.Message.Content)
			}
		}

		l.history = append(l.history, choice.Message)

		if choice.FinishReason != "tool_calls" || len(choice.Message.ToolCalls) == 0 {
			break
		}

		toolCalls := choice.Message.ToolCalls

		for _, toolCall := range toolCalls {
			if err := ctx.Err(); err != nil {
				l.printInterruptOrTimeout(ctx)
				return
			}
			toolChainCount++
			if l.cfg.Mode != "autonomous" && l.cfg.MaxChainLimit > 0 && toolChainCount > l.cfg.MaxChainLimit {
				if !l.UI.RequestChainContinue(ctx) {
					l.UI.PrintMessage("system", "⚠ Tool chain terminated by user.")
					return
				}
				toolChainCount = 0 
			}

			l.UI.PrintToolCall(toolCall.Function.Name, toolCall.Function.Arguments)
			approved := l.UI.RequestApproval(ctx, toolCall.Function.Name, toolCall.Function.Arguments, l.cfg.Mode)
			
			var result string
			if approved {
				if err := ctx.Err(); err != nil {
					l.printInterruptOrTimeout(ctx)
					return
				}
				result = l.executeTool(ctx, toolCall)
				l.UI.PrintToolResult(toolCall.Function.Name, result)
			} else {
				result = "[Error] Tool execution rejected by user."
				l.UI.PrintToolResult(toolCall.Function.Name, result)
			}

			compactedResult := truncateToolResult(result)

			l.history = append(l.history, client.Message{
				Role:       "tool",
				ToolCallID: toolCall.ID,
				Name:       toolCall.Function.Name,
				Content:    compactedResult,
			})
		}
	}

	newHist := SmartCompress(l.history, l.cfg.MaxTokens, l.cfg.KeepLastTurns)
	if len(newHist) < len(l.history) {
		l.history = newHist
		l.UI.PrintCompression(CountTurns(l.history))
	} else {
		newHist, compressed, turns := CompressHistory(l.cli, l.history, l.cfg.MaxTokens, l.cfg.Model, 5)
		if compressed {
			l.history = newHist
			l.UI.PrintCompression(turns)
		}
	}
}

func (l *Loop) RunAgentCycleStream(ctx context.Context, userInput string) {
	InvalidateSkillsCache()
	if l.cfg.AgentMode == "planned" || l.cfg.AgentMode == "deep" {
		l.RunAgentCycle(ctx, userInput)
		return
	}

	l.history = append(l.history, client.Message{
		Role:    "user",
		Content: userInput,
	})

	l.TurnCount++
	interval := l.cfg.NudgeInterval
	if interval <= 0 {
		interval = 10
	}
	if l.TurnCount > 0 && l.TurnCount%interval == 0 {
		l.history = append(l.history, client.Message{
			Role:    "system",
			Content: "[System Nudge: Please review the recent conversation turns. If the user provided new preferences, corrections, feedback, or there are new environment facts, use the 'manage_memory' tool to update USER.md or MEMORY.md before answering. If no updates are needed, proceed normally.]",
		})
	}

	toolChainCount := 0

	for {
		if err := ctx.Err(); err != nil {
			l.printInterruptOrTimeout(ctx)
			return
		}

		l.UI.PrintThinking(l.cfg.Model)

		var toolsList []client.Tool
		if l.cfg.AgentMode == "chat" {
			toolsList = getChatTools()
		} else {
			toolsList = getTools()
		}
		sc, err := l.cli.SendStream(ctx, l.history, toolsList)
		if err != nil {
			if ctx.Err() != nil {
				l.printInterruptOrTimeout(ctx)
			} else {
				l.UI.PrintMessage("system", fmt.Sprintf("[Error] from API: %v", err))
			}
			return
		}

		result := client.AccumulateStream(sc)

		if result.Usage != nil {
			l.UI.PrintTokenUsage(result.Usage.TotalTokens)
		}

		if result.Content != "" {
			cleaned := cleanAssistantContent(result.Content)
			if cleaned != "" {
				l.UI.PrintMessage("assistant", cleaned)
			}
		}

		assistantMsg := client.Message{
			Role:    "assistant",
			Content: result.Content,
		}

		if len(result.ToolCalls) > 0 {
			assistantMsg.ToolCalls = result.ToolCalls
		}

		l.history = append(l.history, assistantMsg)

		if result.FinishReason != "tool_calls" || len(result.ToolCalls) == 0 {
			break
		}

		for _, toolCall := range result.ToolCalls {
			if err := ctx.Err(); err != nil {
				l.printInterruptOrTimeout(ctx)
				return
			}
			toolChainCount++
			if l.cfg.Mode != "autonomous" && l.cfg.MaxChainLimit > 0 && toolChainCount > l.cfg.MaxChainLimit {
				if !l.UI.RequestChainContinue(ctx) {
					l.UI.PrintMessage("system", "⚠︎ Tool chain terminated by user.")
					return
				}
				toolChainCount = 0
			}

			l.UI.PrintToolCall(toolCall.Function.Name, toolCall.Function.Arguments)
			approved := l.UI.RequestApproval(ctx, toolCall.Function.Name, toolCall.Function.Arguments, l.cfg.Mode)

			var result string
			if approved {
				if err := ctx.Err(); err != nil {
					l.printInterruptOrTimeout(ctx)
					return
				}
				result = l.executeTool(ctx, toolCall)
				l.UI.PrintToolResult(toolCall.Function.Name, result)
			} else {
				result = "[Error] Tool execution rejected by user."
				l.UI.PrintToolResult(toolCall.Function.Name, result)
			}

			compactedResult := truncateToolResult(result)

			l.history = append(l.history, client.Message{
				Role:       "tool",
				ToolCallID: toolCall.ID,
				Name:       toolCall.Function.Name,
				Content:    compactedResult,
			})
		}
	}

	newHist := SmartCompress(l.history, l.cfg.MaxTokens, l.cfg.KeepLastTurns)
	if len(newHist) < len(l.history) {
		l.history = newHist
		l.UI.PrintCompression(CountTurns(l.history))
	} else {
		newHist, compressed, turns := CompressHistory(l.cli, l.history, l.cfg.MaxTokens, l.cfg.Model, 5)
		if compressed {
			l.history = newHist
			l.UI.PrintCompression(turns)
		}
	}
}
