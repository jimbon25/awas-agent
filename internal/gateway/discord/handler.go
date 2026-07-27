package discord

import (
	"awas/internal/agent"
	"awas/internal/client"
	"awas/internal/gateway"
	"awas/internal/provider"
	"awas/internal/tools"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
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

				reply = fmt.Sprintf("⎔ **Session Status Info**\n\n"+
					"• **Session ID**: `%s`\n"+
					"• **Status**: `%s`\n"+
					"• **Active Subagents**: `%s`\n"+
					"• **Active Mode**: `%s` (Switch via `/mode`)\n"+
					"• **Active Model**: `%s`\n"+
					"• **Work Directory**: `%s`\n"+
					"• **Conversation**: %d messages\n"+
					"• **Token Usage**: ~%d tokens\n"+
					"• **Last Active**: %s",
					session.SessionID,
					statusStr,
					subagentStr,
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

		case "help":
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "📋 **Available Commands**\n\n" +
						"`/status` — Show session info\n" +
						"`/reset` — Reset conversation memory\n" +
						"`/mode` — Switch mode (chat/simple/planned/deep)\n" +
						"`/model` — View/switch provider profiles and models\n" +
						"`/tokens` — Show token usage\n" +
						"`/cron` — Manage scheduled jobs\n" +
						"`/yes` — Approve tool execution\n" +
						"`/no` — Reject tool execution\n" +
						"`/continue` — Continue tool chain\n" +
						"`/stop` — Stop tool chain",
				},
			})

		case "model":
			modelArg := ""
			options := i.ApplicationCommandData().Options
			if len(options) > 0 {
				modelArg = options[0].StringValue()
			}

			mgr := provider.NewManager("")

			if modelArg == "" {
				// Show all profiles as select menu
				profileNames := make([]string, 0, len(mgr.Profiles))
				for name := range mgr.Profiles {
					profileNames = append(profileNames, name)
				}
				sort.Strings(profileNames)

				if len(profileNames) == 0 {
					s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
						Type: discordgo.InteractionResponseChannelMessageWithSource,
						Data: &discordgo.InteractionResponseData{
							Content: "✘ No provider profiles found. Run `/setup` to configure one.",
						},
					})
					return
				}

				var options []discordgo.SelectMenuOption
				for _, name := range profileNames {
					p := mgr.Profiles[name]
					label := fmt.Sprintf("%s — %s", name, p.Model)
					if name == mgr.ActiveProfile {
						label += " (active)"
					}
					options = append(options, discordgo.SelectMenuOption{
						Label:       label,
						Value:       name,
						Description: fmt.Sprintf("Provider: %s", p.Name),
					})
				}

				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "📋 **Select Provider Profile:**",
						Components: []discordgo.MessageComponent{
							discordgo.ActionsRow{
								Components: []discordgo.MessageComponent{
									discordgo.SelectMenu{
										CustomID:    "model_select_discord:" + threadID,
										Placeholder: "Select a provider profile...",
										Options:     options,
									},
								},
							},
						},
					},
				})
				return
			}

			dg.mu.RLock()
			session, ok := dg.users[threadID]
			dg.mu.RUnlock()

			if !ok {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "✘ No active session found in this thread.",
					},
				})
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
				session.SaveSession(dg.cfg)
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: fmt.Sprintf("✔ Switched to profile: **%s** (Model: **%s**)", modelArg, p.Model),
					},
				})
			} else {
				// Treat as model name
				session.Loop.GetConfig().Model = modelArg
				if p, ok := mgr.Profiles[mgr.ActiveProfile]; ok {
					p.Model = modelArg
					mgr.Save()
					session.Loop.SetClient(client.New(p))
				}
				session.SaveSession(dg.cfg)
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: fmt.Sprintf("✔ Model changed to: **%s**", modelArg),
					},
				})
			}

		case "tokens":
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

			history := session.Loop.GetHistory()
			tokenCount := agent.EstimateTotalTokens(history)
			tokenMax := session.Loop.GetConfig().MaxTokens
			if tokenMax <= 0 {
				tokenMax = 1
			}

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf("📊 %s / %s tokens",
						formatDiscordTokens(tokenCount),
						formatDiscordTokens(tokenMax)),
				},
			})
		}

	case discordgo.InteractionMessageComponent:
		customID := i.MessageComponentData().CustomID

		// Handle model select menu
		if strings.HasPrefix(customID, "model_select_discord:") {
			profileName := i.MessageComponentData().Values[0]
			selectedThreadID := strings.TrimPrefix(customID, "model_select_discord:")

			dg.mu.RLock()
			session, ok := dg.users[selectedThreadID]
			dg.mu.RUnlock()

			if !ok {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseUpdateMessage,
					Data: &discordgo.InteractionResponseData{
						Content:    "✘ No active session found.",
						Components: []discordgo.MessageComponent{},
					},
				})
				return
			}

			mgr := provider.NewManager("")
			p, ok := mgr.Profiles[profileName]
			if !ok {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseUpdateMessage,
					Data: &discordgo.InteractionResponseData{
						Content:    fmt.Sprintf("✘ Profile **%s** not found.", profileName),
						Components: []discordgo.MessageComponent{},
					},
				})
				return
			}

			mgr.ActiveProfile = profileName
			mgr.Save()
			session.Loop.GetConfig().Model = p.Model
			session.Loop.GetConfig().Endpoint = p.GetEndpoint()
			session.Loop.GetConfig().APIKey = p.GetAPIKey()
			session.Loop.SetClient(client.New(p))
			session.SaveSession(dg.cfg)

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseUpdateMessage,
				Data: &discordgo.InteractionResponseData{
					Content:    fmt.Sprintf("✔ Switched to profile: **%s** (Model: **%s**)", profileName, p.Model),
					Components: []discordgo.MessageComponent{},
				},
			})
			return
		}

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

func formatDiscordTokens(n int) string {
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
