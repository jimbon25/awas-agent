package telegram

import (
	"awas/internal/gateway"
	"awas/internal/tools"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (tg *TelegramGateway) handleUpdate(update tgbotapi.Update, mgr *gateway.Manager) {
	if update.CallbackQuery != nil {
		tg.handleCallbackQuery(update.CallbackQuery, mgr)
		return
	}

	if update.Message != nil {
		tg.handleMessage(update.Message, mgr)
		return
	}
}

func (tg *TelegramGateway) handleMessage(msg *tgbotapi.Message, mgr *gateway.Manager) {
	chatID := msg.Chat.ID
	displayName := msg.From.FirstName
	if displayName == "" {
		displayName = msg.From.UserName
	}
	if displayName == "" {
		displayName = fmt.Sprintf("%d", msg.From.ID)
	}

	text := strings.TrimSpace(msg.Text)
	if text == "" && msg.Caption != "" {
		text = strings.TrimSpace(msg.Caption)
	}

	key := fmt.Sprintf("%d", chatID)
	tg.mu.RLock()
	session, hasSession := tg.users[key]
	tg.mu.RUnlock()
	if hasSession && session.Loop.UI != nil {
		if telUI, ok := session.Loop.UI.(*TelegramUI); ok && telUI.askChan != nil {
			telUI.askChan <- text
			return
		}
	}

	switch {
	case text == "/start":
		tg.sendText(chatID, "Hello! I'm AWAS, your AI coding assistant.\n\nSend me any message and I'll help you with your code.\n\nCommands:\n/start — Show this message\n/reset — Reset conversation\n/status — Show session info")
		return
	case text == "/reset":
		key := fmt.Sprintf("%d", chatID)
		sessionID := fmt.Sprintf("gw-telegram-%s", key)
		gateway.DeleteSession(sessionID)
		tg.removeSession(chatID)
		tg.sendText(chatID, "✔ Conversation reset. Send me a new message to start.")
		return
	case text == "/status":
		key := fmt.Sprintf("%d", chatID)
		tg.mu.RLock()
		session, ok := tg.users[key]
		tg.mu.RUnlock()
		if ok {
			history := session.Loop.GetHistory()
			charCount := 0
			for _, m := range history {
				charCount += len(m.Content)
			}
			estTokens := charCount / 4
			statusStr := "Idle ⧗"
			if session.IsRunning {
				statusStr = "Running ❯"
			}
			loopCfg := session.Loop.GetConfig()

			reply := fmt.Sprintf("⎔ <b>Session Status Info</b>\n"+
				"──────────────────────────\n"+
				"• <b>Session ID</b>: <code>%s</code>\n"+
				"• <b>Status</b>: <code>%s</code>\n"+
				"• <b>Active Mode</b>: <code>%s</code> (Switch via /mode)\n"+
				"• <b>Active Model</b>: <code>%s</code>\n"+
				"• <b>Work Directory</b>: <code>%s</code>\n"+
				"• <b>Conversation</b>: %d messages\n"+
				"• <b>Token Usage</b>: ~%d tokens\n"+
				"• <b>Last Active</b>: %s",
				escapeHTML(session.SessionID),
				escapeHTML(statusStr),
				escapeHTML(loopCfg.AgentMode),
				escapeHTML(loopCfg.Model),
				escapeHTML(loopCfg.WorkDir),
				len(history),
				estTokens,
				session.LastActive.Format("2006-01-02 15:04:05"),
			)
			tg.sendText(chatID, reply)
		} else {
			tg.sendText(chatID, "✘ No active session.")
		}
		return

	case strings.HasPrefix(text, "/mode"):
		key := fmt.Sprintf("%d", chatID)
		tg.mu.RLock()
		session, ok := tg.users[key]
		tg.mu.RUnlock()
		if !ok {
			tg.sendText(chatID, "✘ No active session.")
			return
		}

		modeArg := ""
		if len(text) > len("/mode") {
			modeArg = strings.TrimSpace(text[len("/mode"):])
		}

		if modeArg == "" {
			tg.sendText(chatID, fmt.Sprintf("ℹ Current session mode: <b>%s</b>", escapeHTML(session.Loop.GetConfig().AgentMode)))
			return
		}

		switch modeArg {
		case "chat", "simple", "planned", "deep":
			session.Loop.GetConfig().AgentMode = modeArg
			session.SaveSession(tg.cfg)
			tg.sendText(chatID, fmt.Sprintf("✔ Mode switched to <b>%s</b> successfully for this session.", escapeHTML(modeArg)))
		default:
			tg.sendText(chatID, "✘ Invalid mode. Use: /mode chat, simple, planned, or deep.")
		}
		return
	case text == "/yes" || text == "/no":
		tg.handleApprovalReply(chatID, text == "/yes")
		return
	case text == "/continue":
		tg.handleChainReply(chatID, true)
		return
	case text == "/stop":
		key := fmt.Sprintf("%d", chatID)
		tg.mu.RLock()
		session, ok := tg.users[key]
		tg.mu.RUnlock()

		if ok && session.IsRunning && session.ActiveCancel != nil {
			session.ActiveCancel()
			tg.sendText(chatID, "⏹ Aborting active execution...")
		} else {
			tg.handleChainReply(chatID, false)
		}
		return
	case strings.HasPrefix(text, "/cron") ||
		strings.HasPrefix(strings.ToLower(text), "jadwalin") ||
		strings.HasPrefix(strings.ToLower(text), "buat cron") ||
		strings.HasPrefix(strings.ToLower(text), "buat jadwal") ||
		strings.HasPrefix(strings.ToLower(text), "schedule") ||
		strings.HasPrefix(strings.ToLower(text), "tambahkan cron"):

		var args []string
		if strings.HasPrefix(text, "/cron") {
			if len(text) > len("/cron") {
				args = strings.Fields(text[len("/cron"):])
			}
		} else {
			args = []string{text}
		}

		response := gateway.HandleCronCommand(mgr.CronStore, "telegram", fmt.Sprintf("%d", chatID), "", args)
		tg.sendText(chatID, response)
		return
	}

	session = tg.getSession(chatID, displayName, mgr)
	if session == nil {
		tg.sendText(chatID, "⚠ Access denied or max users reached.")
		return
	}

	var fileID string
	var fileName string

	if msg.Document != nil {
		fileID = msg.Document.FileID
		fileName = msg.Document.FileName
	} else if len(msg.Photo) > 0 {
		largestPhoto := msg.Photo[len(msg.Photo)-1]
		fileID = largestPhoto.FileID
		fileName = fmt.Sprintf("photo_%d.jpg", time.Now().Unix())
	}

	if fileID != "" {
		fileURL, err := tg.bot.GetFileDirectURL(fileID)
		if err == nil {
			downloadsDir := filepath.Join(session.Loop.GetConfig().WorkDir, "downloads")
			os.MkdirAll(downloadsDir, 0755)
			destPath := filepath.Join(downloadsDir, fileName)
			tools.DownloadFile(fileURL, destPath)

			text = fmt.Sprintf("[System Notification: User uploaded file '%s' and saved to 'downloads/%s']\n%s", fileName, fileName, text)
		}
	}

	bot := tg.getBot()
	if bot == nil {
		return
	}

	ui := NewTelegramUI(bot, chatID, tg)

	tg.mu.Lock()
	key = fmt.Sprintf("%d", chatID)
	tg.ensureProcessor(key, session)
	ch := tg.msgChs[key]
	tg.mu.Unlock()

	select {
	case ch <- pendingMsg{text: text, ui: ui}:
	default:
		tg.sendText(chatID, "⧗ Still processing previous message. Please wait...")
	}
}

func (tg *TelegramGateway) handleCallbackQuery(query *tgbotapi.CallbackQuery, mgr *gateway.Manager) {
	bot := tg.getBot()
	if bot == nil {
		return
	}

	data := query.Data
	chatID := query.Message.Chat.ID

	callback := tgbotapi.NewCallback(query.ID, "")
	bot.Request(callback)

	edit := tgbotapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, query.Message.Text)
	edit.ParseMode = "HTML"
	edit.ReplyMarkup = nil
	sendBot(bot, edit)

	switch {
	case data == "approve":
		tg.handleApprovalReply(chatID, true)
	case data == "reject":
		tg.handleApprovalReply(chatID, false)
	case data == "continue_yes":
		tg.handleChainReply(chatID, true)
	case data == "continue_no":
		tg.handleChainReply(chatID, false)
	}
}

func (tg *TelegramGateway) handleApprovalReply(chatID int64, approved bool) {
	key := fmt.Sprintf("%d", chatID)
	tg.mu.RLock()
	session, ok := tg.users[key]
	tg.mu.RUnlock()

	if !ok {
		return
	}

	if ui, ok := session.Loop.UI.(*TelegramUI); ok {
		ui.SendApprovalResponse(approved)
	}
}

func (tg *TelegramGateway) handleChainReply(chatID int64, continueChain bool) {
	key := fmt.Sprintf("%d", chatID)
	tg.mu.RLock()
	session, ok := tg.users[key]
	tg.mu.RUnlock()

	if !ok {
		return
	}

	if ui, ok := session.Loop.UI.(*TelegramUI); ok {
		ui.SendChainResponse(continueChain)
	}
}

func (tg *TelegramGateway) sendText(chatID int64, text string) {
	bot := tg.getBot()
	if bot == nil {
		return
	}
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	sendBot(bot, msg)
}

var _ gateway.Gateway = (*TelegramGateway)(nil)
