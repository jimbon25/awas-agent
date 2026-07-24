package tui

import (
	"awas/internal/client"
	"awas/internal/config"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func formatToolCallAgyStyle(name, argsJSON string) (string, string) {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return name, argsJSON
	}

	action := name
	detail := ""

	switch name {
	case "read_file":
		action = "Read"
		if path, ok := args["path"].(string); ok {
			detail = path
		}
	case "edit_file":
		action = "Edit"
		if path, ok := args["file_path"].(string); ok {
			detail = path
		}
	case "write_file":
		action = "Create"
		if path, ok := args["path"].(string); ok {
			detail = path
		}
	case "execute_command":
		action = "Run"
		if cmd, ok := args["command"].(string); ok {
			detail = cmd
		}
	case "list_directory":
		action = "ListDir"
		if path, ok := args["path"].(string); ok {
			detail = path
		}
	case "search_code":
		action = "Search"
		if pattern, ok := args["pattern"].(string); ok {
			detail = pattern
		}
	case "delete_file":
		action = "Delete"
		if path, ok := args["path"].(string); ok {
			detail = path
		}
	}

	if detail == "" {
		detail = argsJSON
	}
	return action, detail
}

func msgCacheKey(msg UIMessage, expanded bool) int {
	h := 0
	for _, c := range msg.Content {
		h = h*31 + int(c)
	}
	if expanded {
		h += 1000000
	}
	h += len(msg.Content) * 2000000
	return h
}

func updateViewportContent(m *Model) {
	if len(m.Messages) != m.lastMsgCount {
		m.RenderedLines = make(map[int][]string)
		m.lastMsgCount = len(m.Messages)
	}

	var historyView []string

	contentWidth := m.Width - 8
	if contentWidth < 20 {
		contentWidth = 20
	}

	wrapStyleUser := lipgloss.NewStyle().Width(contentWidth - 4)
	wrapStyleSystem := lipgloss.NewStyle().Width(contentWidth - 6)

	for idx, msg := range m.Messages {
		cacheKey := msgCacheKey(msg, m.ExpandedTools[idx])
		if cached, ok := m.RenderedLines[cacheKey]; ok {
			historyView = append(historyView, cached...)
			continue
		}
		var rendered []string

		switch msg.Role {
		case "user":
			content := strings.ReplaceAll(msg.Content, "\r", "")
			wrapped := wrapStyleUser.Render(content)
			lines := strings.Split(wrapped, "\n")
			for i, line := range lines {
				if i == 0 {
					rendered = append(rendered, "  "+lipgloss.NewStyle().Foreground(ColorClaudeOrange).Render("> ")+StyleUserMsg.Render(line))
				} else {
					rendered = append(rendered, "    "+StyleUserMsg.Render(line))
				}
			}
			rendered = append(rendered, "")
		case "assistant":
			content := strings.ReplaceAll(msg.Content, "\r", "")
			renderedMD := RenderMarkdown(content, contentWidth)
			lines := strings.Split(renderedMD, "\n")
			for _, line := range lines {
				rendered = append(rendered, "  "+StyleAgentMsg.Render(line))
			}
			rendered = append(rendered, "")
		case "tool_call":
			action, detail := formatToolCallAgyStyle(msg.Name, msg.Content)
			
			isLong := strings.Contains(detail, "\n") || len(detail) > 60
			
			label := ""
			displayDetail := detail
			if isLong {
				if m.ExpandedTools[idx] {
					label = " (ctrl+o to collapse)"
				} else {
					label = " (ctrl+o to expand)"
					firstLine := strings.Split(detail, "\n")[0]
					if len(firstLine) > 55 {
						firstLine = firstLine[:52] + "..."
					} else {
						firstLine = firstLine + "..."
					}
					displayDetail = firstLine
				}
			}
			
			line := fmt.Sprintf("  %s %s(%s)%s",
				lipgloss.NewStyle().Foreground(ColorPrimary).Render("●"),
				lipgloss.NewStyle().Foreground(ColorWarning).Bold(true).Render(action),
				lipgloss.NewStyle().Foreground(ColorWhite).Render(displayDetail),
				lipgloss.NewStyle().Foreground(ColorMuted).Render(label),
			)
			wrapped := lipgloss.NewStyle().Width(contentWidth).Render(line)
			rendered = append(rendered, wrapped)

			if idx == len(m.Messages)-1 && m.State == StateThinking {
				spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
				spinChar := spinner[m.ThinkingTicks%len(spinner)]

				statusText := "Running..."
				if action == "Read" {
					statusText = "Loading..."
				} else if action == "Write" || action == "Edit" {
					statusText = "Writing..."
				} else if action == "Search" || action == "Find" {
					statusText = "Searching..."
				}

				spinnerLine := fmt.Sprintf("    %s  %s",
					lipgloss.NewStyle().Foreground(ColorWarning).Render(spinChar),
					StyleThought.Render(statusText),
				)
				rendered = append(rendered, spinnerLine)

				if m.LastTaskOutput != "" {
					displayOut := m.LastTaskOutput
					if len(displayOut) > contentWidth-8 {
						displayOut = displayOut[:contentWidth-11] + "..."
					}
					outLine := fmt.Sprintf("       %s", lipgloss.NewStyle().Foreground(ColorMuted).Italic(true).Render(displayOut))
					rendered = append(rendered, outLine)
				}
			}

			hasNextToolCall := false
			for j := idx + 1; j < len(m.Messages); j++ {
				next := m.Messages[j]
				if next.Role == "tool_call" {
					hasNextToolCall = true
					break
				}
				if next.Role == "tool_result" && next.Success && !strings.HasPrefix(next.Content, "[Error]") {
					continue
				}
				break
			}
			if !hasNextToolCall {
				rendered = append(rendered, "")
			}
		case "tool_result":
			if !msg.Success || strings.HasPrefix(msg.Content, "[Error]") {
				wrapped := wrapStyleSystem.Render("✗ Tool failed: " + msg.Content)
				lines := strings.Split(wrapped, "\n")
				for i, line := range lines {
					if i == 0 {
						rendered = append(rendered, "  "+StyleToolError.Render(line))
					} else {
						rendered = append(rendered, "      "+StyleToolError.Render(line))
					}
				}
				rendered = append(rendered, "")
			}
		case "system":
			content := strings.ReplaceAll(msg.Content, "\r", "")
			wrapped := wrapStyleSystem.Render(content)
			lines := strings.Split(wrapped, "\n")
			for i, line := range lines {
				if i == 0 {
					rendered = append(rendered, "  "+StyleSystemMsg.Render("\u2139\ufe0f  "+line))
				} else {
					rendered = append(rendered, "      "+StyleSystemMsg.Render(line))
				}
			}
			rendered = append(rendered, "")
		}

		if rendered != nil {
			m.RenderedLines[cacheKey] = rendered
		}
		historyView = append(historyView, rendered...)
	}
	if m.ActivePlanGoal != "" && len(m.ActivePlanSteps) > 0 {
		var planLines []string
		planLines = append(planLines, "")
		planLines = append(planLines, "  "+lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("⎔ Planned Steps for: ")+lipgloss.NewStyle().Foreground(ColorWhite).Bold(true).Render(m.ActivePlanGoal))

		for idx, rawStep := range m.ActivePlanSteps {
			parts := strings.SplitN(rawStep, "|", 2)
			if len(parts) != 2 {
				continue
			}
			stepID := parts[0]
			stepDesc := parts[1]

			status := m.ActivePlanStepStatuses[stepID]
			icon := "⧗"
			iconColor := ColorMuted

			switch status {
			case "completed":
				icon = "✓"
				iconColor = ColorSuccess
			case "failed":
				icon = "✗"
				iconColor = ColorError
			case "running":
				icon = "●"
				iconColor = ColorWarning
			}

			descColor := ColorWhite
			if status == "completed" {
				descColor = ColorMuted
			}

			stepLine := fmt.Sprintf("    %s %d. %s",
				lipgloss.NewStyle().Foreground(iconColor).Bold(true).Render(icon),
				idx+1,
				lipgloss.NewStyle().Foreground(descColor).Render(stepDesc),
			)
			planLines = append(planLines, stepLine)
		}
		planLines = append(planLines, "")
		historyView = append(historyView, planLines...)
	}

	m.Viewport.SetContent(strings.Join(historyView, "\n"))
	m.Viewport.SetXOffset(0) 
	if !m.UserScrolledUp {
		m.Viewport.GotoBottom()
	}
	saveModelSession(m) 
}

