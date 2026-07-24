package discord

import (
	"awas/internal/gateway"
	"awas/internal/tools"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func (dg *DiscordGateway) OnReady(s *discordgo.Session, r *discordgo.Ready) {
	log.Printf("[discord] Bot logged in as: %s#%s", r.User.Username, r.User.Discriminator)
}

func (dg *DiscordGateway) OnMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate, mgr *gateway.Manager) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	dg.mu.RLock()
	session, hasSession := dg.users[m.ChannelID]
	dg.mu.RUnlock()
	if hasSession && session.Loop.UI != nil {
		if discUI, ok := session.Loop.UI.(*DiscordUI); ok && discUI.askChan != nil {
			discUI.askChan <- m.Content
			return
		}
	}

	guildID := ""
	if dg.config.Extra != nil {
		guildID = dg.config.Extra["guild_id"]
	}
	if guildID != "" && m.GuildID != guildID {
		return 
	}

	channel, err := s.State.Channel(m.ChannelID)
	if err != nil {
		channel, err = s.Channel(m.ChannelID)
		if err != nil {
			log.Printf("[discord] Failed to get channel details for %s: %v", m.ChannelID, err)
			return
		}
	}

	isThread := channel.Type == discordgo.ChannelTypeGuildPublicThread ||
		channel.Type == discordgo.ChannelTypeGuildPrivateThread ||
		channel.Type == discordgo.ChannelTypeGuildNewsThread

	botMention := fmt.Sprintf("<@%s>", s.State.User.ID)
	botMentionNickname := fmt.Sprintf("<@!%s>", s.State.User.ID)

	cleanContent := strings.TrimSpace(m.Content)
	cleanContent = strings.ReplaceAll(cleanContent, botMention, "")
	cleanContent = strings.ReplaceAll(cleanContent, botMentionNickname, "")
	cleanContent = strings.TrimSpace(cleanContent)

	isCronTrigger := strings.HasPrefix(cleanContent, "/cron") ||
		strings.HasPrefix(strings.ToLower(cleanContent), "jadwalin") ||
		strings.HasPrefix(strings.ToLower(cleanContent), "buat cron") ||
		strings.HasPrefix(strings.ToLower(cleanContent), "buat jadwal") ||
		strings.HasPrefix(strings.ToLower(cleanContent), "schedule") ||
		strings.HasPrefix(strings.ToLower(cleanContent), "tambahkan cron")

	if isCronTrigger {
		if !isThread {
			hasMention := false
			for _, mention := range m.Mentions {
				if mention.ID == s.State.User.ID {
					hasMention = true
					break
				}
			}
			if !hasMention {
				return
			}
		}

		var args []string
		if strings.HasPrefix(cleanContent, "/cron") {
			if len(cleanContent) > len("/cron") {
				args = strings.Fields(cleanContent[len("/cron"):])
			}
		} else {
			args = []string{cleanContent}
		}

		response := gateway.HandleCronCommand(mgr.CronStore, "discord", m.ChannelID, m.GuildID, args)
		dg.SendText(m.ChannelID, response)
		return
	}

	if isThread {
		session := dg.getSession(m.ChannelID, m.Author.Username, mgr)
		if session == nil {
			s.ChannelMessageSend(m.ChannelID, "✘ Access denied or max active sessions reached.")
			return
		}

		ui := NewDiscordUI(s, m.ChannelID, dg)
		dg.mu.Lock()
		dg.ensureProcessor(m.ChannelID, session)
		ch := dg.msgChs[m.ChannelID]
		dg.mu.Unlock()

		text := downloadAttachments(m.Attachments, session.Loop.GetConfig().WorkDir, m.Content)

		select {
		case ch <- pendingMsg{text: text, ui: ui}:
		default:
			s.ChannelMessageSend(m.ChannelID, "⧗ Still processing previous command. Please wait...")
		}

	} else {

		hasMention := false
		for _, mention := range m.Mentions {
			if mention.ID == s.State.User.ID {
				hasMention = true
				break
			}
		}

		if !hasMention {
			return
		}

		cleanContent := m.Content
		cleanContent = strings.ReplaceAll(cleanContent, botMention, "")
		cleanContent = strings.ReplaceAll(cleanContent, botMentionNickname, "")
		cleanContent = strings.TrimSpace(cleanContent)

		if cleanContent == "" {
			s.ChannelMessageSend(m.ChannelID, "👋 Hello! Mention me followed by your message to start a new session inside a thread.")
			return
		}

		threadName := cleanContent
		if len(threadName) > 40 {
			threadName = threadName[:37] + "..."
		}
		threadName = fmt.Sprintf("awas-%s", threadName)

		thread, err := s.MessageThreadStartComplex(m.ChannelID, m.ID, &discordgo.ThreadStart{
			Name:                threadName,
			AutoArchiveDuration: 60, 
			Type:                discordgo.ChannelTypeGuildPublicThread,
		})
		if err != nil {
			log.Printf("[discord] Failed to create thread: %v", err)
			s.ChannelMessageSend(m.ChannelID, "✘ Failed to create a new thread for conversation.")
			return
		}

		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("🧵 New session started in thread <#%s>", thread.ID))

		session := dg.getSession(thread.ID, m.Author.Username, mgr)
		if session == nil {
			s.ChannelMessageSend(thread.ID, "✘ Access denied or max active sessions reached.")
			return
		}

		ui := NewDiscordUI(s, thread.ID, dg)
		dg.mu.Lock()
		dg.ensureProcessor(thread.ID, session)
		ch := dg.msgChs[thread.ID]
		dg.mu.Unlock()

		text := downloadAttachments(m.Attachments, session.Loop.GetConfig().WorkDir, cleanContent)

		select {
		case ch <- pendingMsg{text: text, ui: ui}:
		default:
			s.ChannelMessageSend(thread.ID, "⧗ Still processing previous command. Please wait...")
		}
	}
}

