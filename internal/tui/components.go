package tui

import (
	"awas/internal/agent"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func formatCommas(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var res []string
	for len(s) > 3 {
		res = append([]string{s[len(s)-3:]}, res...)
		s = s[:len(s)-3]
	}
	if len(s) > 0 {
		res = append([]string{s}, res...)
	}
	return strings.Join(res, ",")
}

func formatRelativeTime(t time.Time) string {
	diff := time.Since(t)
	if diff < time.Minute {
		return "just now"
	}
	if diff < time.Hour {
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	}
	if diff < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	}
	if diff < 48*time.Hour {
		return "1d ago"
	}
	return t.Format("Jan _2")
}

func RenderHeader(modelName, mode string, tokens, maxTokens int, workdir string, width int, compressedTurns int, streamEnabled bool, latestVersionAvailable string) string {
	home, err := os.UserHomeDir()
	displayWorkdir := workdir
	if err == nil && strings.HasPrefix(workdir, home) {
		displayWorkdir = "~" + strings.TrimPrefix(workdir, home)
	}

	contentWidth := width - 2
	if contentWidth < 20 {
		contentWidth = 20
	}

	modeStyle := StyleModeSafe
	if mode == "autonomous" {
		modeStyle = StyleModeAuto
	}
	modeLabel := modeStyle.Render(" " + strings.ToUpper(mode) + " ")

	asciiL1 := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(" ▄▀█ █ █ █ ▄▀█ █▀▀ ")
	asciiL2 := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(" █▀█ █▄█▄█ █▀█ ▄██ ")

	// Line 1
	leftPart1 := asciiL1 + "  Model: " + StyleModelLabel.Render(modelName)
	rightPart1 := "Mode: " + modeLabel + "  "
	leftLen1 := lipgloss.Width(leftPart1)
	rightLen1 := lipgloss.Width(rightPart1)
	padLen1 := contentWidth - leftLen1 - rightLen1
	if padLen1 < 0 {
		padLen1 = 0
	}
	row1 := leftPart1 + strings.Repeat(" ", padLen1) + rightPart1
	row1Width := lipgloss.Width(row1)
	if row1Width < contentWidth {
		row1 += strings.Repeat(" ", contentWidth-row1Width)
	}

	leftPart2 := asciiL2 + "  " + RenderTokenBar(tokens, maxTokens)
	rightPart2 := "  Workdir: " + lipgloss.NewStyle().Foreground(ColorPrimary).Render(displayWorkdir)
	leftLen2 := lipgloss.Width(leftPart2)
	rightLen2 := lipgloss.Width(rightPart2)
	padLen2 := contentWidth - leftLen2 - rightLen2
	if padLen2 < 0 {
		padLen2 = 0
	}
	row2 := leftPart2 + strings.Repeat(" ", padLen2) + rightPart2
	row2Width := lipgloss.Width(row2)
	if row2Width < contentWidth {
		row2 += strings.Repeat(" ", contentWidth-row2Width)
	}

	borderStyle := lipgloss.NewStyle().Foreground(ColorPrimary)

	topBorder := fmt.Sprintf("┌─ AWAS v%s ", Version)
	subagents := agent.GetSubagentRegistry().List()
	activeCount := 0
	activeRole := ""
	activeStep := ""
	for _, s := range subagents {
		if s.Status == agent.SubagentStatusRunning {
			activeCount++
			activeRole = s.Role
			activeStep = s.CurrentStep
		}
	}
	if activeCount > 0 {
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		frame := frames[(time.Now().UnixNano()/100000000)%int64(len(frames))]
		if activeCount == 1 {
			if activeStep != "" {
				topBorder = fmt.Sprintf("┌─ AWAS v%s [%s %s: %s] ", Version, frame, activeRole, activeStep)
			} else {
				topBorder = fmt.Sprintf("┌─ AWAS v%s [%s Subagent: %s] ", Version, frame, activeRole)
			}
		} else {
			topBorder = fmt.Sprintf("┌─ AWAS v%s [%s %d Subagents Running] ", Version, frame, activeCount)
		}
	} else if latestVersionAvailable != "" {
		topBorder = fmt.Sprintf("┌─ AWAS v%s [Update: v%s available — npm i -g awas-agent] ", Version, latestVersionAvailable)
	}
	topBorderLen := lipgloss.Width(topBorder)
	padTop := width - topBorderLen - 1
	if padTop < 0 {
		padTop = 0
	}
	topLine := topBorder + strings.Repeat("─", padTop) + "┐"
	midLine1 := "│" + row1 + "│"
	midLine2 := "│" + row2 + "│"
	bottomLine := "└" + strings.Repeat("─", contentWidth) + "┘"

	return strings.Join([]string{
		borderStyle.Render(topLine),
		midLine1,
		midLine2,
		borderStyle.Render(bottomLine),
	}, "\n")
}

