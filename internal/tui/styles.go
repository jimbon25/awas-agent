package tui

import "github.com/charmbracelet/lipgloss"

const (
	ColorPrimary   = lipgloss.Color("#00F0FF") // Cyan
	ColorSuccess   = lipgloss.Color("#00FF66") // Green
	ColorWarning   = lipgloss.Color("#FF9900") // Orange
	ColorError     = lipgloss.Color("#FF3333") // Red
	ColorMuted     = lipgloss.Color("#888888") // Medium Grey
	ColorDarkMuted = lipgloss.Color("#444444") // Dark Grey
	ColorWhite     = lipgloss.Color("#FFFFFF") // White
	ColorBg        = lipgloss.Color("#1a1a1a") // Charcoal
	ColorClaudeOrange = lipgloss.Color("#D97706")
)

var (
	StyleLogo = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true).
			Align(lipgloss.Center)

	StyleHeader = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(0, 1).
			MarginBottom(1)

	StyleModelLabel = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	StyleModeSafe = lipgloss.NewStyle().
			Foreground(ColorBg).
			Background(ColorWarning).
			Bold(true).
			Padding(0, 1)

	StyleModeAuto = lipgloss.NewStyle().
			Foreground(ColorBg).
			Background(ColorSuccess).
			Bold(true).
			Padding(0, 1)

	StyleUserMsg = lipgloss.NewStyle().
			Foreground(ColorClaudeOrange).
			Bold(true)

	StyleAgentMsg = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#dddddd"))

	StyleSystemMsg = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)

	StyleToolCall = lipgloss.NewStyle().
			Foreground(ColorPrimary)

	StyleToolSuccess = lipgloss.NewStyle().
			Foreground(ColorSuccess)

	StyleToolError = lipgloss.NewStyle().
			Foreground(ColorError)

	StyleThought = lipgloss.NewStyle().
			Foreground(ColorWarning).
			Italic(true)

	StyleTokenUsage = lipgloss.NewStyle().
			Foreground(ColorWarning)

	StyleStatusBar = lipgloss.NewStyle().
			Background(ColorDarkMuted).
			Foreground(ColorWhite).
			Padding(0, 1)

	StyleShortcut = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	StyleInputBox = lipgloss.NewStyle().
			Border(lipgloss.Border{Left: "┃"}, false, false, false, true).
			BorderForeground(ColorPrimary).
			PaddingLeft(1).
			MarginTop(1)

	StylePendingPrompt = lipgloss.NewStyle().
				Foreground(ColorWarning).
				Bold(true)

	StyleMenuBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(0, 1).
			MarginBottom(1)

	StyleSelectionCursor = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	StyleSuggestionActive = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	StyleItalic = lipgloss.NewStyle().
			Italic(true)
 
	StyleMuted = lipgloss.NewStyle().
			Foreground(ColorMuted)
)