func (dg *DiscordGateway) OnInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate, mgr *gateway.Manager) {
	guildID := ""
	if dg.config.Extra != nil {
		guildID = dg.config.Extra["guild_id"]
	}
	if guildID != "" && i.GuildID != guildID {
		return 
	}

	threadID := i.ChannelID

	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		cmdName := i.ApplicationCommandData().Name
		switch cmdName {
		case "status":
			dg.mu.RLock()
			session, exists := dg.users[threadID]
			dg.mu.RUnlock()

			var reply string
			if exists {
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

				reply = fmt.Sprintf("⎔ **Session Status Info**\n"+
					"──────────────────────────\n"+
					"• **Session ID**: `%s`\n"+
					"• **Status**: `%s`\n"+
					"• **Active Mode**: `%s` (Switch via `/mode`)\n"+
					"• **Active Model**: `%s`\n"+
					"• **Work Directory**: `%s`\n"+
					"• **Conversation**: %d messages\n"+
					"• **Token Usage**: ~%d tokens\n"+
					"• **Last Active**: %s",
					session.SessionID,
					statusStr,
					loopCfg.AgentMode,
					loopCfg.Model,
					loopCfg.WorkDir,
					len(history),
					estTokens,
					session.LastActive.Format("2006-01-02 15:04:05"),
				)
			} else {
				reply = "✘ No active session found in this thread."
			}

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: reply,
				},
			})

		case "reset":
			sessionID := fmt.Sprintf("gw-discord-%s", threadID)
			gateway.DeleteSession(sessionID)
			dg.removeSession(threadID)
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "⟳ Session reset successfully for this thread.",
				},
			})

		case "yes", "no":
			dg.handleApprovalReply(threadID, cmdName == "yes")
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf("✔ Sent approval decision: **%s**", cmdName),
				},
			})

		case "continue", "stop":
			if cmdName == "stop" {
				dg.mu.RLock()
				session, ok := dg.users[threadID]
				dg.mu.RUnlock()

				if ok && session.IsRunning && session.ActiveCancel != nil {
					session.ActiveCancel()
					s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
						Type: discordgo.InteractionResponseChannelMessageWithSource,
						Data: &discordgo.InteractionResponseData{
							Content: "⏹️ Aborting active execution...",
						},
					})
					return
				}
			}

			dg.handleChainReply(threadID, cmdName == "continue")
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf("✔ Sent chain decision: **%s**", cmdName),
				},
			})

		case "cron":
			cmdArg := ""
			options := i.ApplicationCommandData().Options
			if len(options) > 0 {
				cmdArg = options[0].StringValue()
			}

			var args []string
			if cmdArg != "" {
				args = strings.Fields(cmdArg)
			}

			response := gateway.HandleCronCommand(mgr.CronStore, "discord", threadID, i.GuildID, args)

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: response,
				},
			})

		case "mode":
			dg.mu.RLock()
			session, exists := dg.users[threadID]
			dg.mu.RUnlock()

			if !exists {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "✘ No active session found in this thread.",
					},
				})
				return
			}

			modeType := ""
			options := i.ApplicationCommandData().Options
			if len(options) > 0 {
				modeType = options[0].StringValue()
			}

			if modeType == "" {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: fmt.Sprintf("ℹ Current session mode: **%s**", session.Loop.GetConfig().AgentMode),
					},
				})
				return
			}

			switch modeType {
			case "chat", "simple", "planned", "deep":
				session.Loop.GetConfig().AgentMode = modeType
				session.SaveSession(dg.cfg)
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: fmt.Sprintf("✔ Mode switched to **%s** successfully for this session.", modeType),
					},
				})
			default:
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "✘ Invalid mode. Use: chat, simple, planned, or deep.",
					},
				})
			}
		}

	case discordgo.InteractionMessageComponent:
		customID := i.MessageComponentData().CustomID

		var action string
		var targetThread string

		switch {
		case strings.HasPrefix(customID, "approve_"):
			action = "approve"
			targetThread = strings.TrimPrefix(customID, "approve_")
		case strings.HasPrefix(customID, "reject_"):
			action = "reject"
			targetThread = strings.TrimPrefix(customID, "reject_")
		case strings.HasPrefix(customID, "continue_yes_"):
			action = "continue_yes"
			targetThread = strings.TrimPrefix(customID, "continue_yes_")
		case strings.HasPrefix(customID, "continue_no_"):
			action = "continue_no"
			targetThread = strings.TrimPrefix(customID, "continue_no_")
		}

		if targetThread == "" {
			return
		}

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    i.Message.Content + fmt.Sprintf("\n\n*Selected:* **%s**", strings.ToUpper(action)),
				Components: []discordgo.MessageComponent{}, 
			},
		})

		switch action {
		case "approve":
			dg.handleApprovalReply(targetThread, true)
		case "reject":
			dg.handleApprovalReply(targetThread, false)
		case "continue_yes":
			dg.handleChainReply(targetThread, true)
		case "continue_no":
			dg.handleChainReply(targetThread, false)
		}
	}
}