func RenderTokenBar(current, max int) string {
	if max <= 0 {
		max = 1
	}
	pct := float64(current) / float64(max) * 100
	if pct > 100 {
		pct = 100
	}
	barWidth := 10
	filled := int(pct / 100 * float64(barWidth))
	empty := barWidth - filled

	var barColor lipgloss.Color
	switch {
	case pct > 85:
		barColor = ColorError // red
	case pct > 65:
		barColor = ColorWarning // orange
	case pct > 40:
		barColor = lipgloss.Color("#FFD700") // yellow
	default:
		barColor = ColorSuccess // green
	}

	filledBar := lipgloss.NewStyle().Foreground(barColor).Render(strings.Repeat("█", filled))
	emptyBar := lipgloss.NewStyle().Foreground(ColorDarkMuted).Render(strings.Repeat("░", empty))

	return fmt.Sprintf("Context: %s%s %.1f%% (%s)",
		filledBar, emptyBar,
		pct,
		formatCompact(current),
	)
}

func formatCompact(n int) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func RenderToolCall(name, args string) string {
	return StyleToolCall.Render(fmt.Sprintf("→ %s(%s)", name, args))
}

func RenderToolResult(name, result string, success bool) string {
	icon := "✓"
	style := StyleToolSuccess
	if !success || strings.HasPrefix(result, "[Error]") {
		icon = "✗"
		style = StyleToolError
	}
	displayRes := result
	if len(displayRes) > 200 {
		displayRes = displayRes[:200] + "... (truncated)"
	}
	displayRes = strings.ReplaceAll(displayRes, "\n", " ")

	return style.Render(fmt.Sprintf("  %s %s: %s", icon, name, displayRes))
}

func RenderStatusBar(shortcuts []string, modelName string, width int, activeTasks int, streamEnabled bool, agentMode string) string {
	var badges []string
	if agentMode != "" {
		badges = append(badges, lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(fmt.Sprintf("[%s]", agentMode)))
	}
	if streamEnabled {
		badges = append(badges, lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true).Render("[SSE]"))
	}
	if activeTasks > 0 {
		badges = append(badges, lipgloss.NewStyle().Foreground(ColorWarning).Bold(true).Render(fmt.Sprintf("⚡%d", activeTasks)))
	}
	right := ""
	if len(badges) > 0 {
		right = "  " + strings.Join(badges, " ")
	}

	left := "  " + strings.Join(shortcuts, "    ")

	availWidth := width - 2
	padding := availWidth - lipgloss.Width(left) - lipgloss.Width(right)
	if padding < 0 {
		padding = 0
	}

	return StyleStatusBar.Render(left + strings.Repeat(" ", padding) + right)
}

type CommandInfo struct {
	Command     string
	Description string
	Usage       string
}

