package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"
	"path/filepath"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func sendBot(bot *tgbotapi.BotAPI, msg tgbotapi.Chattable) {
	switch msg.(type) {
	case tgbotapi.ChatActionConfig, *tgbotapi.ChatActionConfig,
		tgbotapi.SetMyCommandsConfig, *tgbotapi.SetMyCommandsConfig:
		if _, err := bot.Request(msg); err != nil {
			log.Printf("[telegram] sendBot request error: %v", err)
		}
		return
	}

	if _, err := bot.Send(msg); err != nil {
		log.Printf("[telegram] sendBot error: %v", err)
	}
}

func sendBotWithResult(bot *tgbotapi.BotAPI, msg tgbotapi.Chattable) (tgbotapi.Message, error) {
	return bot.Send(msg)
}

type TelegramUI struct {
	bot    *tgbotapi.BotAPI
	chatID int64
	gw     *TelegramGateway

	approvalChan  chan bool
	approvalMsgID int 

	chainChan  chan bool
	chainMsgID int

	askChan chan string

	planMsgID          int
	activeGoal         string
	activeSteps        []string
	activeStepStatuses map[string]string

	activeMsgID  int
	editCount    int
	streamBuffer string
	lastEditTime time.Time
}

func NewTelegramUI(bot *tgbotapi.BotAPI, chatID int64, gw *TelegramGateway) *TelegramUI {
	return &TelegramUI{
		bot:    bot,
		chatID: chatID,
		gw:     gw,
	}
}

func (u *TelegramUI) SendApprovalResponse(approved bool) {
	if u.approvalChan != nil {
		select {
		case u.approvalChan <- approved:
		default:
		}
	}
}

func (u *TelegramUI) SendChainResponse(continueChain bool) {
	if u.chainChan != nil {
		select {
		case u.chainChan <- continueChain:
		default:
		}
	}
}

func (u *TelegramUI) startTypingIndicator() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 28*time.Second)
		defer cancel()
		action := tgbotapi.NewChatAction(u.chatID, tgbotapi.ChatTyping)
		sendBot(u.bot, action)
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				sendBot(u.bot, action)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (u *TelegramUI) PrintThinking(model string) {
	u.activeMsgID = 0
	u.editCount = 0
	u.streamBuffer = ""
	u.startTypingIndicator()
}

func (u *TelegramUI) PrintMessage(role string, content string) {
	if role == "assistant" && content != "" {
		u.streamBuffer = content
		u.sendLongMessage(markdownToHTML(content))
	} else if role == "system" {
		msg := tgbotapi.NewMessage(u.chatID, "ℹ "+escapeHTML(content))
		msg.ParseMode = "HTML"
		sendBot(u.bot, msg)
	}
}

func (u *TelegramUI) SendFile(filePath string, caption string) error {
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" {
		photo := tgbotapi.NewPhoto(u.chatID, tgbotapi.FilePath(filePath))
		photo.Caption = caption
		_, err := u.bot.Send(photo)
		return err
	}
	doc := tgbotapi.NewDocument(u.chatID, tgbotapi.FilePath(filePath))
	doc.Caption = caption
	_, err := u.bot.Send(doc)
	return err
}

func (u *TelegramUI) PrintMessageDelta(content string) {
	u.streamBuffer += content
	if time.Since(u.lastEditTime) >= 1200*time.Millisecond {
		u.lastEditTime = time.Now()
		u.sendLongMessage(markdownToHTML(u.streamBuffer))
	}
}

func (u *TelegramUI) PrintToolCall(name string, args string) {
	summary := summarizeArgs(name, args)
	toolHtml := fmt.Sprintf("\n<pre>▸ %s\n  %s</pre>", escapeHTML(name), escapeHTML(summary))
	u.streamBuffer += toolHtml
	u.lastEditTime = time.Now()
	u.sendLongMessage(u.streamBuffer)
}

func (u *TelegramUI) PrintToolResult(name string, result string) {
	success := !strings.HasPrefix(result, "[Error]")
	icon := "✓"
	if !success {
		icon = "✗"
	}
	displayRes := result
	displayRes = strings.ReplaceAll(displayRes, "\n", "\n  ")
	if len(displayRes) > 300 {
		displayRes = displayRes[:300] + "\n  ..."
	}
	resHtml := fmt.Sprintf("\n<pre>  %s %s\n  %s</pre>", icon, escapeHTML(name), escapeHTML(displayRes))
	u.streamBuffer += resHtml
	u.lastEditTime = time.Now()
	u.sendLongMessage(u.streamBuffer)
}

