package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"awas/internal/client"
)

func EstimateTotalTokens(history []client.Message) int {
	total := 0
	for _, m := range history {
		total += estimateTokens(m)
	}
	return total
}

func CompressHistory(cli *client.Client, history []client.Message, maxTokens int, model string, keepTurns int) ([]client.Message, bool, int) {
	if maxTokens <= 0 {
		return history, false, 0
	}

	totalTokens := EstimateTotalTokens(history)
	if totalTokens <= maxTokens {
		return history, false, 0
	}

	userMsgCount := 0
	firstKeepIdx := -1

	for i := 1; i < len(history); i++ {
		if history[i].Role == "user" {
			userMsgCount++
			if userMsgCount == keepTurns+1 {
				firstKeepIdx = i
				break
			}
		}
	}

	if firstKeepIdx == -1 {
		secondUserIdx := -1
		for i := 1; i < len(history); i++ {
			if history[i].Role == "user" {
				if secondUserIdx == -1 {
					secondUserIdx = i
				} else {
					secondUserIdx = i
					break
				}
			}
		}
		if secondUserIdx != -1 {
			newHistory := append([]client.Message{history[0]}, history[secondUserIdx:]...)
			return newHistory, true, 1
		}
		return history, false, 0
	}

	turnsToCompress := history[1:firstKeepIdx]
	summary, err := summarizeWithLLM(cli, model, turnsToCompress)

	var compressedMsg client.Message
	if err != nil {
		compressedMsg = client.Message{
			Role:    "system",
			Content: "[COMPRESSED LOG] (Earlier conversation turns pruned to save context window)",
		}
	} else {
		compressedMsg = client.Message{
			Role:    "system",
			Content: "[COMPRESSED LOG] Summary of earlier conversation:\n" + summary,
		}
	}

	newHistory := append([]client.Message{history[0], compressedMsg}, history[firstKeepIdx:]...)
	return newHistory, true, keepTurns
}

func summarizeWithLLM(cli *client.Client, model string, turns []client.Message) (string, error) {
	if cli == nil {
		return "", fmt.Errorf("client is nil")
	}
	var sb strings.Builder
	for _, m := range turns {
		roleLabel := strings.ToUpper(m.Role)
		if m.Role == "user" {
			sb.WriteString(fmt.Sprintf("\n%s: %s\n", roleLabel, m.Content))
		} else if m.Role == "assistant" {
			if m.Content != "" {
				sb.WriteString(fmt.Sprintf("%s: %s\n", roleLabel, m.Content))
			}
			for _, tc := range m.ToolCalls {
				sb.WriteString(fmt.Sprintf("SYSTEM (Tool Call): Called %s with args %s\n", tc.Function.Name, tc.Function.Arguments))
			}
		} else if m.Role == "tool" {
			res := m.Content
			if len(res) > 200 {
				res = res[:200] + "... (truncated)"
			}
			sb.WriteString(fmt.Sprintf("SYSTEM (Tool Result): %s returned %s\n", m.Name, res))
		}
	}

	summaryPrompt := []client.Message{
		{
			Role:    "system",
			Content: "You are a helpful assistant that summarizes conversation logs concisely.",
		},
		{
			Role:    "user",
			Content: fmt.Sprintf("Summarize the key events, findings, and decisions of the following conversation log in 2-3 concise sentences. Focus on what was achieved and what the user wanted:\n\n%s", sb.String()),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	choice, _, err := cli.Send(ctx, summaryPrompt, nil)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(choice.Message.Content), nil
}
