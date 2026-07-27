package telegram

import (
	"awas/internal/agent"
	"awas/internal/client"
	"awas/internal/gateway"
	"awas/internal/provider"
	"awas/internal/tools"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	case text == "/help":
		tg.sendText(chatID, "📋 <b>Available Commands</b>\n\n"+
			"/help — Show this message\n"+
			"/reset — Reset conversation memory\n"+
			"/status — Show session info\n"+
			"/mode — Switch mode (chat/simple/planned/deep)\n"+
			"/model — View/switch provider profiles and models\n"+
			"/tokens — Show token usage\n"+
			"/cron — Manage scheduled jobs\n"+
			"/yes — Approve tool execution\n"+
			"/no — Reject tool execution\n"+
			"/continue — Continue tool chain\n"+
			"/stop — Stop tool chain")
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

			subagents := agent.GetSubagentRegistry().List()
			subagentStr := "None"
			activeSubCount := 0
			activeRole := ""
			activeID := ""
			for _, s := range subagents {
				if s.Status == agent.SubagentStatusRunning {
					activeSubCount++
					activeRole = s.Role
					activeID = s.ID
				}
			}
			if activeSubCount == 1 {
				subagentStr = fmt.Sprintf("%s (%s)", activeRole, activeID)
			} else if activeSubCount > 1 {
				subagentStr = fmt.Sprintf("%d running", activeSubCount)
			}

			reply := fmt.Sprintf("⎔ <b>Session Status Info</b>\n\n"+
				"• <b>Session ID</b>: <code>%s</code>\n"+
				"• <b>Status</b>: <code>%s</code>\n"+
				"• <b>Active Subagents</b>: <code>%s</code>\n"+
				"• <b>Active Mode</b>: <code>%s</code> (Switch via /mode)\n"+
				"• <b>Active Model</b>: <code>%s</code>\n"+
				"• <b>Work Directory</b>: <code>%s</code>\n"+
				"• <b>Conversation</b>: %d messages\n"+
				"• <b>Token Usage</b>: ~%d tokens\n"+
				"• <b>Last Active</b>: %s",
				escapeHTML(session.SessionID),
				escapeHTML(statusStr),
				escapeHTML(subagentStr),
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

	case text == "/mode" || strings.HasPrefix(text, "/mode "):
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
	case strings.HasPrefix(text, "/model"):
		key := fmt.Sprintf("%d", chatID)
		tg.mu.RLock()
		session, ok := tg.users[key]
		tg.mu.RUnlock()

		modelArg := ""
		if len(text) > len("/model") {
			modelArg = strings.TrimSpace(text[len("/model"):])
		}

		mgr := provider.NewManager("")

		if modelArg == "" {
			// Show all profiles as inline keyboard buttons
			var rows [][]tgbotapi.InlineKeyboardButton
			profileNames := make([]string, 0, len(mgr.Profiles))
			for name := range mgr.Profiles {
				profileNames = append(profileNames, name)
			}
			sort.Strings(profileNames)

			for _, name := range profileNames {
				p := mgr.Profiles[name]
				label := fmt.Sprintf("%s — %s (%s)", name, p.Model, p.Name)
				if name == mgr.ActiveProfile {
					label = "▶ " + label
				}
				btn := tgbotapi.NewInlineKeyboardButtonData(label, "model_select:"+name)
				rows = append(rows, tgbotapi.NewInlineKeyboardRow(btn))
			}

			if len(rows) == 0 {
				tg.sendText(chatID, "✘ No provider profiles found. Run /setup to configure one.")
				return
			}

			keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
			msg := tgbotapi.NewMessage(chatID, "📋 <b>Select Provider Profile:</b>")
			msg.ParseMode = "HTML"
			msg.ReplyMarkup = keyboard
			sendBot(tg.getBot(), msg)
			return
		}

		if !ok {
			tg.sendText(chatID, "✘ No active session.")
			return
		}

		if p, ok := mgr.Profiles[modelArg]; ok {
			// Switch profile
			mgr.ActiveProfile = modelArg
			mgr.Save()
			session.Loop.GetConfig().Model = p.Model
			session.Loop.GetConfig().Endpoint = p.GetEndpoint()
			session.Loop.GetConfig().APIKey = p.GetAPIKey()
			session.Loop.SetClient(client.New(p))
			session.SaveSession(tg.cfg)
			tg.sendText(chatID, fmt.Sprintf("✔ Switched to profile: <b>%s</b> (Model: <b>%s</b>)", escapeHTML(modelArg), escapeHTML(p.Model)))
		} else {
			// Treat as model name — update current profile's model
			session.Loop.GetConfig().Model = modelArg
			if p, ok := mgr.Profiles[mgr.ActiveProfile]; ok {
				p.Model = modelArg
				mgr.Save()
				session.Loop.SetClient(client.New(p))
			}
			session.SaveSession(tg.cfg)
			tg.sendText(chatID, fmt.Sprintf("✔ Model changed to: <b>%s</b>", escapeHTML(modelArg)))
		}
		return
	case strings.HasPrefix(text, "/tokens"):
		key := fmt.Sprintf("%d", chatID)
		tg.mu.RLock()
		session, ok := tg.users[key]
		tg.mu.RUnlock()
		if !ok {
			tg.sendText(chatID, "✘ No active session.")
			return
		}

		history := session.Loop.GetHistory()
		tokenCount := agent.EstimateTotalTokens(history)
		tokenMax := session.Loop.GetConfig().MaxTokens
		if tokenMax <= 0 {
			tokenMax = 1
		}

		tg.sendText(chatID, fmt.Sprintf("📊 %s / %s tokens",
			escapeHTML(formatGatewayTokens(tokenCount)),
			escapeHTML(formatGatewayTokens(tokenMax))))
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

	editMarkup := tgbotapi.NewEditMessageReplyMarkup(query.Message.Chat.ID, query.Message.MessageID, tgbotapi.InlineKeyboardMarkup{InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{}})
	sendBot(bot, editMarkup)

	switch {
	case data == "approve":
		tg.handleApprovalReply(chatID, true)
	case data == "reject":
		tg.handleApprovalReply(chatID, false)
	case data == "continue_yes":
		tg.handleChainReply(chatID, true)
	case data == "continue_no":
		tg.handleChainReply(chatID, false)
	case strings.HasPrefix(data, "model_select:"):
		profileName := strings.TrimPrefix(data, "model_select:")
		tg.handleModelSelect(chatID, profileName)
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

func (tg *TelegramGateway) handleModelSelect(chatID int64, profileName string) {
	key := fmt.Sprintf("%d", chatID)
	tg.mu.RLock()
	session, ok := tg.users[key]
	tg.mu.RUnlock()

	if !ok {
		tg.sendText(chatID, "✘ No active session.")
		return
	}

	mgr := provider.NewManager("")
	p, ok := mgr.Profiles[profileName]
	if !ok {
		tg.sendText(chatID, fmt.Sprintf("✘ Profile <b>%s</b> not found.", escapeHTML(profileName)))
		return
	}

	mgr.ActiveProfile = profileName
	mgr.Save()
	session.Loop.GetConfig().Model = p.Model
	session.Loop.GetConfig().Endpoint = p.GetEndpoint()
	session.Loop.GetConfig().APIKey = p.GetAPIKey()
	session.Loop.SetClient(client.New(p))
	session.SaveSession(tg.cfg)

	tg.sendText(chatID, fmt.Sprintf("✔ Switched to profile: <b>%s</b> (Model: <b>%s</b>)", escapeHTML(profileName), escapeHTML(p.Model)))
}

func formatGatewayTokens(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var res []string
	for len(s) > 3 {
		res = append([]string{s[len(s)-3:]}, res...)
		s = s[:len(s)-3]
	}
	if len(s) > 0 {
		res = append([]string{s}, res...)
	}
	return strings.Join(res, ",")
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
