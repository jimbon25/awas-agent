package tui

import (
	"awas/internal/agent"
	"awas/internal/config"
	"awas/internal/gateway"
	_ "awas/internal/gateway/telegram" 
	_ "awas/internal/gateway/discord"  
	"awas/internal/provider"
	"awas/internal/tools"
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
)

type AgentCancelInitMsg struct {
	Cancel context.CancelFunc
}

func Run(cfg *config.Config, initialQuery string) error {
	promptChan := make(chan AgentPrompt, 10)
	taskChan := make(chan tools.TaskEvent, 100)

	tools.RegisterTaskEventChan(taskChan)

	tools.SetSearchConfig(tools.WebSearchConfig{
		SearXNGURL: cfg.SearXNGURL,
	})

	loop := agent.NewLoop(cfg)

	m := NewModel(cfg, promptChan)
	m.Loop = loop

	gwMgr := gateway.NewManager(cfg)
	m.GatewayMgr = gwMgr

	if gateway.TryAcquireTUIGatewayLock() {
		defer gateway.ReleaseTUIGatewayLock()
		gwCfg := gwMgr.Load()
		if gwCfg.Enabled {
			for name, platform := range gwCfg.Platforms {
				if platform.Enabled {
					go gwMgr.Start(name)
				}
			}
		}
	}

	if initialQuery != "" {
		m.Messages = append(m.Messages, UIMessage{
			Role:    "user",
			Content: initialQuery,
		})
		updateViewportContent(&m)
	}

	p := tea.NewProgram(m)

	go func() {
		for ev := range taskChan {
			p.Send(ev)
		}
	}()

	presenter := NewTUIPresenter(p)
	loop.UI = presenter

	go func() {
		if initialQuery != "" {
			ctx, cancel := context.WithCancel(context.Background())
			p.Send(AgentCancelInitMsg{Cancel: cancel})
			if cfg.Stream {
				loop.RunAgentCycleStream(ctx, initialQuery)
			} else {
				loop.RunAgentCycle(ctx, initialQuery)
			}
			p.Send(AgentFinishedMsg{})
		}

		for {
			agentPrompt, ok := <-promptChan
			if !ok {
				break
			}
			if agentPrompt.Prompt == "/exit" {
				break
			}
			if agentPrompt.Prompt == "/reset" {
				loop.ResetHistory()
				p.Send(AgentMessageMsg{
					Role:    "system",
					Content: "Conversation memory and history cleared successfully.",
				})
				p.Send(AgentFinishedMsg{})
				continue
			}
			if cfg.Stream {
				loop.RunAgentCycleStream(agentPrompt.Ctx, agentPrompt.Prompt)
			} else {
				loop.RunAgentCycle(agentPrompt.Ctx, agentPrompt.Prompt)
			}
			p.Send(AgentFinishedMsg{})
		}
	}()

	_, err := p.Run()

	if m.GatewayMgr != nil {
		m.GatewayMgr.StopAll()
	}

	CloseAllSessionDB()
	select {
	case promptChan <- AgentPrompt{Prompt: "/exit"}:
	default:
	}
	return err
}

var lastSessionSave time.Time

func saveModelSession(m *Model) {
	if m.Loop == nil {
		return
	}
	if len(m.Messages) == 0 {
		return
	}

	if m.IsStreaming {
		return
	}

	if time.Since(lastSessionSave) < 3*time.Second {
		return
	}
	lastSessionSave = time.Now()

	title := m.ActiveSessionTitle

	createdAt := m.ActiveSessionCreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	activeProfile := provider.NewManager("").ActiveProfile

	s := &Session{
		ID:         m.ActiveSessionID,
		Title:      title,
		WorkDir:    m.Cfg.WorkDir,
		Provider:   activeProfile,
		Model:      m.Cfg.Model,
		Mode:       m.Cfg.Mode,
		CreatedAt:  createdAt,
		Messages:   m.Messages,
		History:    m.Loop.GetHistory(),
		TokenCount: m.TokenCount,
		CompressedTurns: m.CompressedTurns,
	}

	err := SaveSession(s, m.LastSavedSeq)
	if err == nil {
		m.LastSavedSeq = len(m.Messages) - 1
		if m.ActiveSessionTitle != s.Title {
			m.ActiveSessionTitle = s.Title
		}
		if m.ActiveSessionCreatedAt.IsZero() {
			m.ActiveSessionCreatedAt = createdAt
		}
	}
}
