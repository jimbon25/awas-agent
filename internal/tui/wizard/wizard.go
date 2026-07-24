package wizard

import (
	"context"
	"fmt"
	"strings"
	"time"

	"awas/internal/auth"
	"awas/internal/provider"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
)

type Step int

const (
	StepProviderSelect Step = iota
	StepEndpointInput
	StepAPIKeyInput
	StepModelSelect
	StepOAuthDisplay
)

const GitHubClientID = "Ov23lianaqy6tIN7nS9g"

var (
	ColorPrimary = lipgloss.Color("#00F0FF")
	ColorWarning = lipgloss.Color("#FF9900")
	ColorMuted   = lipgloss.Color("#888888")
	ColorWhite   = lipgloss.Color("#FFFFFF")
	ColorSuccess = lipgloss.Color("#00FF66")

	StyleTitle   = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	StyleMuted   = lipgloss.NewStyle().Foreground(ColorMuted)
	StyleBold    = lipgloss.NewStyle().Bold(true)
	StyleAlert   = lipgloss.NewStyle().Foreground(ColorWarning).Bold(true)
	StyleSuccess = lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true)
)

type SetupFinishedMsg struct {
	Config     *provider.ProviderConfig
	OAuthToken string
}

type Model struct {
	Step         Step
	Cursor       int
	Options      []string
	SelectedProv provider.ProviderType

	EndpointIn  textinput.Model
	APIKeyIn    textinput.Model
	CustomModel textinput.Model

	ActiveConfig *provider.ProviderConfig

	FetchedModels []string
	ModelCursor   int
	LoadingModels bool
	LoadingError  string
	ManualInput   bool

	DeviceFlow *auth.DeviceFlow
	AuthClient *auth.AuthClient
	CancelPoll context.CancelFunc
	OAuthError string

	SpinnerTicks int
	Width        int
	Height       int
}

type spinnerTickMsg struct{}
type modelsFetchedMsg []string
type oauthTokenMsg string

func tickSpinner() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

func fetchModelsCmd(p *provider.ProviderConfig) tea.Cmd {
	return func() tea.Msg {
		models, err := p.FetchModels()
		if err != nil {
			return err
		}
		return modelsFetchedMsg(models)
	}
}

func StartDeviceFlowCmd(a *auth.AuthClient) tea.Cmd {
	return func() tea.Msg {
		flow, err := a.StartDeviceFlow()
		if err != nil {
			return err
		}
		return flow
	}
}

func pollTokenCmd(ctx context.Context, a *auth.AuthClient, f *auth.DeviceFlow) tea.Cmd {
	return func() tea.Msg {
		token, err := a.PollForToken(ctx, f)
		if err != nil {
			return err
		}
		return oauthTokenMsg(token)
	}
}

