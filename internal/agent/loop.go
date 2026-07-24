package agent

import (
	"context"

	"awas/internal/client"
	"awas/internal/config"
	"awas/internal/index"
)

type UI interface {
	PrintThinking(model string)
	PrintMessage(role string, content string)
	PrintMessageDelta(content string)
	PrintToolCall(name string, args string)
	PrintToolResult(name string, result string)
	PrintTokenUsage(count int)
	PrintCompression(turns int)
	RequestApproval(ctx context.Context, toolName string, args string, mode string) bool
	RequestChainContinue(ctx context.Context) bool
	AskUser(ctx context.Context, question string) (string, error)

	PrintPlan(goal string, steps []string)
	PrintStepStart(stepID string)
	PrintStepFinish(stepID string, success bool, result string)
	SendFile(filePath string, caption string) error
}

type Loop struct {
	cfg       *config.Config
	cli       *client.Client
	history   []client.Message
	UI        UI
	Index     *index.Index
	TurnCount int
}

func NewLoop(cfg *config.Config) *Loop {
	cloned := *cfg
	l := &Loop{
		cfg: &cloned,
		cli: client.New(&cloned),
		UI:  DefaultUI{},
	}
	l.history = []client.Message{
		{
			Role:    "system",
			Content: SystemPrompt + loadLocalSkills() + loadLocalMemories(l.cfg),
		},
	}
	return l
}

func (l *Loop) GetConfig() *config.Config {
	return l.cfg
}

func (l *Loop) SetClient(cli *client.Client) {
	l.cli = cli
}

func (l *Loop) SetHistory(history []client.Message) {
	l.history = history
	if len(l.history) > 0 {
		l.history[0].Content = SystemPrompt + loadLocalSkills()
		if l.Index != nil {
			l.UpdateSystemPromptWithContext()
		}
	}
}

func (l *Loop) GetHistory() []client.Message {
	return l.history
}

func (l *Loop) ResetHistory() {
	l.history = []client.Message{
		{
			Role:    "system",
			Content: SystemPrompt + loadLocalSkills() + loadLocalMemories(l.cfg),
		},
	}
}

func (l *Loop) Start(initialQuery string) {
	l.UI.PrintThinking(l.cfg.Model)
	if l.Index == nil {
		idx, err := index.LoadIndex(l.cfg.WorkDir)
		if err == nil {
			l.Index = idx
			l.UpdateSystemPromptWithContext()
		}
	}
	l.RunAgentCycle(context.Background(), initialQuery)
}
