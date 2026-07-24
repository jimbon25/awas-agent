package discord

import (
	"awas/internal/config"
	"awas/internal/gateway"
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

func init() {
	gateway.RegisterAdapter("discord", func(p gateway.Platform, cfg *config.Config) gateway.Gateway {
		return &DiscordGateway{
			config: p,
			cfg:    cfg,
			users:  make(map[string]*gateway.UserSession),
			msgChs: make(map[string]chan pendingMsg),
		}
	})
}

type DiscordGateway struct {
	config  gateway.Platform
	cfg     *config.Config
	session *discordgo.Session
	users   map[string]*gateway.UserSession 
	msgChs  map[string]chan pendingMsg      
	mu      sync.RWMutex
	cancel  context.CancelFunc
}

type pendingMsg struct {
	text string
	ui   *DiscordUI
}

const processTimeout = 300 * time.Second

func (dg *DiscordGateway) Name() string {
	return "discord"
}

func (dg *DiscordGateway) Status() gateway.GatewayStatus {
	dg.mu.RLock()
	defer dg.mu.RUnlock()

	running := dg.session != nil
	info := "Not connected"
	if running && dg.session.State != nil && dg.session.State.User != nil {
		info = fmt.Sprintf("Connected as %s#%s (%d active threads)",
			dg.session.State.User.Username,
			dg.session.State.User.Discriminator,
			len(dg.users),
		)
	}

	return gateway.GatewayStatus{
		Running:  running,
		Platform: "discord",
		Info:     info,
	}
}

func (dg *DiscordGateway) Start(ctx context.Context, mgr *gateway.Manager) error {
	session, err := discordgo.New("Bot " + dg.config.Token)
	if err != nil {
		return fmt.Errorf("failed to create discord session: %v", err)
	}

	dg.mu.Lock()
	dg.session = session
	dg.mu.Unlock()

	session.AddHandler(dg.OnReady)
	session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		dg.OnMessageCreate(s, m, mgr)
	})
	session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		dg.OnInteractionCreate(s, i, mgr)
	})

	err = session.Open()
	if err != nil {
		dg.mu.Lock()
		dg.session = nil
		dg.mu.Unlock()
		return fmt.Errorf("failed to open discord gateway connection: %v", err)
	}

	log.Printf("[discord] Bot connection opened successfully")

	dg.registerCommands()

	<-ctx.Done()

	return dg.Stop()
}

func (dg *DiscordGateway) Stop() error {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	if dg.session != nil {
		dg.session.Close()
		dg.session = nil
	}

	for _, s := range dg.users {
		if s.Cancel != nil {
			s.Cancel()
		}
	}

	for _, ch := range dg.msgChs {
		if ch != nil {
			close(ch)
		}
	}

	dg.users = make(map[string]*gateway.UserSession)
	dg.msgChs = make(map[string]chan pendingMsg)

	log.Printf("[discord] Bot connection stopped")
	return nil
}

func (dg *DiscordGateway) getDiscordSession() *discordgo.Session {
	dg.mu.RLock()
	defer dg.mu.RUnlock()
	return dg.session
}

func (dg *DiscordGateway) SendText(channelID string, text string) {
	session := dg.getDiscordSession()
	if session == nil {
		log.Printf("[discord] Cannot send text: session not initialized")
		return
	}

	const maxLen = 1900
	if len(text) <= maxLen {
		session.ChannelMessageSend(channelID, text)
		return
	}

	for len(text) > 0 {
		end := maxLen
		if end > len(text) {
			end = len(text)
		}

		if end < len(text) {
			if idx := strings.LastIndex(text[:end], "\n"); idx > maxLen/2 {
				end = idx + 1
			}
		}

		chunk := text[:end]
		text = text[end:]

		session.ChannelMessageSend(channelID, chunk)
	}
}

