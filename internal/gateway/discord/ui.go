package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
	"os"
	"path/filepath"

	"github.com/bwmarrin/discordgo"
)

type DiscordUI struct {
	session  *discordgo.Session
	threadID string
	gw       *DiscordGateway

	approvalChan  chan bool
	approvalMsgID string

	chainChan  chan bool
	chainMsgID string

	askChan chan string

	planMsgID          string
	activeGoal         string
	activeSteps        []string
	activeStepStatuses map[string]string

	activeMessageID string
	editCount       int
	streamBuffer    string
	lastEditTime    time.Time
}

func NewDiscordUI(session *discordgo.Session, threadID string, gw *DiscordGateway) *DiscordUI {
	return &DiscordUI{
		session:  session,
		threadID: threadID,
		gw:       gw,
	}
}

func (u *DiscordUI) SendApprovalResponse(approved bool) {
	if u.approvalChan != nil {
		select {
		case u.approvalChan <- approved:
		default:
		}
	}
}

func (u *DiscordUI) SendChainResponse(continueChain bool) {
	if u.chainChan != nil {
		select {
		case u.chainChan <- continueChain:
		default:
		}
	}
}

func (u *DiscordUI) PrintThinking(model string) {
	u.activeMessageID = ""
	u.editCount = 0
	u.streamBuffer = ""
	u.session.ChannelTyping(u.threadID)
}

func (u *DiscordUI) PrintMessage(role string, content string) {
	if role == "assistant" && content != "" {
		u.streamBuffer = content
		u.sendLongMessage(content)
	} else if role == "system" {
		msg := "ℹ " + content
		if !strings.HasPrefix(content, ">") {
			msg = "> ℹ " + strings.ReplaceAll(content, "\n", "\n> ")
		}
		u.session.ChannelMessageSend(u.threadID, msg)
	}
}

func (u *DiscordUI) SendFile(filePath string, caption string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = u.session.ChannelFileSendWithMessage(u.threadID, caption, filepath.Base(filePath), file)
	return err
}

func (u *DiscordUI) PrintMessageDelta(content string) {
	u.streamBuffer += content
	if time.Since(u.lastEditTime) >= 1200*time.Millisecond {
		u.lastEditTime = time.Now()
		u.sendLongMessage(u.streamBuffer)
	}
}

func (u *DiscordUI) PrintToolCall(name string, args string) {
	summary := summarizeArgs(name, args)
	toolText := fmt.Sprintf("\n```sh\n▸ %s\n  %s\n```", name, summary)
	u.streamBuffer += toolText
	u.lastEditTime = time.Now()
	u.sendLongMessage(u.streamBuffer)
}

func (u *DiscordUI) PrintToolResult(name string, result string) {
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

	resText := fmt.Sprintf("\n```sh\n  %s %s\n  %s\n```", icon, name, displayRes)
	u.streamBuffer += resText
	u.lastEditTime = time.Now()
	u.sendLongMessage(u.streamBuffer)
}

func (u *DiscordUI) PrintTokenUsage(count int) {
}

func (u *DiscordUI) PrintCompression(turns int) {
	text := fmt.Sprintf("⤓ Earlier conversation compressed (%d turns)", turns)
	u.session.ChannelMessageSend(u.threadID, text)
}

func (u *DiscordUI) RequestApproval(ctx context.Context, toolName string, args string, mode string) bool {
	if toolName == "read_file" || toolName == "search_code" || toolName == "list_directory" {
		return true
	}

	displayArgs := args
	if len(displayArgs) > 150 {
		displayArgs = displayArgs[:150] + "..."
	}

	text := fmt.Sprintf(
		"🔐 **Approve tool execution?**\n\n**Tool:** `%s`\n**Args:** `%s`\n\nChoose an action below or reply with `/yes` / `/no`",
		toolName,
		displayArgs,
	)

	buttons := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Approve",
					Style:    discordgo.SuccessButton,
					CustomID: "approve_" + u.threadID,
					Emoji: &discordgo.ComponentEmoji{
						Name: "✅",
					},
				},
				discordgo.Button{
					Label:    "Reject",
					Style:    discordgo.DangerButton,
					CustomID: "reject_" + u.threadID,
					Emoji: &discordgo.ComponentEmoji{
						Name: "❌",
					},
				},
			},
		},
	}

	msgSend := &discordgo.MessageSend{
		Content:    text,
		Components: buttons,
	}

	sent, err := u.session.ChannelMessageSendComplex(u.threadID, msgSend)
	if err != nil {
		log.Printf("[discord] Failed to send approval message: %v", err)
		return false
	}

	u.approvalChan = make(chan bool, 1)
	u.approvalMsgID = sent.ID

	approvalCtx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()

	select {
	case approved := <-u.approvalChan:
		u.approvalChan = nil
		return approved
	case <-approvalCtx.Done():
		u.approvalChan = nil
		return false
	}
}

