package telegram

import (
	"awas/internal/config"
	"awas/internal/gateway"
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func init() {
	gateway.RegisterAdapter("telegram", func(p gateway.Platform, cfg *config.Config) gateway.Gateway {
		return &TelegramGateway{
			config: p,
			cfg:    cfg,
			users:  make(map[string]*gateway.UserSession),
			msgChs: make(map[string]chan pendingMsg),
		}
	})
}

type TelegramGateway struct {
	config gateway.Platform
	cfg    *config.Config
	bot    *tgbotapi.BotAPI
	users  map[string]*gateway.UserSession 
	msgChs map[string]chan pendingMsg       
	mu     sync.RWMutex
	cancel context.CancelFunc
}

type pendingMsg struct {
	text string
	ui   *TelegramUI
}

const processTimeout = 300 * time.Second

func (tg *TelegramGateway) Name() string { return "telegram" }

func (tg *TelegramGateway) Status() gateway.GatewayStatus {
	tg.mu.RLock()
	defer tg.mu.RUnlock()

	running := tg.bot != nil
	info := "Not connected"
	if running && tg.bot.Self.UserName != "" {
		info = fmt.Sprintf("Connected as @%s (%d users)", tg.bot.Self.UserName, len(tg.users))
	}
	return gateway.GatewayStatus{
		Running:  running,
		Platform: "telegram",
		Info:     info,
	}
}

func (tg *TelegramGateway) Start(ctx context.Context, mgr *gateway.Manager) error {
	bot, err := tgbotapi.NewBotAPI(tg.config.Token)
	if err != nil {
		return fmt.Errorf("telegram: invalid bot token: %v", err)
	}

	tg.mu.Lock()
	tg.bot = bot
	tg.mu.Unlock()
	log.Printf("Telegram bot authorized: @%s", bot.Self.UserName)

	tg.registerCommands()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			return nil
		case update, ok := <-updates:
			if !ok {
				return nil
			}
			tg.handleUpdate(update, mgr)
		}
	}
}

func (tg *TelegramGateway) Stop() error {
	tg.mu.Lock()
	defer tg.mu.Unlock()

	if tg.bot != nil {
		tg.bot = nil
	}

	for _, session := range tg.users {
		if session.Cancel != nil {
			session.Cancel()
		}
	}

	for _, ch := range tg.msgChs {
		if ch != nil {
			close(ch)
		}
	}

	tg.users = make(map[string]*gateway.UserSession)
	tg.msgChs = make(map[string]chan pendingMsg)
	return nil
}

func (tg *TelegramGateway) getBot() *tgbotapi.BotAPI {
	tg.mu.RLock()
	defer tg.mu.RUnlock()
	return tg.bot
}

func (tg *TelegramGateway) SendText(chatID int64, text string) {
	tg.sendText(chatID, text)
}

func (tg *TelegramGateway) getSession(chatID int64, displayName string, mgr *gateway.Manager) *gateway.UserSession {
	tg.mu.Lock()
	defer tg.mu.Unlock()

	key := fmt.Sprintf("%d", chatID)
	if session, ok := tg.users[key]; ok {
		session.UpdateActivity()
		return session
	}

	if len(tg.config.AllowedUsers) > 0 {
		allowed := false
		for _, id := range tg.config.AllowedUsers {
			if id == key {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil
		}
	}

	if tg.config.MaxUsers > 0 && len(tg.users) >= tg.config.MaxUsers {
		return nil
	}

	session := gateway.CreateUserSession(key, displayName, "telegram", tg.cfg)
	tg.users[key] = session
	return session
}

func (tg *TelegramGateway) removeSession(chatID int64) {
	tg.mu.Lock()
	defer tg.mu.Unlock()

	key := fmt.Sprintf("%d", chatID)
	if session, ok := tg.users[key]; ok {
		if session.Cancel != nil {
			session.Cancel()
		}
		delete(tg.users, key)
	}
	if ch, ok := tg.msgChs[key]; ok {
		close(ch)
		delete(tg.msgChs, key)
	}
}

func (tg *TelegramGateway) ensureProcessor(key string, session *gateway.UserSession) {
	if _, ok := tg.msgChs[key]; ok {
		return 
	}

	ch := make(chan pendingMsg, 10)
	tg.msgChs[key] = ch

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[telegram] processor panic for %s: %v (resetting)", key, r)
				tg.mu.Lock()
				session.IsRunning = false
				session.ActiveCancel = nil
				delete(tg.msgChs, key)
				tg.mu.Unlock()
			}
		}()
		for msg := range ch {
			if session.Ctx.Err() != nil {
				log.Printf("[telegram] skip message for cancelled session %s", key)
				continue
			}

			session.Loop.UI = msg.ui

			ctx, cancel := context.WithTimeout(session.Ctx, processTimeout)
			ctx = context.WithValue(ctx, "platform", "telegram")
			ctx = context.WithValue(ctx, "chat_id", key)

			tg.mu.Lock()
			session.IsRunning = true
			session.ActiveCancel = cancel
			tg.mu.Unlock()

			if tg.cfg.Stream {
				session.Loop.RunAgentCycleStream(ctx, msg.text)
			} else {
				session.Loop.RunAgentCycle(ctx, msg.text)
			}

			tg.mu.Lock()
			session.IsRunning = false
			session.ActiveCancel = nil
			tg.mu.Unlock()

			cancel()

			session.SaveSession(tg.cfg)
		}
	}()
}

func (tg *TelegramGateway) registerCommands() {
	bot := tg.getBot()
	if bot == nil {
		return
	}

	commands := []tgbotapi.BotCommand{
		{Command: "start", Description: "Start the bot and show welcome message"},
		{Command: "reset", Description: "Reset conversation memory"},
		{Command: "status", Description: "Show current session info"},
		{Command: "yes", Description: "Approve tool execution"},
		{Command: "no", Description: "Reject tool execution"},
		{Command: "continue", Description: "Continue tool chain"},
		{Command: "stop", Description: "Stop tool chain"},
		{Command: "cron", Description: "Manage cron/scheduled jobs"},
		{Command: "mode", Description: "Switch agent execution mode (chat, simple, planned, deep)"},
	}

	config := tgbotapi.NewSetMyCommands(commands...)
	sendBot(bot, config)
}