func New() Model {
	se := textinput.New()
	se.Placeholder = "https://api.openai.com/v1"
	se.Focus()
	se.SetWidth(50)

	sa := textinput.New()
	sa.Placeholder = "Enter your API Key..."
	sa.EchoMode = textinput.EchoPassword
	sa.EchoCharacter = '•'
	sa.SetWidth(50)

	sm := textinput.New()
	sm.Placeholder = "Enter model name (e.g. gpt-4o)..."
	sm.SetWidth(45)

	return Model{
		Step:   StepProviderSelect,
		Cursor: 0,
		Options: []string{
			"1. OpenAI",
			"2. Anthropic",
			"3. Gemini",
			"4. OpenRouter",
			"5. Custom Endpoint",
			"6. OAuth Login (GitHub)",
		},
		EndpointIn:   se,
		APIKeyIn:     sa,
		CustomModel:  sm,
		ActiveConfig: &provider.ProviderConfig{},
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		tickSpinner(),
	)
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case spinnerTickMsg:
		m.SpinnerTicks++
		return m, tickSpinner()

	case modelsFetchedMsg:
		m.LoadingModels = false
		m.FetchedModels = msg
		m.ModelCursor = 0
		if len(m.FetchedModels) == 0 {
			m.ManualInput = true
			m.CustomModel.Focus()
		}
		return m, nil

	case error:
		if m.Step == StepOAuthDisplay {
			m.OAuthError = msg.Error()
		} else {
			m.LoadingModels = false
			m.LoadingError = msg.Error()
			m.ManualInput = true
			m.CustomModel.Focus()
		}
		return m, nil

	case *auth.DeviceFlow:
		m.DeviceFlow = msg
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		m.CancelPoll = cancel
		return m, pollTokenCmd(ctx, m.AuthClient, m.DeviceFlow)

	case oauthTokenMsg:
		if m.CancelPoll != nil {
			m.CancelPoll()
		}
		return m, func() tea.Msg {
			return SetupFinishedMsg{
				OAuthToken: string(msg),
			}
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			if m.CancelPoll != nil {
				m.CancelPoll()
			}
			return m, tea.Quit

		case "esc":
			switch m.Step {
			case StepEndpointInput, StepAPIKeyInput:
				m.Step = StepProviderSelect
				return m, nil
			case StepModelSelect:
				m.Step = StepAPIKeyInput
				m.APIKeyIn.Focus()
				m.ManualInput = false
				m.LoadingError = ""
				return m, nil
			case StepOAuthDisplay:
				if m.CancelPoll != nil {
					m.CancelPoll()
				}
				m.OAuthError = ""
				m.Step = StepProviderSelect
				return m, nil
			}

		case "up", "k":
			if m.Step == StepProviderSelect {
				m.Cursor--
				if m.Cursor < 0 {
					m.Cursor = len(m.Options) - 1
				}
			} else if m.Step == StepModelSelect && !m.ManualInput {
				m.ModelCursor--
				if m.ModelCursor < 0 {
					m.ModelCursor = len(m.FetchedModels) - 1
				}
			}

		case "down", "j":
			if m.Step == StepProviderSelect {
				m.Cursor++
				if m.Cursor >= len(m.Options) {
					m.Cursor = 0
				}
			} else if m.Step == StepModelSelect && !m.ManualInput {
				m.ModelCursor++
				if m.ModelCursor >= len(m.FetchedModels) {
					m.ModelCursor = 0
				}
			}

		case "enter":
			switch m.Step {
			case StepProviderSelect:
				switch m.Cursor {
				case 0:
					m.ActiveConfig.Name = provider.ProviderOpenAI
					m.Step = StepAPIKeyInput
					m.APIKeyIn.Focus()
				case 1:
					m.ActiveConfig.Name = provider.ProviderAnthropic
					m.Step = StepAPIKeyInput
					m.APIKeyIn.Focus()
				case 2:
					m.ActiveConfig.Name = provider.ProviderGemini
					m.Step = StepAPIKeyInput
					m.APIKeyIn.Focus()
				case 3:
					m.ActiveConfig.Name = provider.ProviderOpenRouter
					m.Step = StepAPIKeyInput
					m.APIKeyIn.Focus()
				case 4:
					m.ActiveConfig.Name = provider.ProviderCustom
					m.Step = StepEndpointInput
					m.EndpointIn.Focus()
				case 5:
					// Start OAuth flow
					m.AuthClient = auth.NewAuthClient(
						GitHubClientID,
						"https://github.com/login/device/code",
						"https://github.com/login/oauth/access_token",
					)
					m.OAuthError = ""
					m.DeviceFlow = nil
					m.Step = StepOAuthDisplay
					return m, StartDeviceFlowCmd(m.AuthClient)
				}
				return m, nil

			case StepEndpointInput:
				endpoint := strings.TrimSpace(m.EndpointIn.Value())
				if endpoint == "" {
					endpoint = m.EndpointIn.Placeholder
				}
				m.ActiveConfig.Endpoint = endpoint
				m.Step = StepAPIKeyInput
				m.APIKeyIn.Focus()
				return m, nil

			case StepAPIKeyInput:
				m.ActiveConfig.APIKey = strings.TrimSpace(m.APIKeyIn.Value())
				m.Step = StepModelSelect
				m.LoadingModels = true
				m.LoadingError = ""
				m.FetchedModels = nil
				m.ManualInput = false
				return m, fetchModelsCmd(m.ActiveConfig)

			case StepModelSelect:
				if m.ManualInput {
					modelName := strings.TrimSpace(m.CustomModel.Value())
					if modelName == "" {
						modelName = m.CustomModel.Placeholder
					}
					m.ActiveConfig.Model = modelName
				} else {
					if len(m.FetchedModels) > 0 {
						m.ActiveConfig.Model = m.FetchedModels[m.ModelCursor]
					}
				}

				return m, func() tea.Msg {
					return SetupFinishedMsg{
						Config: m.ActiveConfig,
					}
				}
			}
		}
	}

	switch m.Step {
	case StepEndpointInput:
		m.EndpointIn, cmd = m.EndpointIn.Update(msg)
	case StepAPIKeyInput:
		m.APIKeyIn, cmd = m.APIKeyIn.Update(msg)
	case StepModelSelect:
		if m.ManualInput {
			m.CustomModel, cmd = m.CustomModel.Update(msg)
		}
	}

	return m, cmd
}