func summarizeArgs(name, args string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(args), &m); err != nil {
		if len(args) > 60 {
			return args[:60] + "..."
		}
		return args
	}

	switch name {
	case "execute_command":
		cmd, _ := m["command"].(string)
		return cmd
	case "read_file", "delete_file", "restore_file":
		p, _ := m["path"].(string)
		return p
	case "write_file":
		p, _ := m["path"].(string)
		content, _ := m["content"].(string)
		lines := strings.Count(content, "\n") + 1
		return fmt.Sprintf("%s (%d lines)", p, lines)
	case "edit_file":
		p, _ := m["file_path"].(string)
		old, _ := m["old_string"].(string)
		if len(old) > 30 {
			old = old[:30] + "..."
		}
		return fmt.Sprintf("%s → %s", p, old)
	case "search_code":
		pattern, _ := m["pattern"].(string)
		path, _ := m["path"].(string)
		if path != "" {
			return fmt.Sprintf("%s in %s/", pattern, path)
		}
		return pattern
	case "list_directory":
		p, _ := m["path"].(string)
		return p + "/"
	case "search_symbols":
		query, _ := m["query"].(string)
		return query
	case "http_request":
		method, _ := m["method"].(string)
		url, _ := m["url"].(string)
		return fmt.Sprintf("%s %s", strings.ToUpper(method), url)
	case "web_search":
		query, _ := m["query"].(string)
		return fmt.Sprintf(`"%s"`, query)
	case "download_file":
		url, _ := m["url"].(string)
		path, _ := m["path"].(string)
		return fmt.Sprintf("%s → %s", url, path)
	case "sql_query":
		query, _ := m["query"].(string)
		if len(query) > 50 {
			query = query[:50] + "..."
		}
		return query
	default:
		if len(args) > 60 {
			return args[:60] + "..."
		}
		return args
	}
}

func (u *TelegramUI) PrintTokenUsage(count int) {
}

func (u *TelegramUI) PrintCompression(turns int) {
	msg := tgbotapi.NewMessage(u.chatID, fmt.Sprintf("⤓ Earlier conversation compressed (%d turns)", turns))
	msg.ParseMode = "HTML"
	sendBot(u.bot, msg)
}

func (u *TelegramUI) RequestApproval(ctx context.Context, toolName string, args string, mode string) bool {
	if toolName == "read_file" || toolName == "search_code" || toolName == "list_directory" {
		return true
	}

	displayArgs := args
	if len(displayArgs) > 150 {
		displayArgs = displayArgs[:150] + "..."
	}
	text := fmt.Sprintf(
		"⛭ Approve tool execution?\n\n<b>Tool:</b> <code>%s</code>\n<b>Args:</b> <code>%s</code>\n\nReply with /yes or /no",
		escapeHTML(toolName),
		escapeHTML(displayArgs),
	)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✔ Approve", "approve"),
			tgbotapi.NewInlineKeyboardButtonData("✘ Reject", "reject"),
		),
	)

	msg := tgbotapi.NewMessage(u.chatID, text)
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = keyboard
	sent, err := u.bot.Send(msg)
	if err != nil {
		return false
	}

	u.approvalChan = make(chan bool, 1)
	u.approvalMsgID = sent.MessageID

	approvalCtx, aCancel := context.WithTimeout(ctx, 300*time.Second)
	defer aCancel()

	select {
	case approved := <-u.approvalChan:
		u.approvalChan = nil
		return approved
	case <-approvalCtx.Done():
		u.approvalChan = nil
		return false
	}
}

func (u *TelegramUI) RequestChainContinue(ctx context.Context) bool {
	text := "⚠ Reached consecutive tool calls limit. Continue?"

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("▶ Continue", "continue_yes"),
			tgbotapi.NewInlineKeyboardButtonData("■ Stop", "continue_no"),
		),
	)

	msg := tgbotapi.NewMessage(u.chatID, text)
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = keyboard
	sendBot(u.bot, msg)

	u.chainChan = make(chan bool, 1)

	select {
	case cont := <-u.chainChan:
		u.chainChan = nil
		return cont
	case <-ctx.Done():
		u.chainChan = nil
		return false
	}
}

func (u *TelegramUI) sendLongMessage(text string) {
	const maxLen = 4000 
	const maxEdits = 25 

	if len(text) <= maxLen {
		if u.activeMsgID != 0 && u.editCount < maxEdits {
			editMsg := tgbotapi.NewEditMessageText(u.chatID, u.activeMsgID, text)
			editMsg.ParseMode = "HTML"
			if _, err := u.bot.Send(editMsg); err == nil {
				u.editCount++
				return
			}
		}

		msg := tgbotapi.NewMessage(u.chatID, text)
		msg.ParseMode = "HTML"
		sent, err := sendBotWithResult(u.bot, msg)
		if err == nil {
			u.activeMsgID = sent.MessageID
			u.editCount = 0
		}
		return
	}

	u.activeMsgID = 0
	u.editCount = 0

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

		msg := tgbotapi.NewMessage(u.chatID, chunk)
		msg.ParseMode = "HTML"
		sendBot(u.bot, msg)
	}
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func markdownToHTML(md string) string {
	s := escapeHTML(md)

	s = fenceRe.ReplaceAllStringFunc(s, func(match string) string {
		submatches := fenceRe.FindStringSubmatch(match)
		if len(submatches) < 3 {
			return match
		}
		lang := strings.TrimSpace(submatches[1])
		code := submatches[2]
		if lang != "" {
			return fmt.Sprintf("<pre><code class=\"language-%s\">%s</code></pre>", lang, code)
		}
		return fmt.Sprintf("<pre><code>%s</code></pre>", code)
	})

	s = inlineCodeRe.ReplaceAllString(s, "<code>$1</code>")

	s = boldRe.ReplaceAllString(s, "<b>$1</b>")

	s = italicRe.ReplaceAllString(s, "<i>$1</i>")

	s = linkRe.ReplaceAllString(s, `<a href="$2">$1</a>`)

	s = renderBlocks(s)

	return s
}