func (u *DiscordUI) RequestChainContinue(ctx context.Context) bool {
	text := "⚠ **Reached consecutive tool calls limit. Continue?**"

	buttons := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Continue",
					Style:    discordgo.PrimaryButton,
					CustomID: "continue_yes_" + u.threadID,
					Emoji: &discordgo.ComponentEmoji{
						Name: "▶️",
					},
				},
				discordgo.Button{
					Label:    "Stop",
					Style:    discordgo.DangerButton,
					CustomID: "continue_no_" + u.threadID,
					Emoji: &discordgo.ComponentEmoji{
						Name: "⏹️",
					},
				},
			},
		},
	}

	msgSend := &discordgo.MessageSend{
		Content:    text,
		Components: buttons,
	}

	sent, err := u.session.ChannelMessageSendComplex(u.threadID, msgSend)
	if err != nil {
		log.Printf("[discord] Failed to send chain continue: %v", err)
		return false
	}

	u.chainChan = make(chan bool, 1)
	u.chainMsgID = sent.ID

	select {
	case cont := <-u.chainChan:
		u.chainChan = nil
		return cont
	case <-ctx.Done():
		u.chainChan = nil
		return false
	}
}

func (u *DiscordUI) sendLongMessage(text string) {
	const maxLen = 1900 
	const maxEdits = 10 

	if len(text) <= maxLen {
		if u.activeMessageID != "" && u.editCount < maxEdits {
			if _, err := u.session.ChannelMessageEdit(u.threadID, u.activeMessageID, text); err == nil {
				u.editCount++
				return
			}
		}

		sent, err := u.session.ChannelMessageSend(u.threadID, text)
		if err == nil {
			u.activeMessageID = sent.ID
			u.editCount = 0
		}
		return
	}

	u.activeMessageID = ""
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

		u.session.ChannelMessageSend(u.threadID, chunk)
	}
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

func (u *DiscordUI) AskUser(ctx context.Context, question string) (string, error) {
	u.session.ChannelMessageSend(u.threadID, fmt.Sprintf("? **Question:** %s\n\n*(Please reply directly in this thread to answer)*", question))

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

func (u *DiscordUI) formatChecklist() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⎔ **Planned Steps for:** *%s*\n", u.activeGoal))
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

		sb.WriteString(fmt.Sprintf("• %s %d. %s\n", icon, idx+1, stepDesc))
	}
	return sb.String()
}

func (u *DiscordUI) updateChecklist() {
	if u.planMsgID == "" {
		return
	}
	text := u.formatChecklist()
	u.session.ChannelMessageEdit(u.threadID, u.planMsgID, text)
}

func (u *DiscordUI) PrintPlan(goal string, steps []string) {
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
	sent, err := u.session.ChannelMessageSend(u.threadID, text)
	if err == nil {
		u.planMsgID = sent.ID
	}
}

func (u *DiscordUI) PrintStepStart(stepID string) {
	if u.activeStepStatuses != nil {
		u.activeStepStatuses[stepID] = "running"
	}
	u.updateChecklist()
}

func (u *DiscordUI) PrintStepFinish(stepID string, success bool, result string) {
	if u.activeStepStatuses != nil {
		if success {
			u.activeStepStatuses[stepID] = "completed"
		} else {
			u.activeStepStatuses[stepID] = "failed"
		}
	}
	u.updateChecklist()
}
