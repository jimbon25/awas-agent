package tui

import (
	"awas/internal/auth"
	"awas/internal/client"
	"awas/internal/provider"
	"awas/internal/tools"
	"awas/internal/tui/wizard"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type AgentThinkingMsg struct {
	Model string
}

type AgentCompressionMsg struct {
	Turns int
}

type AgentMessageMsg struct {
	Role    string
	Content string
}

type AgentMessageDeltaMsg struct {
	Content string
}

type AgentToolCallMsg struct {
	Name string
	Args string
}

type AgentToolResultMsg struct {
	Name    string
	Result  string
	Success bool
}

type AgentApprovalRequestMsg struct {
	ToolName string
	Args     string
	RespChan chan bool
}

type AgentAskUserRequestMsg struct {
	Question string
	RespChan chan string
}

type AgentChainLimitRequestMsg struct {
	RespChan chan bool
}

type AgentTokenUsageMsg struct {
	Count int
}

type AgentFinishedMsg struct{}

type ThinkingTickMsg struct{}

func tickThinking() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return ThinkingTickMsg{}
	})
}

type TypewriterTickMsg struct{}

func tickTypewriter() tea.Cmd {
	return tea.Tick(30*time.Millisecond, func(t time.Time) tea.Msg {
		return TypewriterTickMsg{}
	})
}