func (dg *DiscordGateway) handleApprovalReply(threadID string, approved bool) {
	dg.mu.RLock()
	session, ok := dg.users[threadID]
	dg.mu.RUnlock()

	if !ok {
		return
	}

	if ui, ok := session.Loop.UI.(*DiscordUI); ok {
		ui.SendApprovalResponse(approved)
	}
}

func (dg *DiscordGateway) handleChainReply(threadID string, continueChain bool) {
	dg.mu.RLock()
	session, ok := dg.users[threadID]
	dg.mu.RUnlock()

	if !ok {
		return
	}

	if ui, ok := session.Loop.UI.(*DiscordUI); ok {
		ui.SendChainResponse(continueChain)
	}
}

func downloadAttachments(attachments []*discordgo.MessageAttachment, workDir string, originalText string) string {
	if len(attachments) == 0 {
		return originalText
	}
	downloadsDir := filepath.Join(workDir, "downloads")
	os.MkdirAll(downloadsDir, 0755)

	text := originalText
	for _, att := range attachments {
		destPath := filepath.Join(downloadsDir, att.Filename)
		tools.DownloadFile(att.URL, destPath)
		text = fmt.Sprintf("[System Notification: User uploaded file '%s' and saved to 'downloads/%s']\n%s", att.Filename, att.Filename, text)
	}
	return text
}
