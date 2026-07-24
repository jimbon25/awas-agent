package agent

import (
	"fmt"
	"strings"

	"awas/internal/client"
)

func SmartCompress(history []client.Message, maxTokens int, keepLastTurns int) []client.Message {
	if maxTokens <= 0 || len(history) == 0 {
		return history
	}

	totalTokens := EstimateTotalTokens(history)
	if totalTokens <= maxTokens {
		return history
	}

	if keepLastTurns <= 0 {
		keepLastTurns = 5
	}

	result := []client.Message{history[0]}

	userMsgCount := 0
	keepFromIdx := -1
	for i := len(history) - 1; i >= 1; i-- {
		if history[i].Role == "user" {
			userMsgCount++
			if userMsgCount == keepLastTurns {
				keepFromIdx = i
				break
			}
		}
	}
	if keepFromIdx == -1 {
		keepFromIdx = 1
	}

	for i := 1; i < keepFromIdx; i++ {
		msg := history[i]
		switch msg.Role {
		case "user":
			result = append(result, msg)
		case "tool":
			if isToolError(msg) {
				result = append(result, msg)
			} else {
				result = append(result, compressToolResult(msg))
			}
		case "assistant":
			if len(msg.Content) > 300 && len(msg.ToolCalls) == 0 {
				compressed := msg
				compressed.Content = msg.Content[:200] + " [... compressed ...]"
				result = append(result, compressed)
			} else {
				result = append(result, msg)
			}
		default:
			if msg.Role == "system" && strings.Contains(msg.Content, "[System Nudge:") {
				continue
			}
			result = append(result, msg)
		}
	}

	if len(result) < len(history)-1 {
		summary := client.Message{
			Role:    "system",
			Content: "[COMPRESSED LOG] " + summarizeCompressedSection(history[1:keepFromIdx]),
		}
		result = append(result, summary)
	}

	result = append(result, history[keepFromIdx:]...)
	return result
}

func isToolError(msg client.Message) bool {
	return strings.HasPrefix(msg.Content, "[Error]") ||
		strings.Contains(msg.Content, "error:") ||
		strings.Contains(msg.Content, "Error:") ||
		strings.Contains(msg.Content, "failed:") ||
		strings.Contains(msg.Content, "FAILED:")
}

func compressToolResult(msg client.Message) client.Message {
	content := msg.Content
	if len(content) > 1500 {
		content = content[:1000] + "\n[... truncated for context compression ...]"
	}
	return client.Message{
		Role:       msg.Role,
		ToolCallID: msg.ToolCallID,
		Name:       msg.Name,
		Content:    content,
	}
}

func summarizeCompressedSection(msgs []client.Message) string {
	toolCount := 0
	assistantCount := 0
	for _, m := range msgs {
		switch m.Role {
		case "tool":
			toolCount++
		case "assistant":
			assistantCount++
		}
	}
	return fmt.Sprintf("earlier conversation (%d tool results, %d assistant messages)", toolCount, assistantCount)
}

func CountTurns(history []client.Message) int {
	count := 0
	for _, m := range history {
		if m.Role == "user" {
			count++
		}
	}
	return count
}
