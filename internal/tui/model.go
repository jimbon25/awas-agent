package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"awas/internal/agent"
	"awas/internal/config"
	"awas/internal/gateway"
	"awas/internal/tools"
	"awas/internal/tui/wizard"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

type State int

type AgentPrompt struct {
	Prompt string
	Ctx    context.Context
}

const (
	StateIdle State = iota
	StateThinking
	StateApprovalPending
	StateChainLimitPending
	StateHistoryView 
	StateSetupWizard 
	StateTasksView   
	StateProfileSwitch 
	StateSkillsMenu    
	StateAskUserPending
)

type SkillMeta struct {
	Name   string
	Path   string
	Active bool
}

type UIMessage struct {
	Role    string 
	Name    string 
	Content string
	Success bool
}

type Task struct {
	ID        string
	Command   string
	StartTime time.Time
	Status    string
	ExitCode  int
}

type Model struct {
	Cfg            *config.Config
	State          State
	PreviousState  State
	Messages       []UIMessage
	Input          textarea.Model
	PastedBuffer   string
	HasPastedText  bool
	CompressedTurns int
	IsStreaming     bool
	Width          int
	Height         int

	TokenCount     int
	TokenMax       int

	ThinkingModel  string
	ThinkingTicks  int

	PendingTool    string
	PendingArgs    string
	ApprovalChan   chan bool

	PendingQuestion string
	AskUserChan     chan string

	PromptChan     chan AgentPrompt
	AgentCancel    context.CancelFunc

	ShowSuggestionMenu  bool
	SuggestionCursor    int
	FilteredSuggestions []string

	WorkspaceFiles          []string
	ShowFileSuggestion      bool
	FileSearchQuery         string
	FilteredFileSuggestions []string
	FileSuggestionCursor    int

	WizardModel             wizard.Model

	QueryQueue              []string

	InputHistory            []string
	HistoryIndex            int

	ApprovalCursor          int
	AutoApproveTools        map[string]bool
	AutoApproveCommands     []string

	Tasks                   []Task
	TaskCursor              int
	ActiveTaskCount         int 

	LastCtrlCTime           time.Time

	LatestVersionAvailable  string
	UpdateStatus            string 
	UpdateNewVersion        string

	TypewriterRunes         []rune
	TypewriterIndex         int
	TypewriterMsgIndex      int

	Viewport            viewport.Model

	Loop                *agent.Loop
	ActiveSessionID     string
	ActiveSessionTitle  string
	ActiveSessionCreatedAt time.Time
	LastSavedSeq        int 
	ExpandedTools       map[int]bool
	LastTaskOutput      string
	HistorySearchInput  textinput.Model
	HistoryRenameMode   bool
	HistoryRenameInput  textinput.Model
	Sessions            []SessionMeta
	FilteredSessions    []SessionMeta
	HistoryCursor       int
	HistoryPage         int

	SwitchProfiles      []string
	SwitchCursor        int

	Skills              []SkillMeta
	SkillsCursor        int
	SkillsPage          int

	GatewayMgr          *gateway.Manager

	ActivePlanGoal         string
	ActivePlanSteps        []string
	ActivePlanStepStatuses map[string]string 

	RenderedLines       map[int][]string  
	UserScrolledUp      bool              
	lastMsgCount        int               
}

type WorkspaceFilesMsg []string

func getWorkspaceFiles(dir string) tea.Cmd {
	return func() tea.Msg {
		var files []string
		filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				if strings.HasPrefix(name, ".") || name == "node_modules" || name == "dist" || name == "vendor" {
					return filepath.SkipDir
				}
				return nil
			}

			rel, err := filepath.Rel(dir, path)
			if err == nil {
				files = append(files, rel)
			}
			return nil
		})
		return WorkspaceFilesMsg(files)
	}
}

func NewModel(cfg *config.Config, promptChan chan AgentPrompt) Model {
	var setupMode bool
	home, err := os.UserHomeDir()
	if err != nil {
		setupMode = true
	} else {
		configPath := filepath.Join(home, ".awas", "config.json")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			setupMode = true
		}
	}

	if cfg.Endpoint == "" || cfg.Model == "" {
		setupMode = true
	}

	state := StateIdle
	var wz wizard.Model
	if setupMode {
		state = StateSetupWizard
		wz = wizard.New()
	}

	ti := textarea.New()
	ti.Placeholder = "Ask awas to write some code, run commands..."
	ti.Focus()
	ti.SetHeight(1)
	ti.SetWidth(60)
	ti.ShowLineNumbers = false
	ti.CharLimit = 0
	ti.Prompt = ""

	vp := viewport.New(viewport.WithWidth(60), viewport.WithHeight(10))
	vp.SoftWrap = false
	vp.MouseWheelEnabled = true

	si := textinput.New()
	si.Placeholder = "Type to search past conversations..."
	si.SetWidth(40)

	ri := textinput.New()
	ri.Placeholder = "Enter new title..."
	ri.SetWidth(40)

	sessionID := fmt.Sprintf("session-%d", time.Now().UnixNano())
	tools.SetCurrentSessionID(sessionID)

	return Model{
		Cfg:                cfg,
		State:              state,
		PreviousState:      state,
		Messages:           []UIMessage{},
		Input:              ti,
		TokenCount:         0,
		TokenMax:           cfg.MaxTokens,
		PromptChan:         promptChan,
		Viewport:           vp,
		ActiveSessionID:    sessionID,
		ActiveSessionTitle: "New Conversation",
		ActiveSessionCreatedAt: time.Now(),
		HistorySearchInput: si,
		HistoryRenameInput: ri,
		WizardModel:        wz,
		ApprovalCursor:      0,
		AutoApproveTools:    make(map[string]bool),
		AutoApproveCommands: []string{},
		Tasks:               []Task{},
		TaskCursor:          0,
		LastCtrlCTime:       time.Time{},
		AgentCancel:         nil,
		TypewriterRunes:     nil,
		TypewriterIndex:     0,
		TypewriterMsgIndex:  -1,
		ExpandedTools:       make(map[int]bool),
		LastTaskOutput:      "",
		Skills:              []SkillMeta{},
		SkillsCursor:        0,
		GatewayMgr:          nil, 
		RenderedLines:       make(map[int][]string),
		UserScrolledUp:      false,
		ActivePlanStepStatuses: make(map[string]string),
	}
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		textarea.Blink,
		CheckForUpdates(Version),
	}
	if m.State == StateSetupWizard {
		cmds = append(cmds, m.WizardModel.Init())
	}
	return tea.Batch(cmds...)
}