func generateSessionTitle(m *Model) {
	if len(m.Messages) < 2 {
		return
	}

	isDefault := m.ActiveSessionTitle == "New Conversation" ||
		strings.HasPrefix(m.ActiveSessionTitle, "session-") ||
		len(m.ActiveSessionTitle) > 30

	if !isDefault {
		return
	}

	var history []client.Message
	count := 0
	for _, msg := range m.Messages {
		if msg.Role == "user" || msg.Role == "assistant" {
			history = append(history, client.Message{
				Role:    msg.Role,
				Content: msg.Content,
			})
			count++
			if count >= 4 { 
				break
			}
		}
	}

	history = append(history, client.Message{
		Role:    "user",
		Content: "Summarize this conversation into a short title of at most 4 words. Respond ONLY with the title. Do not include quotes, punctuation, or explanations.",
	})

	go func(sessionID string, cfg *config.Config) {
		cli := client.New(cfg)
		choice, _, err := cli.Send(context.Background(), history, nil)
		if err == nil && choice != nil {
			title := strings.TrimSpace(choice.Message.Content)
			title = strings.Trim(title, "\"`'.-")
			if title != "" && len(title) < 50 {
				sess, loadErr := LoadSession(sessionID)
				if loadErr == nil {
					sess.Title = title
					RenameSession(sessionID, title)
				}
			}
		}
	}(m.ActiveSessionID, m.Cfg)
}

func (m *Model) getApprovalOptions() []string {
	if m.PendingTool == "execute_command" {
		var args map[string]any
		json.Unmarshal([]byte(m.PendingArgs), &args)
		cmd, _ := args["command"].(string)
		fields := strings.Fields(cmd)
		prefix := ""
		if len(fields) > 0 {
			prefix = fields[0]
		}
		if prefix != "" {
			return []string{
				"Yes, execute once",
				fmt.Sprintf("Yes, and always allow in this session for commands starting with '%s'", prefix),
				"Yes, and always allow all 'execute_command' tools (this session)",
				"No, reject",
			}
		}
	}
	return []string{
		"Yes, execute once",
		fmt.Sprintf("Yes, and always allow '%s' (this session)", m.PendingTool),
		"No, reject",
	}
}

func (m *Model) getApprovalOptionsCount() int {
	return len(m.getApprovalOptions())
}