func recalculateViewportHeight(m *Model) {
	if m.Height <= 0 {
		return
	}
	fixedHeight := 9
	if m.State == StateThinking {
		fixedHeight += 2 
	}
	if m.State == StateApprovalPending {
		optionsCount := m.getApprovalOptionsCount()
		cmdLines := 1
		if m.PendingTool == "execute_command" {
			var args map[string]any
			if json.Unmarshal([]byte(m.PendingArgs), &args) == nil {
				cmd, _ := args["command"].(string)
				
				cmdLinesList := strings.Split(cmd, "\n")
				displayCmd := cmd
				if len(cmdLinesList) > 5 {
					displayCmd = strings.Join(cmdLinesList[:5], "\n") + "\n    ... (truncated)"
				}
				
				width := m.Width - 8
				if width > 0 {
					totalLines := 0
					for _, l := range strings.Split(displayCmd, "\n") {
						wrapped := (len(l) + width - 1) / width
						if wrapped <= 0 {
							wrapped = 1
						}
						totalLines += wrapped
					}
					cmdLines = totalLines
				}
			}
		}
		fixedHeight = 15 + cmdLines + optionsCount
	}
	if (m.State == StateIdle || m.State == StateThinking) && m.ShowSuggestionMenu && len(m.FilteredSuggestions) > 0 {
		visibleCount := len(m.FilteredSuggestions)
		if visibleCount > 4 {
			visibleCount = 4
		}
		fixedHeight += visibleCount + 4 
	}
	if (m.State == StateIdle || m.State == StateThinking) && m.ShowFileSuggestion && len(m.FilteredFileSuggestions) > 0 {
		visibleCount := len(m.FilteredFileSuggestions)
		if visibleCount > 4 {
			visibleCount = 4
		}
		fixedHeight += visibleCount + 4 
	}
	inputHeight := m.Input.Height()
	if inputHeight < 1 {
		inputHeight = 1
	}
	vpHeight := m.Height - fixedHeight - (inputHeight - 1)
	if vpHeight < 3 {
		vpHeight = 3
	}
	if m.Viewport.Width() != m.Width-4 || m.Viewport.Height() != vpHeight {
		m.Viewport.SetWidth(m.Width - 4)
		m.Viewport.SetHeight(vpHeight)
		m.Viewport.SetXOffset(0)
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	if m.State == StateSetupWizard {
		if _, ok := msg.(wizard.SetupFinishedMsg); !ok {
			if _, ok := msg.(tea.WindowSizeMsg); !ok {
				var wzCmd tea.Cmd
				m.WizardModel, wzCmd = m.WizardModel.Update(msg)
				return m, wzCmd
			}
		}
	}

	switch msg := msg.(type) {
	case wizard.SetupFinishedMsg:
		if msg.OAuthToken != "" {
			creds := auth.NewCredentials("")
			creds.AccessToken = msg.OAuthToken
			creds.Provider = "github"
			creds.Save()

			mgr := provider.NewManager("")
			mgr.Profiles["github"] = &provider.ProviderConfig{
				Name:     "github",
				Endpoint: "https://models.inference.ai.azure.com/chat/completions",
				APIKey:    msg.OAuthToken,
				Model:    "gpt-4o-mini",
			}
			mgr.ActiveProfile = "github"
			mgr.Save()

			m.Cfg.APIKey = msg.OAuthToken
			m.Cfg.Endpoint = "https://models.inference.ai.azure.com/chat/completions"
			m.Cfg.Model = "gpt-4o-mini"
			m.Cfg.Save()

			m.State = StateIdle
			recalculateViewportHeight(&m)
			m.Messages = append(m.Messages, UIMessage{
				Role:    "system",
				Content: "GitHub OAuth Authentication completed successfully!",
			})
			updateViewportContent(&m)
		} else if msg.Config != nil {
			mgr := provider.NewManager("")
			profileName := string(msg.Config.Name)
			if profileName == "custom" {
				profileName = "custom_endpoint"
			}
			mgr.Profiles[profileName] = msg.Config
			mgr.ActiveProfile = profileName
			mgr.Save()

			m.Cfg.Endpoint = msg.Config.GetEndpoint()
			m.Cfg.APIKey = msg.Config.GetAPIKey()
			m.Cfg.Model = msg.Config.GetModel()
			m.Cfg.Save()

			m.Loop.SetClient(client.New(msg.Config))

			m.State = StateIdle
			recalculateViewportHeight(&m)
			m.Messages = append(m.Messages, UIMessage{
				Role:    "system",
				Content: "Setup completed! Connected to provider: " + string(msg.Config.Name) + " (Model: " + msg.Config.Model + ")",
			})
			updateViewportContent(&m)
		}
		return m, nil

	case tea.KeyPressMsg:
		if cmd, handled := handleKeyPress(&m, msg); handled {
			return m, cmd
		}

	case tea.MouseMsg:
		if (m.State == StateIdle || m.State == StateThinking) && !m.ShowSuggestionMenu && !m.ShowFileSuggestion {
			var vpCmd tea.Cmd
			m.Viewport, vpCmd = m.Viewport.Update(msg)
			if m.Viewport.AtBottom() {
				m.UserScrolledUp = false
			}
			return m, vpCmd
		}

	case tea.PasteMsg:
		if m.State == StateSetupWizard {
			var wzCmd tea.Cmd
			m.WizardModel, wzCmd = m.WizardModel.Update(msg)
			return m, wzCmd
		}
		if m.State == StateIdle {
			pastedText := strings.ReplaceAll(msg.Content, "\r", "")
			if len(pastedText) >= 50 || strings.Contains(pastedText, "\n") {
				lineCount := strings.Count(pastedText, "\n") + 1
				var placeholder string
				if lineCount > 1 {
					placeholder = fmt.Sprintf("[Pasted text #1 +%d lines]", lineCount)
				} else {
					placeholder = fmt.Sprintf("[Pasted text #1 +%d chars]", len(pastedText))
				}
				m.PastedBuffer = pastedText
				m.HasPastedText = true
				m.Input.InsertString(placeholder)
			} else {
				m.Input.InsertString(pastedText)
			}
			updateInputHeight(&m)
			return m, nil
		}


	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.Input.SetWidth(msg.Width - 10)
		recalculateViewportHeight(&m)
		updateViewportContent(&m)

		if m.State == StateSetupWizard {
			var wzCmd tea.Cmd
			m.WizardModel, wzCmd = m.WizardModel.Update(msg)
			return m, wzCmd
		}
		return m, nil

	case WorkspaceFilesMsg, AgentCancelInitMsg, AgentThinkingMsg, ThinkingTickMsg, AgentPlanCreatedMsg,
		AgentPlanStepStartMsg, AgentPlanStepFinishMsg, AgentMessageMsg, AgentMessageDeltaMsg,
		TypewriterTickMsg, AgentToolCallMsg, tools.TaskEvent, AgentToolResultMsg,
		AgentApprovalRequestMsg, AgentChainLimitRequestMsg, AgentAskUserRequestMsg,
		AgentCompressionMsg, AgentTokenUsageMsg, AgentFinishedMsg:
		cmd, _ = handleAgentMessage(&m, msg)
		return m, cmd
	}

	if m.State == StateIdle || m.State == StateThinking {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if keyMsg.String() == "ctrl+j" {
				m.Input.InsertString("\n")
				updateInputHeight(&m)
				return m, nil
			}
		}

		oldVal := m.Input.Value()
		m.Input, cmd = m.Input.Update(msg)
		cmds = append(cmds, cmd)

		newVal := m.Input.Value()
		if strings.Contains(newVal, "\r") {
			newVal = strings.ReplaceAll(newVal, "\r", "")
			m.Input.SetValue(newVal)
		}
		if newVal != oldVal {
			addedLen := len(newVal) - len(oldVal)
			if addedLen >= 50 || (addedLen > 0 && strings.Contains(newVal[len(oldVal):], "\n")) {
				pastedText := newVal[len(oldVal):]
				lineCount := strings.Count(pastedText, "\n") + 1
				var placeholder string
				if lineCount > 1 {
					placeholder = fmt.Sprintf("[Pasted text #1 +%d lines]", lineCount)
				} else {
					placeholder = fmt.Sprintf("[Pasted text #1 +%d chars]", len(pastedText))
				}
				m.PastedBuffer = pastedText
				m.HasPastedText = true
				newVal = oldVal + placeholder
				m.Input.SetValue(newVal)
				m.Input.CursorEnd()
			}

			if m.HasPastedText && !strings.Contains(newVal, "[Pasted text #1") {
				m.PastedBuffer = ""
				m.HasPastedText = false
			}
			updateInputHeight(&m)

			if strings.HasPrefix(newVal, "/") {
				var filtered []string
				for _, info := range AvailableCommands {
					if strings.HasPrefix(info.Command, newVal) {
						filtered = append(filtered, info.Command)
					}
				}
				m.FilteredSuggestions = filtered
				m.ShowSuggestionMenu = len(filtered) > 0
				if m.SuggestionCursor >= len(filtered) {
					m.SuggestionCursor = 0
				}
				m.ShowFileSuggestion = false
			} else if strings.Contains(newVal, "@") {
				idx := strings.LastIndex(newVal, "@")
				searchQuery := newVal[idx+1:]
				if !strings.Contains(searchQuery, " ") {
					if len(m.WorkspaceFiles) == 0 {
						recalculateViewportHeight(&m)
						return m, getWorkspaceFiles(m.Cfg.WorkDir)
					}
					var filtered []string
					for _, f := range m.WorkspaceFiles {
						if strings.Contains(strings.ToLower(f), strings.ToLower(searchQuery)) {
							filtered = append(filtered, f)
						}
					}
					m.FilteredFileSuggestions = filtered
					m.ShowFileSuggestion = len(filtered) > 0
					if m.FileSuggestionCursor >= len(filtered) {
						m.FileSuggestionCursor = 0
					}
					if len(m.FilteredFileSuggestions) > 5 {
						m.FilteredFileSuggestions = m.FilteredFileSuggestions[:5]
					}
				} else {
					m.ShowFileSuggestion = false
				}
				m.ShowSuggestionMenu = false
			} else {
				m.ShowSuggestionMenu = false
				m.ShowFileSuggestion = false
			}
			recalculateViewportHeight(&m)
		}
	}

	return m, tea.Batch(cmds...)
}



func updateInputHeight(m *Model) {
	newVal := m.Input.Value()
	lines := strings.Split(newVal, "\n")
	totalLines := 0
	width := m.Input.Width()
	if width <= 0 {
		width = 60
	}
	for _, l := range lines {
		wrapped := (len(l) + width - 1) / width
		if wrapped <= 0 {
			wrapped = 1
		}
		totalLines += wrapped
	}
	if totalLines < 1 {
		totalLines = 1
	}
	if totalLines > 5 {
		totalLines = 5
	}
	if totalLines != m.Input.Height() {
		m.Input.SetHeight(totalLines)
		recalculateViewportHeight(m)
	}
}
