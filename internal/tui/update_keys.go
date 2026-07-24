package tui

import (
	"awas/internal/agent"
	"awas/internal/client"
	"awas/internal/provider"
	"awas/internal/tools"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

func handleKeyPress(m *Model, msg tea.KeyPressMsg) (tea.Cmd, bool) {
	var cmd tea.Cmd

	if msg.String() == "ctrl+c" {
		if time.Since(m.LastCtrlCTime) < 2*time.Second {
			if m.ApprovalChan != nil {
				select {
				case m.ApprovalChan <- false:
				default:
				}
			}
			return tea.Quit, true
		}
		m.LastCtrlCTime = time.Now()
		recalculateViewportHeight(m)
		return nil, true
	}

	if m.State == StateSetupWizard {
		var wzCmd tea.Cmd
		m.WizardModel, wzCmd = m.WizardModel.Update(msg)
		return wzCmd, true
	}

	if m.State == StateProfileSwitch {
		switch msg.String() {
		case "up", "k":
			m.SwitchCursor--
			if m.SwitchCursor < 0 {
				m.SwitchCursor = len(m.SwitchProfiles) - 1
			}
			return nil, true
		case "down", "j":
			m.SwitchCursor++
			if m.SwitchCursor >= len(m.SwitchProfiles) {
				m.SwitchCursor = 0
			}
			return nil, true
		case "esc":
			m.State = StateIdle
			recalculateViewportHeight(m)
			updateViewportContent(m)
			return nil, true
		case "enter":
			selectedName := m.SwitchProfiles[m.SwitchCursor]
			mgr := provider.NewManager("")
			p, ok := mgr.Profiles[selectedName]
			if ok {
				mgr.ActiveProfile = selectedName
				mgr.Save()

				m.Cfg.Endpoint = p.GetEndpoint()
				m.Cfg.APIKey = p.GetAPIKey()
				m.Cfg.Model = p.GetModel()
				m.Cfg.Save()

				if m.Loop != nil {
					m.Loop.SetClient(client.New(p))
				}

				m.Messages = append(m.Messages, UIMessage{
					Role:    "system",
					Content: fmt.Sprintf("Switched to provider profile: %s (Model: %s)", selectedName, p.Model),
				})
			}
			m.State = StateIdle
			recalculateViewportHeight(m)
			updateViewportContent(m)
			return nil, true
		}
		return nil, true
	}

	if m.State == StateSkillsMenu {
		switch msg.String() {
		case "up", "k":
			m.SkillsCursor--
			if m.SkillsCursor < 0 {
				m.SkillsCursor = len(m.Skills) - 1
			}
			if m.SkillsCursor < m.SkillsPage*10 {
				m.SkillsPage = m.SkillsCursor / 10
			}
			if m.SkillsCursor >= (m.SkillsPage+1)*10 {
				m.SkillsPage = m.SkillsCursor / 10
			}
			return nil, true
		case "down", "j":
			m.SkillsCursor++
			if m.SkillsCursor >= len(m.Skills) {
				m.SkillsCursor = 0
			}
			// Adjust page if cursor moves out of view
			if m.SkillsCursor < m.SkillsPage*10 {
				m.SkillsPage = m.SkillsCursor / 10
			}
			if m.SkillsCursor >= (m.SkillsPage+1)*10 {
				m.SkillsPage = m.SkillsCursor / 10
			}
			return nil, true
		case "left":
			m.SkillsPage--
			if m.SkillsPage < 0 {
				m.SkillsPage = 0
			}
			if m.SkillsCursor < m.SkillsPage*10 {
				m.SkillsCursor = m.SkillsPage * 10
			}
			if m.SkillsCursor >= len(m.Skills) {
				m.SkillsCursor = len(m.Skills) - 1
			}
			return nil, true
		case "right":
			maxPage := (len(m.Skills) - 1) / 10
			if maxPage < 0 {
				maxPage = 0
			}
			m.SkillsPage++
			if m.SkillsPage > maxPage {
				m.SkillsPage = maxPage
			}
			if m.SkillsCursor < m.SkillsPage*10 {
				m.SkillsCursor = m.SkillsPage * 10
			}
			if m.SkillsCursor >= len(m.Skills) {
				m.SkillsCursor = len(m.Skills) - 1
			}
			return nil, true
		case "esc", "q":
			m.State = StateIdle
			recalculateViewportHeight(m)
			updateViewportContent(m)
			return nil, true
		case " ", "enter":
			if len(m.Skills) > 0 {
				s := &m.Skills[m.SkillsCursor]
				var newPath string
				var newName string
				if s.Active {
					newName = s.Name + ".disabled"
					newPath = s.Path + ".disabled"
				} else {
					newName = strings.TrimSuffix(s.Name, ".disabled")
					newPath = strings.TrimSuffix(s.Path, ".disabled")
				}
				err := os.Rename(s.Path, newPath)
				if err == nil {
					s.Name = newName
					s.Path = newPath
					s.Active = !s.Active
					agent.InvalidateSkillsCache()
				} else {
					m.Messages = append(m.Messages, UIMessage{
						Role:    "system",
						Content: fmt.Sprintf("[Error] Failed to toggle skill: %v", err),
					})
					updateViewportContent(m)
				}
			}
			return nil, true
		}
		return nil, true
	}

	if m.State == StateHistoryView {
		switch msg.String() {
		case "ctrl+c":
			return tea.Quit, true
		case "esc":
			if m.HistoryRenameMode {
				m.HistoryRenameMode = false
				return nil, true
			}
			if m.HistorySearchInput.Value() != "" {
				m.HistorySearchInput.SetValue("")
				m.FilteredSessions = m.Sessions
				m.HistoryCursor = 0
				m.HistoryPage = 0
				return nil, true
			}
			m.State = m.PreviousState
			return nil, true
		case "up":
			if !m.HistoryRenameMode && len(m.FilteredSessions) > 0 {
				m.HistoryCursor--
				if m.HistoryCursor < 0 {
					m.HistoryCursor = len(m.FilteredSessions) - 1
				}
			}
			return nil, true
		case "down":
			if !m.HistoryRenameMode && len(m.FilteredSessions) > 0 {
				m.HistoryCursor++
				if m.HistoryCursor >= len(m.FilteredSessions) {
					m.HistoryCursor = 0
				}
			}
			return nil, true
		case "left":
			if !m.HistoryRenameMode {
				m.HistoryPage--
				if m.HistoryPage < 0 {
					m.HistoryPage = 0
				}
			}
			return nil, true
		case "right":
			if !m.HistoryRenameMode {
				maxPage := (len(m.FilteredSessions) - 1) / 10
				if maxPage < 0 {
					maxPage = 0
				}
				m.HistoryPage++
				if m.HistoryPage > maxPage {
					m.HistoryPage = maxPage
				}
			}
			return nil, true
		case "r":
			if len(m.FilteredSessions) > 0 && !m.HistoryRenameMode {
				selected := m.FilteredSessions[m.HistoryCursor]
				m.HistoryRenameMode = true
				m.HistoryRenameInput.SetValue(selected.Title)
				m.HistoryRenameInput.Focus()
			}
			return nil, true
		case "ctrl+delete", "ctrl+d":
			if len(m.FilteredSessions) > 0 && !m.HistoryRenameMode {
				selected := m.FilteredSessions[m.HistoryCursor]
				DeleteSession(selected.ID)
				list, _ := ListSessions()
				m.Sessions = list
				m.FilteredSessions = list
				if m.HistoryCursor >= len(m.FilteredSessions) {
					m.HistoryCursor = len(m.FilteredSessions) - 1
				}
				if m.HistoryCursor < 0 {
					m.HistoryCursor = 0
				}
			}
			return nil, true
		case "enter":
			if m.HistoryRenameMode {
				selected := m.FilteredSessions[m.HistoryCursor]
				newName := m.HistoryRenameInput.Value()
				if newName != "" {
					RenameSession(selected.ID, newName)
					list, _ := ListSessions()
					m.Sessions = list
					m.FilteredSessions = list
					m.HistoryRenameMode = false
				}
				return nil, true
			}

			if len(m.FilteredSessions) > 0 {
				selected := m.FilteredSessions[m.HistoryCursor]
				sess, err := LoadSession(selected.ID)
				if err == nil {
					m.ActiveSessionID = sess.ID
					m.ActiveSessionTitle = sess.Title
					m.ActiveSessionCreatedAt = sess.CreatedAt
					m.LastSavedSeq = len(sess.Messages) - 1
					m.Messages = sess.Messages
					m.TokenCount = sess.TokenCount
					m.CompressedTurns = sess.CompressedTurns
					m.Cfg.Model = sess.Model
					m.Cfg.Mode = sess.Mode
					if l := m.Loop; l != nil {
						l.SetHistory(sess.History)
					}
					tools.SetCurrentSessionID(sess.ID)
					updateViewportContent(m)
					recalculateViewportHeight(m)
					m.State = StateIdle
				}
			}
			return nil, true
		}

		if m.HistoryRenameMode {
			m.HistoryRenameInput, cmd = m.HistoryRenameInput.Update(msg)
			return cmd, true
		} else {
			oldSearch := m.HistorySearchInput.Value()
			m.HistorySearchInput, cmd = m.HistorySearchInput.Update(msg)
			newSearch := m.HistorySearchInput.Value()
			if newSearch != oldSearch {
				var filtered []SessionMeta
				for _, s := range m.Sessions {
					if strings.Contains(strings.ToLower(s.Title), strings.ToLower(newSearch)) ||
						strings.Contains(strings.ToLower(s.ID), strings.ToLower(newSearch)) {
						filtered = append(filtered, s)
					}
				}
				m.FilteredSessions = filtered
				m.HistoryCursor = 0
				m.HistoryPage = 0
			}
			return cmd, true
		}
	}

	if m.State == StateTasksView {
		switch msg.String() {
		case "ctrl+c":
			return tea.Quit, true
		case "esc":
			m.State = m.PreviousState
			return nil, true
		case "up":
			if len(m.Tasks) > 0 {
				m.TaskCursor--
				if m.TaskCursor < 0 {
					m.TaskCursor = len(m.Tasks) - 1
				}
			}
			return nil, true
		case "down":
			if len(m.Tasks) > 0 {
				m.TaskCursor++
				if m.TaskCursor >= len(m.Tasks) {
					m.TaskCursor = 0
				}
			}
			return nil, true
		case "k":
			if len(m.Tasks) > 0 && m.TaskCursor >= 0 && m.TaskCursor < len(m.Tasks) {
				task := m.Tasks[m.TaskCursor]
				if task.Status == "running" {
					tools.KillTask(task.ID)
				}
			}
			return nil, true
		case "x":
			if len(m.Tasks) > 0 && m.TaskCursor >= 0 && m.TaskCursor < len(m.Tasks) {
				task := m.Tasks[m.TaskCursor]
				if task.Status == "running" {
					tools.KillTask(task.ID)
				}
				m.Tasks = append(m.Tasks[:m.TaskCursor], m.Tasks[m.TaskCursor+1:]...)
				if m.TaskCursor >= len(m.Tasks) {
					m.TaskCursor = len(m.Tasks) - 1
				}
				if m.TaskCursor < 0 {
					m.TaskCursor = 0
				}
			}
			return nil, true
		}
		return nil, true 
	}

	if (m.State == StateIdle || m.State == StateThinking) && m.ShowSuggestionMenu && len(m.FilteredSuggestions) > 0 {
		switch msg.String() {
		case "up":
			m.SuggestionCursor--
			if m.SuggestionCursor < 0 {
				m.SuggestionCursor = len(m.FilteredSuggestions) - 1
			}
			return nil, true
		case "down":
			m.SuggestionCursor++
			if m.SuggestionCursor >= len(m.FilteredSuggestions) {
				m.SuggestionCursor = 0
			}
			return nil, true
		case "esc":
			m.ShowSuggestionMenu = false
			recalculateViewportHeight(m)
			return nil, true
		case "tab":
			selected := m.FilteredSuggestions[m.SuggestionCursor]
			m.Input.SetValue(selected + " ")
			m.Input.CursorEnd()
			m.ShowSuggestionMenu = false
			recalculateViewportHeight(m)
			return nil, true
		case "enter":
			selected := m.FilteredSuggestions[m.SuggestionCursor]
			m.Input.SetValue("")
			m.ShowSuggestionMenu = false
			m.Messages = append(m.Messages, UIMessage{
				Role:    "user",
				Content: selected,
			})
			cmd := handleSlashCommand(m, selected)
			updateViewportContent(m)
			recalculateViewportHeight(m)
			return cmd, true
		}
	}

	if (m.State == StateIdle || m.State == StateThinking) && m.ShowFileSuggestion && len(m.FilteredFileSuggestions) > 0 {
		switch msg.String() {
		case "up":
			m.FileSuggestionCursor--
			if m.FileSuggestionCursor < 0 {
				m.FileSuggestionCursor = len(m.FilteredFileSuggestions) - 1
			}
			return nil, true
		case "down":
			m.FileSuggestionCursor++
			if m.FileSuggestionCursor >= len(m.FilteredFileSuggestions) {
				m.FileSuggestionCursor = 0
			}
			return nil, true
		case "esc":
			m.ShowFileSuggestion = false
			recalculateViewportHeight(m)
			return nil, true
		case "tab", "enter":
			selected := m.FilteredFileSuggestions[m.FileSuggestionCursor]
			val := m.Input.Value()
			idx := strings.LastIndex(val, "@")
			if idx != -1 {
				prefix := val[:idx]
				autocompleteVal := prefix + "@" + selected + " "
				m.Input.SetValue(autocompleteVal)
				m.Input.CursorEnd()
			}
			m.ShowFileSuggestion = false
			recalculateViewportHeight(m)
			return nil, true
		}
	}

	switch msg.String() {
	case "ctrl+o":
		for i := len(m.Messages) - 1; i >= 0; i-- {
			if m.Messages[i].Role == "tool_call" {
				_, detail := formatToolCallAgyStyle(m.Messages[i].Name, m.Messages[i].Content)
				isLong := strings.Contains(detail, "\n") || len(detail) > 60
				if isLong {
					m.ExpandedTools[i] = !m.ExpandedTools[i]
					updateViewportContent(m)
					return nil, true
				}
			}
		}
		return nil, true
	case "up":
		if m.State == StateApprovalPending {
			m.ApprovalCursor--
			if m.ApprovalCursor < 0 {
				m.ApprovalCursor = m.getApprovalOptionsCount() - 1
			}
			return nil, true
		}
		if (m.State == StateIdle || m.State == StateThinking) && !m.ShowSuggestionMenu && !m.ShowFileSuggestion && len(m.InputHistory) > 0 {
			if m.HistoryIndex > 0 {
				m.HistoryIndex--
				m.Input.SetValue(m.InputHistory[m.HistoryIndex])
				m.Input.CursorEnd()
				updateInputHeight(m)
			}
			return nil, true
		}
	case "down":
		if m.State == StateApprovalPending {
			m.ApprovalCursor++
			if m.ApprovalCursor >= m.getApprovalOptionsCount() {
				m.ApprovalCursor = 0
			}
			return nil, true
		}
		if (m.State == StateIdle || m.State == StateThinking) && !m.ShowSuggestionMenu && !m.ShowFileSuggestion && len(m.InputHistory) > 0 {
			if m.HistoryIndex < len(m.InputHistory)-1 {
				m.HistoryIndex++
				m.Input.SetValue(m.InputHistory[m.HistoryIndex])
				m.Input.CursorEnd()
				updateInputHeight(m)
			} else if m.HistoryIndex == len(m.InputHistory)-1 {
				m.HistoryIndex = len(m.InputHistory)
				m.Input.SetValue("")
				m.Input.SetHeight(1)
			}
			return nil, true
		}
	case "ctrl+c":
		if m.ApprovalChan != nil {
			select {
			case m.ApprovalChan <- false:
			default:
			}
		}
		return tea.Quit, true

	case "esc":
		if m.State != StateIdle {
			if m.ApprovalChan != nil {
				select {
				case m.ApprovalChan <- false:
				default:
				}
				m.ApprovalChan = nil
			}
			if m.AgentCancel != nil {
				m.AgentCancel()
				m.AgentCancel = nil
			}
			m.State = StateIdle
			m.Messages = append(m.Messages, UIMessage{
				Role:    "system",
				Content: "Process interrupted by user.",
			})
			updateViewportContent(m)
			recalculateViewportHeight(m)
		}
		return nil, true

	case "tab":
		if m.Cfg.Mode == "safe" {
			m.Cfg.Mode = "autonomous"
		} else {
			m.Cfg.Mode = "safe"
		}
		return nil, true

	case "y", "Y":
		if (m.State == StateApprovalPending || m.State == StateChainLimitPending) && m.ApprovalChan != nil {
			select {
			case m.ApprovalChan <- true:
			default:
			}
			m.ApprovalChan = nil
			m.State = StateThinking
			recalculateViewportHeight(m)
			return tickThinking(), true
		}

	case "n", "N":
		if (m.State == StateApprovalPending || m.State == StateChainLimitPending) && m.ApprovalChan != nil {
			select {
			case m.ApprovalChan <- false:
			default:
			}
			m.ApprovalChan = nil
			m.State = StateIdle
			m.Messages = append(m.Messages, UIMessage{
				Role:    "system",
				Content: "Action rejected by user.",
			})
			updateViewportContent(m)
			recalculateViewportHeight(m)
			return nil, true
		}

	case "enter":
		if m.State == StateApprovalPending && m.ApprovalChan != nil {
			options := m.getApprovalOptions()
			selected := options[m.ApprovalCursor]

			var approved bool
			if strings.HasPrefix(selected, "Yes") {
				approved = true

				if strings.Contains(selected, "commands starting with") {
					var args map[string]any
					json.Unmarshal([]byte(m.PendingArgs), &args)
					cmd, _ := args["command"].(string)
					fields := strings.Fields(cmd)
					if len(fields) > 0 {
						m.AutoApproveCommands = append(m.AutoApproveCommands, fields[0])
					}
				} else if strings.Contains(selected, "always allow all 'execute_command'") || strings.Contains(selected, "always allow '"+m.PendingTool+"'") {
					m.AutoApproveTools[m.PendingTool] = true
				}
			} else {
				approved = false
			}

			select {
			case m.ApprovalChan <- approved:
			default:
			}
			m.ApprovalChan = nil

			if approved {
				m.State = StateThinking
				recalculateViewportHeight(m)
				return tickThinking(), true
			} else {
				m.State = StateIdle
				m.Messages = append(m.Messages, UIMessage{
					Role:    "system",
					Content: "Action rejected by user.",
				})
				updateViewportContent(m)
			}
			recalculateViewportHeight(m)
			return nil, true
		}

		if m.State == StateAskUserPending {
			userInput := m.Input.Value()
			if userInput != "" {
				m.Input.SetValue("")
				m.Input.SetHeight(1)

				m.Messages = append(m.Messages, UIMessage{
					Role:    "user",
					Content: userInput,
				})
				updateViewportContent(m)
				recalculateViewportHeight(m)

				go func(ch chan<- string, val string) {
					ch <- val
				}(m.AskUserChan, userInput)

				m.State = StateThinking
			}
			return nil, true
		}

		if m.State == StateIdle || m.State == StateThinking {
			userInput := m.Input.Value()
			if userInput != "" {
				if m.HasPastedText {
					startIdx := strings.Index(userInput, "[Pasted text #1")
					if startIdx != -1 {
						endIdx := strings.Index(userInput[startIdx:], "]")
						if endIdx != -1 {
							placeholder := userInput[startIdx : startIdx+endIdx+1]
							userInput = strings.Replace(userInput, placeholder, m.PastedBuffer, 1)
						}
					}
					m.PastedBuffer = ""
					m.HasPastedText = false
				}

				if len(m.InputHistory) == 0 || m.InputHistory[len(m.InputHistory)-1] != userInput {
					m.InputHistory = append(m.InputHistory, userInput)
				}
				m.HistoryIndex = len(m.InputHistory)

				m.Input.SetValue("")
				m.Input.SetHeight(1)
				m.ShowSuggestionMenu = false

				if strings.HasPrefix(userInput, "/") {
					m.Messages = append(m.Messages, UIMessage{
						Role:    "user",
						Content: userInput,
					})
					cmd := handleSlashCommand(m, userInput)
					updateViewportContent(m)
					recalculateViewportHeight(m)
					return cmd, true
				}

				if m.State == StateThinking {
					m.QueryQueue = append(m.QueryQueue, userInput)
					m.Messages = append(m.Messages, UIMessage{
						Role:    "user",
						Content: userInput + " (queued)",
					})
					updateViewportContent(m)
					recalculateViewportHeight(m)
					return nil, true
				}

				m.Messages = append(m.Messages, UIMessage{
					Role:    "user",
					Content: userInput,
				})
				updateViewportContent(m)
				recalculateViewportHeight(m)

				ctx, cancel := context.WithCancel(context.Background())
				m.AgentCancel = cancel

				go func(input string, c context.Context) {
					m.PromptChan <- AgentPrompt{Prompt: input, Ctx: c}
				}(userInput, ctx)
				return nil, true
			}
		}
	}

	if (m.State == StateIdle || m.State == StateThinking) && !m.ShowSuggestionMenu && !m.ShowFileSuggestion {
		k := msg.String()
		if k == "pgup" || k == "pgdown" || k == "home" || k == "end" {
			var vpCmd tea.Cmd
			m.Viewport, vpCmd = m.Viewport.Update(msg)
			if k == "pgup" || k == "home" {
				m.UserScrolledUp = true
			} else if m.Viewport.AtBottom() {
				m.UserScrolledUp = false
			}
			return vpCmd, true
		}
	}

	return nil, false
}