func (m Model) View() string {
	var s strings.Builder
	s.WriteString("\n  " + StyleTitle.Render("AWAS CLI Setup Wizard") + "\n\n")

	switch m.Step {
	case StepProviderSelect:
		s.WriteString("  Please select your LLM service provider:\n\n")
		for i, option := range m.Options {
			if m.Cursor == i {
				s.WriteString(fmt.Sprintf("    %s\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("> "+option)))
			} else {
				s.WriteString(fmt.Sprintf("      %s\n", option))
			}
		}
		s.WriteString("\n  " + StyleMuted.Render("Use ↑/↓ keys to select · enter to confirm · ctrl+c to exit") + "\n")

	case StepEndpointInput:
		s.WriteString("  [Step 1 of 3] Configure Endpoint URL:\n")
		s.WriteString("  Enter the OpenAI-compatible base endpoint URL.\n\n")
		s.WriteString("  Endpoint:  " + m.EndpointIn.View() + "\n\n")
		s.WriteString("  " + StyleMuted.Render("enter to confirm · esc to go back") + "\n")

	case StepAPIKeyInput:
		s.WriteString("  [Step 2 of 3] Configure API Key:\n")
		s.WriteString("  Enter your secret API authentication key.\n\n")
		s.WriteString("  API Key:   " + m.APIKeyIn.View() + "\n\n")
		s.WriteString("  " + StyleMuted.Render("enter to confirm · esc to go back") + "\n")

	case StepModelSelect:
		s.WriteString("  [Step 3 of 3] Select Model ID:\n")
		if m.LoadingModels {
			spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
			frame := spinnerFrames[m.SpinnerTicks%len(spinnerFrames)]
			s.WriteString(fmt.Sprintf("\n  %s %s\n\n", StyleAlert.Render(frame), StyleBold.Render("Fetching available models from endpoint...")))
		} else if m.ManualInput {
			if m.LoadingError != "" {
				s.WriteString("  " + StyleAlert.Render("Could not fetch models automatically:") + "\n")
				s.WriteString(fmt.Sprintf("  %s\n\n", StyleMuted.Render(m.LoadingError)))
			}
			s.WriteString("  Please enter your Model Identifier manually:\n\n")
			s.WriteString("  Model ID:  " + m.CustomModel.View() + "\n\n")
			s.WriteString("  " + StyleMuted.Render("enter to confirm · esc to go back") + "\n")
		} else {
			s.WriteString("  Choose a model returned from the server:\n\n")
			
			maxVisible := 8
			start := 0
			end := len(m.FetchedModels)
			if len(m.FetchedModels) > maxVisible {
				start = m.ModelCursor - maxVisible/2
				if start < 0 {
					start = 0
				}
				end = start + maxVisible
				if end > len(m.FetchedModels) {
					end = len(m.FetchedModels)
					start = end - maxVisible
				}
			}

			if start > 0 {
				s.WriteString(fmt.Sprintf("      %s\n", StyleMuted.Render(fmt.Sprintf("▲ ... %d more models above ...", start))))
			} else {
				s.WriteString("\n")
			}

			for i := start; i < end; i++ {
				model := m.FetchedModels[i]
				if m.ModelCursor == i {
					s.WriteString(fmt.Sprintf("    %s\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("> "+model)))
				} else {
					s.WriteString(fmt.Sprintf("      %s\n", model))
				}
			}

			if end < len(m.FetchedModels) {
				s.WriteString(fmt.Sprintf("      %s\n", StyleMuted.Render(fmt.Sprintf("▼ ... %d more models below ...", len(m.FetchedModels)-end))))
			} else {
				s.WriteString("\n")
			}

			s.WriteString("\n  " + StyleMuted.Render("Use ↑/↓ keys to select · enter to confirm · esc to go back") + "\n")
		}

	case StepOAuthDisplay:
		s.WriteString("  [GitHub OAuth Login]\n")
		if m.OAuthError != "" {
			s.WriteString("  " + StyleAlert.Render("OAuth Error:") + "\n")
			s.WriteString(fmt.Sprintf("  %s\n\n", StyleMuted.Render(m.OAuthError)))
		} else if m.DeviceFlow == nil {
			spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
			frame := spinnerFrames[m.SpinnerTicks%len(spinnerFrames)]
			s.WriteString(fmt.Sprintf("\n  %s %s\n\n", StyleAlert.Render(frame), StyleBold.Render("Requesting device authorization code...")))
		} else {
			s.WriteString("  Please authorize this CLI client in your browser:\n\n")
			s.WriteString(fmt.Sprintf("  1. Open: %s\n", StyleSuccess.Render(m.DeviceFlow.VerificationURI)))
			s.WriteString(fmt.Sprintf("  2. Enter Code: %s\n\n", StyleSuccess.Copy().Underline(true).Render(m.DeviceFlow.UserCode)))
			spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
			frame := spinnerFrames[m.SpinnerTicks%len(spinnerFrames)]
			s.WriteString(fmt.Sprintf("  %s %s\n\n", StyleAlert.Render(frame), StyleBold.Render("Waiting for browser confirmation...")))
		}
		s.WriteString("  " + StyleMuted.Render("esc to cancel and go back") + "\n")
	}

	return s.String()
}
