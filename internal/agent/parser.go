package agent

import (
	"fmt"
	"strings"
)

func cleanAssistantContent(content string) string {
	content = strings.ReplaceAll(content, "</parameter>", "")
	content = strings.ReplaceAll(content, "</invoke>", "")
	content = strings.ReplaceAll(content, "</call>", "")
	return strings.TrimSpace(content)
}

func truncateToolResult(result string) string {
	lines := strings.Split(result, "\n")
	if len(lines) <= 100 {
		return result
	}

	firstPart := strings.Join(lines[:40], "\n")
	lastPart := strings.Join(lines[len(lines)-40:], "\n")
	truncatedCount := len(lines) - 80

	return fmt.Sprintf("%s\n\n... [Truncated %d lines of output for efficiency] ...\n\n%s", firstPart, truncatedCount, lastPart)
}