var (
	fenceRe      = regexp.MustCompile("(?s)```([a-zA-Z0-9]*)\n?(.*?)```")
	inlineCodeRe = regexp.MustCompile("`([^`\n]+)`")
	boldRe       = regexp.MustCompile(`\*\*(.+?)\*\*`)
	italicRe     = regexp.MustCompile(`\*([^*\n]+?)\*`)
	linkRe       = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
)

func renderBlocks(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	var quoteActive bool
	flushQuote := func() {
		if quoteActive {
			out = append(out, "</blockquote>")
			quoteActive = false
		}
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "#"):
			flushQuote()
			level := 0
			for level < len(trimmed) && trimmed[level] == '#' {
				level++
			}
			if level > 6 {
				level = 6
			}
			text := strings.TrimSpace(trimmed[level:])
			out = append(out, fmt.Sprintf("<b>%s</b>", text))
		case strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* "):
			flushQuote()
			out = append(out, "• "+strings.TrimSpace(trimmed[2:]))
		case orderedRe.MatchString(trimmed):
			flushQuote()
			m := orderedRe.FindStringSubmatch(trimmed)
			out = append(out, m[1]+". "+strings.TrimSpace(m[2]))
		case strings.HasPrefix(trimmed, "> ") || strings.HasPrefix(trimmed, "&gt; "):
			if !quoteActive {
				out = append(out, "<blockquote>")
				quoteActive = true
			}
			content := strings.TrimSpace(trimmed)
			content = strings.TrimPrefix(content, "> ")
			content = strings.TrimPrefix(content, "&gt; ")
			out = append(out, content)
		default:
			flushQuote()
			if trimmed != "" {
				out = append(out, line)
			}
		}
	}
	flushQuote()
	return strings.Join(out, "\n")
}

var orderedRe = regexp.MustCompile(`^(\d+)\.\s+(.*)$`)

func (u *TelegramUI) AskUser(ctx context.Context, question string) (string, error) {
	msg := tgbotapi.NewMessage(u.chatID, fmt.Sprintf("? **Question:** %s\n\n*(Please reply directly to answer)*", question))
	sendBot(u.bot, msg)

	u.askChan = make(chan string, 1)
	defer func() {
		u.askChan = nil
	}()

	select {
	case answer := <-u.askChan:
		return answer, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (u *TelegramUI) formatChecklist() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⎔ <b>Planned Steps for:</b> %s\n", escapeHTML(u.activeGoal)))
	sb.WriteString("──────────────────────────\n")
	for idx, rawStep := range u.activeSteps {
		parts := strings.SplitN(rawStep, "|", 2)
		if len(parts) != 2 {
			continue
		}
		stepID := parts[0]
		stepDesc := parts[1]

		status := u.activeStepStatuses[stepID]
		icon := "⧗"
		switch status {
		case "running":
			icon = "❯"
		case "completed":
			icon = "✔"
		case "failed":
			icon = "✘"
		}

		sb.WriteString(fmt.Sprintf("• %s %d. %s\n", icon, idx+1, escapeHTML(stepDesc)))
	}
	return sb.String()
}

func (u *TelegramUI) updateChecklist() {
	if u.planMsgID == 0 {
		return
	}
	text := u.formatChecklist()
	msg := tgbotapi.NewEditMessageText(u.chatID, u.planMsgID, text)
	msg.ParseMode = "HTML"
	sendBot(u.bot, msg)
}

func (u *TelegramUI) PrintPlan(goal string, steps []string) {
	u.activeGoal = goal
	u.activeSteps = steps
	u.activeStepStatuses = make(map[string]string)
	for _, rawStep := range steps {
		parts := strings.SplitN(rawStep, "|", 2)
		if len(parts) == 2 {
			u.activeStepStatuses[parts[0]] = "pending"
		}
	}

	text := u.formatChecklist()
	msg := tgbotapi.NewMessage(u.chatID, text)
	msg.ParseMode = "HTML"
	sent, err := sendBotWithResult(u.bot, msg)
	if err == nil {
		u.planMsgID = sent.MessageID
	}
}

func (u *TelegramUI) PrintStepStart(stepID string) {
	if u.activeStepStatuses != nil {
		u.activeStepStatuses[stepID] = "running"
	}
	u.updateChecklist()
}

func (u *TelegramUI) PrintStepFinish(stepID string, success bool, result string) {
	if u.activeStepStatuses != nil {
		if success {
			u.activeStepStatuses[stepID] = "completed"
		} else {
			u.activeStepStatuses[stepID] = "failed"
		}
	}
	u.updateChecklist()
}