func (dg *DiscordGateway) getSession(threadID string, displayName string, mgr *gateway.Manager) *gateway.UserSession {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	if s, ok := dg.users[threadID]; ok {
		s.UpdateActivity()
		return s
	}

	if dg.config.MaxUsers > 0 && len(dg.users) >= dg.config.MaxUsers {
		return nil
	}

	s := gateway.CreateUserSession(threadID, displayName, "discord", dg.cfg)
	dg.users[threadID] = s

	return s
}

func (dg *DiscordGateway) removeSession(threadID string) {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	if s, ok := dg.users[threadID]; ok {
		if s.Cancel != nil {
			s.Cancel()
		}
		delete(dg.users, threadID)
	}

	if ch, ok := dg.msgChs[threadID]; ok {
		close(ch)
		delete(dg.msgChs, threadID)
	}
}

func (dg *DiscordGateway) ensureProcessor(threadID string, session *gateway.UserSession) {
	if dg.msgChs[threadID] != nil {
		return 
	}

	ch := make(chan pendingMsg, 10)
	dg.msgChs[threadID] = ch

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[discord] Processor panic for thread %s: %v", threadID, r)
			}
			dg.mu.Lock()
			session.IsRunning = false
			session.ActiveCancel = nil
			delete(dg.msgChs, threadID)
			dg.mu.Unlock()
		}()

		for msg := range ch {
			if session.Ctx.Err() != nil {
				log.Printf("[discord] Session context cancelled, skipping message for thread %s", threadID)
				continue
			}

			session.Loop.UI = msg.ui

			ctx, cancel := context.WithTimeout(session.Ctx, processTimeout)
			ctx = context.WithValue(ctx, "platform", "discord")
			ctx = context.WithValue(ctx, "chat_id", threadID)

			guildID := ""
			if dg.config.Extra != nil {
				guildID = dg.config.Extra["guild_id"]
			}
			ctx = context.WithValue(ctx, "guild_id", guildID)

			dg.mu.Lock()
			session.IsRunning = true
			session.ActiveCancel = cancel
			dg.mu.Unlock()

			if dg.cfg.Stream {
				session.Loop.RunAgentCycleStream(ctx, msg.text)
			} else {
				session.Loop.RunAgentCycle(ctx, msg.text)
			}

			dg.mu.Lock()
			session.IsRunning = false
			session.ActiveCancel = nil
			dg.mu.Unlock()

			cancel()

			session.SaveSession(dg.cfg)
		}
	}()
}

func (dg *DiscordGateway) registerCommands() {
	session := dg.getDiscordSession()
	if session == nil {
		return
	}

	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "status",
			Description: "Show the status of the current AWAS session",
		},
		{
			Name:        "reset",
			Description: "Reset the current AWAS conversation thread session",
		},
		{
			Name:        "yes",
			Description: "Approve a pending tool execution",
		},
		{
			Name:        "no",
			Description: "Reject a pending tool execution",
		},
		{
			Name:        "continue",
			Description: "Continue the tool execution chain",
		},
		{
			Name:        "stop",
			Description: "Stop the tool execution chain",
		},
		{
			Name:        "cron",
			Description: "Manage cron/scheduled jobs",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "command",
					Description: "Cron command (e.g. list, delete my-job, or create \"every 30m\" \"prompt\")",
					Required:    false,
				},
			},
		},
		{
			Name:        "mode",
			Description: "Switch agent execution mode for this thread",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "type",
					Description: "Select mode: chat, simple, planned, or deep",
					Required:    false,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "Chat Mode (General)", Value: "chat"},
						{Name: "Simple Mode (Coding)", Value: "simple"},
						{Name: "Planned Mode (Steps)", Value: "planned"},
						{Name: "Deep Mode (Review)", Value: "deep"},
					},
				},
			},
		},
	}

	guildID := ""
	if dg.config.Extra != nil {
		guildID = dg.config.Extra["guild_id"]
	}

	for _, cmd := range commands {
		_, err := session.ApplicationCommandCreate(session.State.User.ID, guildID, cmd)
		if err != nil {
			log.Printf("[discord] Cannot create command %q: %v", cmd.Name, err)
		}
	}
	log.Printf("[discord] Commands registered successfully")
}
