package tui

import (
	"awas/internal/provider"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() tea.View {
	var sections []string

	width := m.Width
	if width <= 0 {
		width = 80
	}

	if m.State == StateSetupWizard {
		content := m.WizardModel.View()
		v := tea.NewView(content)
		v.AltScreen = true
		return v
	}

	sections = append(sections, RenderHeader(m.Cfg.Model, m.Cfg.Mode, m.TokenCount, m.TokenMax, m.Cfg.WorkDir, width, m.CompressedTurns, m.Cfg.Stream, m.LatestVersionAvailable))
	sections = append(sections, "")

	if m.State == StateProfileSwitch {
		sections = append(sections, "  "+lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Underline(true).Render("Profile Switcher"))
		sections = append(sections, "  Select an LLM Provider Profile:")
		sections = append(sections, "")

		for i, name := range m.SwitchProfiles {
			cursor := "  "
			if i == m.SwitchCursor {
				cursor = "> "
			}

			mgr := provider.NewManager("")
			isActive := name == mgr.ActiveProfile

			line := fmt.Sprintf("%s %s", cursor, name)
			if isActive {
				line += " " + lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF66")).Render("(active)")
			}

			if i == m.SwitchCursor {
				line = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(line)
			}
			sections = append(sections, "  "+line)
		}
		sections = append(sections, "")
		sections = append(sections, "  "+lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render("Use ↑/↓ to select · enter to confirm · esc to cancel"))
	} else if m.State == StateSkillsMenu {
		sections = append(sections, "  "+lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Underline(true).Render("Skills Manager"))
		sections = append(sections, "  Toggle active/inactive skills:")
		sections = append(sections, "")

		if len(m.Skills) == 0 {
			sections = append(sections, "  (No skills found in ~/.awas/skills/)")
		} else {
			start := m.SkillsPage * 10
			end := start + 10
			if end > len(m.Skills) {
				end = len(m.Skills)
			}

			for i := start; i < end; i++ {
				s := m.Skills[i]
				cursor := "  "
				if i == m.SkillsCursor {
					cursor = "> "
				}

				checkbox := "[ ]"
				label := strings.TrimSuffix(s.Name, ".disabled")
				if s.Active {
					checkbox = "[x]"
				}

				status := ""
				if s.Active {
					status = " " + lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF66")).Render("(active)")
				} else {
					status = " " + lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8800")).Render("(disabled)")
				}

				line := fmt.Sprintf("%s %s %s%s", cursor, checkbox, label, status)
				if i == m.SkillsCursor {
					line = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(line)
				}
				sections = append(sections, "  "+line)
			}

			if len(m.Skills) > 0 {
				sections = append(sections, "")
				sections = append(sections, fmt.Sprintf("  [%d-%d of %d items]", start+1, end, len(m.Skills)))
			}
		}
		sections = append(sections, "")
		sections = append(sections, "  "+lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render("Use ↑/↓ navigate · ←/→ page · space/enter toggle · esc/q exit"))
	} else if m.State == StateHistoryView {
		sections = append(sections, "  "+StyleModelLabel.Copy().Underline(true).Render("Conversations"))
		sections = append(sections, "  Type to search...")
		sections = append(sections, "  "+m.HistorySearchInput.View())
		sections = append(sections, "")

		start := m.HistoryPage * 10
		end := start + 10
		if end > len(m.FilteredSessions) {
			end = len(m.FilteredSessions)
		}

		for idx := start; idx < end; idx++ {
			s := m.FilteredSessions[idx]
			prefix := "  "
			if idx == m.HistoryCursor {
				prefix = "> "
			}

			stepsStr := fmt.Sprintf("%d steps", s.Steps)
			timeStr := formatRelativeTime(s.UpdatedAt)

			displayTitle := s.Title
			maxTitleLen := width - 35
			if maxTitleLen < 15 {
				maxTitleLen = 15
			}
			if lipgloss.Width(displayTitle) > maxTitleLen {
				displayTitle = displayTitle[:maxTitleLen-3] + "..."
			}

			isCurrent := s.ID == m.ActiveSessionID
			titleText := displayTitle
			if isCurrent {
				titleText = "[CURRENT] " + displayTitle
			}

			line := fmt.Sprintf("%s%s", prefix, titleText)
			rightPart := fmt.Sprintf("%10s    %8s", stepsStr, timeStr)
			pad := width - lipgloss.Width(line) - lipgloss.Width(rightPart) - 4
			if pad < 0 {
				pad = 0
			}

			fullLine := line + strings.Repeat(" ", pad) + rightPart
			if idx == m.HistoryCursor {
				fullLine = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(fullLine)
			} else if isCurrent {
				fullLine = lipgloss.NewStyle().Foreground(ColorSuccess).Render(fullLine)
			} else {
				fullLine = lipgloss.NewStyle().Foreground(ColorWhite).Render(fullLine)
			}
			sections = append(sections, fullLine)
		}

		if len(m.FilteredSessions) > 0 {
			sections = append(sections, "")
			sections = append(sections, fmt.Sprintf("  [%d-%d of %d items]", start+1, end, len(m.FilteredSessions)))
		} else {
			sections = append(sections, "  (No sessions found)")
		}

		if m.HistoryRenameMode {
			sections = append(sections, "")
			sections = append(sections, "  "+StyleModelLabel.Render("\U0001f4dd Rename Conversation"))
			sections = append(sections, "  "+m.HistoryRenameInput.View())
		}

		sections = append(sections, "")
		if time.Since(m.LastCtrlCTime) < 2*time.Second {
			sections = append(sections, "  "+StyleToolError.Copy().Bold(true).Render("\u26a0\ufe0f  Press ctrl+c again to exit"))
		} else {
			sections = append(sections, "  "+StyleMuted.Render("Keyboard: ↑/↓ Navigate  │  ←/→ Page  │  enter Select  │  r Rename  │  ctrl+d Delete"))
			sections = append(sections, "  "+StyleMuted.Render("esc Go back / Clear search"))
		}
	} else if m.State == StateTasksView {
		sections = append(sections, "  "+StyleModelLabel.Copy().Underline(true).Render("Tasks"))
		sections = append(sections, "  "+StyleMuted.Render("Agent Backgrounded"))
		sections = append(sections, "")

		if len(m.Tasks) == 0 {
			sections = append(sections, "  (No background tasks in this session)")
		} else {
			for i, t := range m.Tasks {
				timeStr := t.StartTime.Format("15:04:05")

				cmdStr := t.Command
				maxCmdLen := width - 35
				if maxCmdLen > 10 {
					if len(cmdStr) > maxCmdLen {
						cmdStr = cmdStr[:maxCmdLen-3] + "..."
					}
				}

				statusStyle := StyleMuted
				if t.Status == "running" {
					statusStyle = StyleLogo.Copy().Bold(false) // Cyan
				} else if strings.HasPrefix(t.Status, "completed") {
					statusStyle = StyleToolSuccess.Copy() // Green
				} else if strings.HasPrefix(t.Status, "failed") || t.Status == "cancelled" || t.Status == "timed out" {
					statusStyle = StyleToolError.Copy() // Red
				}

				lineContent := fmt.Sprintf("  ● [%s] %s", timeStr, cmdStr)
				statusStr := statusStyle.Render(t.Status)

				padding := width - lipgloss.Width(lineContent) - lipgloss.Width(statusStr) - 4
				if padding > 0 {
					lineContent += strings.Repeat(" ", padding)
				}

				line := lineContent + statusStr
				if i == m.TaskCursor {
					sections = append(sections, StyleSuggestionActive.Render("  > "+strings.TrimPrefix(line, "  ")))
				} else {
					sections = append(sections, line)
				}
			}
		}

		sections = append(sections, "")
		if time.Since(m.LastCtrlCTime) < 2*time.Second {
			sections = append(sections, "  "+StyleToolError.Copy().Bold(true).Render("\u26a0\ufe0f  Press ctrl+c again to exit"))
		} else {
			sections = append(sections, "  "+StyleMuted.Render("Keyboard: ↑/↓ Navigate  │  k Kill Task  │  x Remove Task"))
			sections = append(sections, "  "+StyleMuted.Render("esc Return to Chat"))
		}
	} else {

		sections = append(sections, m.Viewport.View())
		sections = append(sections, "")

		if m.State == StateThinking {
			spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
			spinChar := spinner[m.ThinkingTicks%len(spinner)]
			secs := float64(m.ThinkingTicks) * 0.05
			thoughtText := fmt.Sprintf("* Thought for %.1fs (model: %s)", secs, m.ThinkingModel)
			sections = append(sections, "  "+StyleThought.Render(fmt.Sprintf("%s %s", spinChar, thoughtText)))
			sections = append(sections, "")
		}

		switch m.State {
		case StateApprovalPending:
			var lines []string
			lines = append(lines, "  ⛭  "+StyleModelLabel.Render(fmt.Sprintf("Requesting permission to run: %s", m.PendingTool)))
			
			if m.PendingTool == "execute_command" {
				var args map[string]any
				json.Unmarshal([]byte(m.PendingArgs), &args)
				cmd, _ := args["command"].(string)
				
				cmdLinesList := strings.Split(cmd, "\n")
				displayCmd := cmd
				if len(cmdLinesList) > 5 {
					displayCmd = strings.Join(cmdLinesList[:5], "\n") + "\n    ... (truncated)"
				}
				
				lines = append(lines, "  Command to run:")
				lines = append(lines, "    "+StyleThought.Render(displayCmd))
			} else {
				lines = append(lines, "  Arguments:")
				lines = append(lines, "    "+StyleMuted.Render(m.PendingArgs))
			}
			
			lines = append(lines, "")
			lines = append(lines, "  Do you want to proceed?")
			
			options := m.getApprovalOptions()
			for i, opt := range options {
				if i == m.ApprovalCursor {
					lines = append(lines, "  "+StyleSelectionCursor.Render("> ")+StyleSuggestionActive.Render(opt))
				} else {
					lines = append(lines, "    "+StyleMuted.Render(opt))
				}
			}
			lines = append(lines, "")
			lines = append(lines, "  "+StyleMuted.Copy().Faint(true).Render("↑/↓ Navigate · enter Confirm · y/n Shortcut"))
			
			sections = append(sections, strings.Join(lines, "\n"))
		case StateChainLimitPending:
			prompt := StylePendingPrompt.Render(fmt.Sprintf("  ⚠  Reached limit of %d consecutive tool calls. Continue execution? [y/n]: ", m.Cfg.MaxChainLimit))
			sections = append(sections, prompt)
		case StateIdle, StateThinking, StateAskUserPending:
			if m.ShowSuggestionMenu && len(m.FilteredSuggestions) > 0 {
				sections = append(sections, RenderSuggestionMenu(m.FilteredSuggestions, m.SuggestionCursor, width))
			}
			if m.ShowFileSuggestion && len(m.FilteredFileSuggestions) > 0 {
				sections = append(sections, RenderFileSuggestionMenu(m.FilteredFileSuggestions, m.FileSuggestionCursor, width))
			}
			promptPrefix := lipgloss.NewStyle().Foreground(ColorPrimary).Render("> ")
			inputLines := strings.Split(m.Input.View(), "\n")
			for i, line := range inputLines {
				cleanLine := strings.ReplaceAll(line, "\r", "")
				if i == 0 {
					sections = append(sections, "  "+promptPrefix+cleanLine)
				} else {
					sections = append(sections, "    "+cleanLine)
				}
			}
			if time.Since(m.LastCtrlCTime) < 2*time.Second {
				sections = append(sections, "  "+StyleToolError.Copy().Bold(true).Render("\u26a0\ufe0f  Press ctrl+c again to exit"))
			}
		}

		var shortcuts []string
		switch m.State {
		case StateThinking:
			shortcuts = []string{
				fmt.Sprintf("%s: interrupt", StyleShortcut.Render("esc")),
				fmt.Sprintf("%s: exit", StyleShortcut.Render("ctrl+c")),
			}
		case StateAskUserPending:
			shortcuts = []string{
				fmt.Sprintf("%s: reply", StyleShortcut.Render("enter")),
				fmt.Sprintf("%s: exit", StyleShortcut.Render("ctrl+c")),
			}
		case StateApprovalPending:
			shortcuts = []string{
				fmt.Sprintf("%s: navigate", StyleShortcut.Render("↑/↓")),
				fmt.Sprintf("%s: confirm", StyleShortcut.Render("enter")),
				fmt.Sprintf("%s: shortcut", StyleShortcut.Render("y/n")),
				fmt.Sprintf("%s: exit", StyleShortcut.Render("ctrl+c")),
			}
		case StateChainLimitPending:
			shortcuts = []string{
				fmt.Sprintf("%s: continue", StyleShortcut.Render("y/n")),
				fmt.Sprintf("%s: exit", StyleShortcut.Render("ctrl+c")),
			}
		default:
			shortcuts = []string{
				fmt.Sprintf("%s: history", StyleShortcut.Render("↑/↓")),
				fmt.Sprintf("%s: mode", StyleShortcut.Render("tab")),
				fmt.Sprintf("%s: exit", StyleShortcut.Render("ctrl+c")),
			}
		}

		sections = append(sections, "", RenderStatusBar(shortcuts, m.Cfg.Model, width, m.ActiveTaskCount, m.Cfg.Stream, m.Cfg.AgentMode))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, sections...)
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}