var AvailableCommands = []CommandInfo{
	{"/help", "Show available shortcuts and commands", "/help"},
	{"/history", "Show past conversation history manager", "/history"},
	{"/resume", "Resume a past conversation session", "/resume [index|session_id]"},
	{"/clear", "Clear TUI messages but keep active session", "/clear"},
	{"/reset", "Reset agent loop history and clear TUI messages", "/reset"},
	{"/mode", "Switch mode (safe/auto/chat/simple/planned/deep)", "/mode [safe|autonomous|chat|simple|planned|deep]"},
	{"/stream", "Toggle SSE streaming on/off", "/stream [on|off]"},
	{"/model", "Switch active model on the fly", "/model [model_name]"},
	{"/switch", "Switch between configured provider profiles", "/switch [profile_name]"},
	{"/setup", "Rerun Setup Wizard to configure endpoints", "/setup"},
	{"/logout", "Clear OAuth and provider configurations", "/logout"},
	{"/tokens", "Show detailed token usage breakdown", "/tokens"},
	{"/limit", "Set or view consecutive tool loop limit", "/limit [number|off]"},
	{"/skills", "Manage local skills and template creation", "/skills [create [name] | add [repo] | list]"},
	{"/tasks", "Show active session background task manager", "/tasks"},
	{"/undo", "Undo last file operations in workspace", "/undo [steps]"},
	{"/redo", "Redo last undone operations in workspace", "/redo [steps]"},
	{"/undo-history", "Show workspace modification history list", "/undo-history [clear]"},
	{"/indexing", "Build code index for search_symbols tool", "/indexing"},
	{"/gateway", "Manage chat platform gateways (Telegram, Discord)", "/gateway [start|stop|status|users|setup]"},
	{"/exit", "Exit the application", "/exit"},
}

func RenderSuggestionMenu(filtered []string, cursor int, width int) string {
	if len(filtered) == 0 {
		return ""
	}

	var items []string
	title := fmt.Sprintf("\U0001f50e Suggestions (%d/%d)", cursor+1, len(filtered))
	items = append(items, StyleModelLabel.Render(title))

	maxVisible := 4
	start := 0
	if cursor >= maxVisible {
		start = cursor - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(filtered) {
		end = len(filtered)
	}
	if end-start < maxVisible && len(filtered) >= maxVisible {
		start = end - maxVisible
	}

	for idx := start; idx < end; idx++ {
		cmd := filtered[idx]
		desc := ""
		for _, info := range AvailableCommands {
			if info.Command == cmd {
				desc = info.Description
				break
			}
		}

		line := fmt.Sprintf("  %s - %s", cmd, desc)
		if idx == cursor {
			selectedStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#1a1a1a")).
				Background(ColorPrimary).
				Bold(true)
			items = append(items, selectedStyle.Render(line))
		} else {
			items = append(items, lipgloss.NewStyle().Foreground(ColorWhite).Render(line))
		}
	}

	menuBox := StyleMenuBox.
		Width(width - 6).
		Render(strings.Join(items, "\n"))

	return menuBox
}

func RenderFileSuggestionMenu(filtered []string, cursor int, width int) string {
	if len(filtered) == 0 {
		return ""
	}

	var items []string
	title := fmt.Sprintf("\U0001f4c1 Workspace Files (%d/%d)", cursor+1, len(filtered))
	items = append(items, StyleModelLabel.Render(title))

	maxVisible := 4
	start := 0
	if cursor >= maxVisible {
		start = cursor - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(filtered) {
		end = len(filtered)
	}
	if end-start < maxVisible && len(filtered) >= maxVisible {
		start = end - maxVisible
	}

	for idx := start; idx < end; idx++ {
		f := filtered[idx]
		line := fmt.Sprintf("  @%s", f)
		if idx == cursor {
			selectedStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#1a1a1a")).
				Background(ColorPrimary).
				Bold(true)
			items = append(items, selectedStyle.Render(line))
		} else {
			items = append(items, lipgloss.NewStyle().Foreground(ColorWhite).Render(line))
		}
	}

	menuBox := StyleMenuBox.
		Width(width - 6).
		Render(strings.Join(items, "\n"))

	return menuBox
}
