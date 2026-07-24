package tui

import (
	"awas/internal/agent"
	"awas/internal/client"
	"awas/internal/gateway"
	"awas/internal/provider"
	"awas/internal/tools"
	"awas/internal/tui/wizard"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func handleSlashCommand(m *Model, userInput string) tea.Cmd {
	parts := strings.Fields(userInput)
	if len(parts) == 0 {
		return nil
	}
	cmdName := parts[0]
	cmdArgs := ""
	if len(parts) > 1 {
		cmdArgs = strings.Join(parts[1:], " ")
	}

	switch cmdName {
	case "/exit", "/quit":
		return tea.Quit
	case "/clear":
		m.Messages = nil
		updateViewportContent(m)
	case "/reset":
		m.Messages = nil
		go func() {
			m.PromptChan <- AgentPrompt{Prompt: "/reset", Ctx: context.Background()}
		}()
	case "/mode":
		if cmdArgs == "safe" || cmdArgs == "autonomous" {
			m.Cfg.Mode = cmdArgs
			m.Messages = append(m.Messages, UIMessage{
				Role:    "system",
				Content: "Interaction mode changed to: " + cmdArgs,
			})
		} else if cmdArgs == "chat" || cmdArgs == "simple" || cmdArgs == "planned" || cmdArgs == "deep" {
			m.Cfg.AgentMode = cmdArgs
			m.Cfg.Save()
			m.Messages = append(m.Messages, UIMessage{
				Role:    "system",
				Content: "Agent reasoning mode changed to: " + cmdArgs,
			})
		} else if cmdArgs == "" {
			if m.Cfg.Mode == "safe" {
				m.Cfg.Mode = "autonomous"
			} else {
				m.Cfg.Mode = "safe"
			}
			m.Messages = append(m.Messages, UIMessage{
				Role:    "system",
				Content: "Interaction mode changed to: " + m.Cfg.Mode,
			})
		} else {
			m.Messages = append(m.Messages, UIMessage{
				Role:    "system",
				Content: "Usage: /mode [safe|autonomous] or /mode [chat|simple|planned|deep]",
			})
		}
		updateViewportContent(m)
	case "/stream":
		if cmdArgs == "on" || cmdArgs == "true" {
			m.Cfg.Stream = true
			m.Cfg.Save()
			m.Messages = append(m.Messages, UIMessage{
				Role:    "system",
				Content: "SSE Streaming: ON",
			})
		} else if cmdArgs == "off" || cmdArgs == "false" {
			m.Cfg.Stream = false
			m.Cfg.Save()
			m.Messages = append(m.Messages, UIMessage{
				Role:    "system",
				Content: "SSE Streaming: OFF",
			})
		} else {
			m.Cfg.Stream = !m.Cfg.Stream
			m.Cfg.Save()
			status := "OFF"
			if m.Cfg.Stream {
				status = "ON"
			}
			m.Messages = append(m.Messages, UIMessage{
				Role:    "system",
				Content: "SSE Streaming: " + status,
			})
		}
		updateViewportContent(m)
	case "/model":
		if cmdArgs != "" {
			m.Cfg.Model = cmdArgs
			m.Cfg.Save()

			mgr := provider.NewManager("")
			if p, ok := mgr.Profiles[mgr.ActiveProfile]; ok {
				p.Model = cmdArgs
				mgr.Save()
				if m.Loop != nil {
					m.Loop.SetClient(client.New(p))
				}
			}

			m.Messages = append(m.Messages, UIMessage{
				Role:    "system",
				Content: "Model changed to: " + cmdArgs,
			})
		} else {
			m.Messages = append(m.Messages, UIMessage{
				Role:    "system",
				Content: "Current Model: " + m.Cfg.Model + "\nUsage: /model [model_name]",
			})
		}
		updateViewportContent(m)
	case "/switch":
		mgr := provider.NewManager("")
		if cmdArgs == "" {
			var profiles []string
			for name := range mgr.Profiles {
				profiles = append(profiles, name)
			}
			if len(profiles) == 0 {
				m.Messages = append(m.Messages, UIMessage{
					Role:    "system",
					Content: "No provider profiles configured. Run /setup to configure one.",
				})
			} else {
				sort.Strings(profiles)
				m.SwitchProfiles = profiles
				m.SwitchCursor = 0
				for i, name := range profiles {
					if name == mgr.ActiveProfile {
						m.SwitchCursor = i
						break
					}
				}
				m.State = StateProfileSwitch
				recalculateViewportHeight(m)
			}
		} else {
			p, ok := mgr.Profiles[cmdArgs]
			if !ok {
				m.Messages = append(m.Messages, UIMessage{
					Role:    "system",
					Content: fmt.Sprintf("Profile '%s' not found.", cmdArgs),
				})
			} else {
				mgr.ActiveProfile = cmdArgs
				mgr.Save()

				m.Cfg.Endpoint = p.GetEndpoint()
				m.Cfg.APIKey = p.GetAPIKey()
				m.Cfg.Model = p.GetModel()
				m.Cfg.Save()

				if m.Loop != nil {
					m.Loop.SetClient(client.New(p))
				}

				m.Messages = append(m.Messages, UIMessage{
					Role:    "system",
					Content: fmt.Sprintf("Switched to provider profile: %s (Model: %s)", cmdArgs, p.Model),
				})
			}
		}
		updateViewportContent(m)
	case "/undo":
		steps := 1
		if cmdArgs != "" {
			if s, err := strconv.Atoi(cmdArgs); err == nil && s > 0 {
				steps = s
			}
		}
		res, err := tools.Undo(m.Cfg.WorkDir, steps)
		var content string
		if err != nil {
			content = fmt.Sprintf("\u274c Undo failed: %v", err)
		} else {
			content = fmt.Sprintf("\u2705 %s", res)
		}
		m.Messages = append(m.Messages, UIMessage{
			Role:    "system",
			Content: content,
		})
		updateViewportContent(m)
	case "/redo":
		steps := 1
		if cmdArgs != "" {
			if s, err := strconv.Atoi(cmdArgs); err == nil && s > 0 {
				steps = s
			}
		}
		res, err := tools.Redo(m.Cfg.WorkDir, steps)
		var content string
		if err != nil {
			content = fmt.Sprintf("\u274c Redo failed: %v", err)
		} else {
			content = fmt.Sprintf("\u2705 %s", res)
		}
		m.Messages = append(m.Messages, UIMessage{
			Role:    "system",
			Content: content,
		})
		updateViewportContent(m)
	case "/undo-history":
		if cmdArgs == "clear" {
			err := tools.ClearHistory()
			var content string
			if err != nil {
				content = fmt.Sprintf("\u274c Clear history failed: %v", err)
			} else {
				content = "\u2705 Undo/Redo history successfully cleared."
			}
			m.Messages = append(m.Messages, UIMessage{
				Role:    "system",
				Content: content,
			})
		} else {
			list, err := tools.GetHistoryList(m.Cfg.WorkDir)
			var content string
			if err != nil {
				content = fmt.Sprintf("\u274c Failed to retrieve history: %v", err)
			} else if len(list) == 0 {
				content = "No history found for this workspace."
			} else {
				content = "Recent workspace history:\n" + strings.Join(list, "\n")
			}
			m.Messages = append(m.Messages, UIMessage{
				Role:    "system",
				Content: content,
			})
		}
		updateViewportContent(m)
	case "/setup":
		m.State = StateSetupWizard
		m.WizardModel = wizard.New()
		recalculateViewportHeight(m)
		updateViewportContent(m)
		return m.WizardModel.Init()
	case "/logout":
		credsPath := ""
		if home, err := os.UserHomeDir(); err == nil {
			credsPath = filepath.Join(home, ".awas", "oauth.json")
		}
		os.Remove(credsPath)

		if home, err := os.UserHomeDir(); err == nil {
			os.Remove(filepath.Join(home, ".awas", "providers.json"))
		}

		m.Cfg.APIKey = ""
		m.Cfg.Endpoint = ""
		m.Cfg.Model = ""
		m.Cfg.Save()

		m.State = StateSetupWizard
		m.WizardModel = wizard.New()
		recalculateViewportHeight(m)
		updateViewportContent(m)
		return m.WizardModel.Init()
	case "/tokens":
		tokenPct := 0.0
		if m.TokenMax > 0 {
			tokenPct = float64(m.TokenCount) / float64(m.TokenMax) * 100
		}
		m.Messages = append(m.Messages, UIMessage{
			Role:    "system",
			Content: fmt.Sprintf("Token Usage Stats: %s / %s (%.1f%%)", formatCommas(m.TokenCount), formatCommas(m.TokenMax), tokenPct),
		})
		updateViewportContent(m)
	case "/limit":
		if cmdArgs != "" {
			if cmdArgs == "off" || cmdArgs == "unlimited" {
				m.Cfg.MaxChainLimit = 0
				m.Cfg.Save()
				m.Messages = append(m.Messages, UIMessage{
					Role:    "system",
					Content: "Consecutive tool call limit disabled (unlimited).",
				})
			} else {
				var limit int
				_, scanErr := fmt.Sscan(cmdArgs, &limit)
				if scanErr == nil && limit >= 0 {
					m.Cfg.MaxChainLimit = limit
					m.Cfg.Save()
					if limit == 0 {
						m.Messages = append(m.Messages, UIMessage{
							Role:    "system",
							Content: "Consecutive tool call limit disabled (unlimited).",
						})
					} else {
						m.Messages = append(m.Messages, UIMessage{
							Role:    "system",
							Content: fmt.Sprintf("Consecutive tool call limit set to: %d", limit),
						})
					}
				} else {
					m.Messages = append(m.Messages, UIMessage{
						Role:    "system",
						Content: "Invalid limit. Usage: /limit [number|off]",
					})
				}
			}
		} else {
			if m.Cfg.MaxChainLimit <= 0 {
				m.Messages = append(m.Messages, UIMessage{
					Role:    "system",
					Content: "Current consecutive tool call limit: disabled (unlimited)",
				})
			} else {
				m.Messages = append(m.Messages, UIMessage{
					Role:    "system",
					Content: fmt.Sprintf("Current consecutive tool call limit: %d steps", m.Cfg.MaxChainLimit),
				})
			}
		}
		updateViewportContent(m)
	case "/skills":
		home, err := os.UserHomeDir()
		if err != nil {
			m.Messages = append(m.Messages, UIMessage{
				Role:    "system",
				Content: "[Error] Could not find user home directory.",
			})
			updateViewportContent(m)
			break
		}
		skillsDir := filepath.Join(home, ".awas", "skills")
		os.MkdirAll(skillsDir, 0755)

		if strings.HasPrefix(cmdArgs, "create ") {
			skillName := strings.TrimSpace(strings.TrimPrefix(cmdArgs, "create "))
			if skillName == "" {
				m.Messages = append(m.Messages, UIMessage{
					Role:    "system",
					Content: "Usage: /skills create [skill_name]",
				})
			} else {
				if !strings.HasSuffix(strings.ToLower(skillName), ".md") {
					skillName = skillName + ".md"
				}
				skillPath := filepath.Join(skillsDir, skillName)
				templateContent := fmt.Sprintf("# Skill: %s\nDefine custom behaviors, rules, or instructions for the agent here.\n", strings.TrimSuffix(skillName, ".md"))
				writeErr := os.WriteFile(skillPath, []byte(templateContent), 0644)
				if writeErr == nil {
					m.Messages = append(m.Messages, UIMessage{
						Role:    "system",
						Content: fmt.Sprintf("\u2139\ufe0f Skill template '%s' created successfully at ~/.awas/skills/%s!", skillName, skillName),
					})
				} else {
					m.Messages = append(m.Messages, UIMessage{
						Role:    "system",
						Content: fmt.Sprintf("[Error] Failed to create skill file: %v", writeErr),
					})
				}
			}
		} else if strings.HasPrefix(cmdArgs, "add ") {
			repo := strings.TrimSpace(strings.TrimPrefix(cmdArgs, "add "))
			if repo == "" {
				m.Messages = append(m.Messages, UIMessage{
					Role:    "system",
					Content: "Usage: /skills add [github_owner/repo_name]",
				})
			} else {
				m.Messages = append(m.Messages, UIMessage{
					Role:    "system",
					Content: fmt.Sprintf("Cloning and installing skills from %s...", repo),
				})
				updateViewportContent(m)
				installed, installErr := installSkillFromGit(repo)
				if installErr != nil {
					m.Messages = append(m.Messages, UIMessage{
						Role:    "system",
						Content: fmt.Sprintf("\u274c Failed to install skill: %v", installErr),
					})
				} else {
					m.Messages = append(m.Messages, UIMessage{
						Role:    "system",
						Content: fmt.Sprintf("\u2705 Successfully installed skills: %s", strings.Join(installed, ", ")),
					})
					agent.InvalidateSkillsCache()
				}
			}
		} else if cmdArgs != "" && cmdArgs != "list" {
			m.Messages = append(m.Messages, UIMessage{
				Role:    "system",
				Content: "Invalid skills command. Usage: /skills [create [name] | add [repo] | list]",
			})
		} else {
			files, err := os.ReadDir(skillsDir)
			if err != nil {
				m.Messages = append(m.Messages, UIMessage{
					Role:    "system",
					Content: fmt.Sprintf("[Error] Failed to read skills directory: %v", err),
				})
			} else {
				var list []SkillMeta
				for _, f := range files {
					if f.IsDir() {
						continue
					}
					name := f.Name()
					lower := strings.ToLower(name)
					if strings.HasSuffix(lower, ".md") {
						list = append(list, SkillMeta{
							Name:   name,
							Path:   filepath.Join(skillsDir, name),
							Active: true,
						})
					} else if strings.HasSuffix(lower, ".md.disabled") {
						list = append(list, SkillMeta{
							Name:   name,
							Path:   filepath.Join(skillsDir, name),
							Active: false,
						})
					}
				}
				sort.Slice(list, func(i, j int) bool {
					return strings.Compare(strings.ToLower(list[i].Name), strings.ToLower(list[j].Name)) < 0
				})
				m.Skills = list
				m.SkillsCursor = 0
				m.State = StateSkillsMenu
				recalculateViewportHeight(m)
				updateViewportContent(m)
			}
		}
	case "/task", "/tasks", "/taks":
		m.HistorySearchInput.SetValue("")
		m.HistoryRenameMode = false
		m.TaskCursor = 0
		m.PreviousState = m.State
		m.State = StateTasksView
	case "/help":
		helpLines := []string{
			"Commands:",
		}
		for _, info := range AvailableCommands {
			helpLines = append(helpLines, fmt.Sprintf("  %-12s %s", info.Command, info.Description))
		}
		helpLines = append(helpLines, "")
		helpLines = append(helpLines, "Keyboard Shortcuts:")
		helpLines = append(helpLines, "  enter        Send message")
		helpLines = append(helpLines, "  tab          Toggle safe/autonomous mode")
		helpLines = append(helpLines, "  pgup/pgdown  Scroll viewport")
		helpLines = append(helpLines, "  home/end     Scroll to top/bottom")
		helpLines = append(helpLines, "  ctrl+o       Expand/collapse tool call")
		helpLines = append(helpLines, "  ctrl+j       Insert newline in input")
		helpLines = append(helpLines, "  esc          Cancel / interrupt agent")
		helpLines = append(helpLines, "  ctrl+c x2    Exit application")
		helpLines = append(helpLines, "  @            File autocomplete in input")
		m.Messages = append(m.Messages, UIMessage{
			Role:    "system",
			Content: strings.Join(helpLines, "\n"),
		})
		updateViewportContent(m)
	case "/history":
		list, err := ListSessions()
		if err == nil {
			m.Sessions = list
			m.FilteredSessions = list
		}
		m.HistorySearchInput.SetValue("")
		m.HistoryRenameMode = false
		m.HistoryCursor = 0
		m.HistoryPage = 0
		m.PreviousState = m.State
		m.State = StateHistoryView
		m.HistorySearchInput.Focus()
	case "/resume":
		if cmdArgs != "" {
			list, err := ListSessions()
			if err == nil && len(list) > 0 {
				var foundID string
				var idx int
				_, scanErr := fmt.Sscan(cmdArgs, &idx)
				if scanErr == nil && idx >= 1 && idx <= len(list) {
					foundID = list[idx-1].ID
				} else {
					for _, s := range list {
						if s.ID == cmdArgs || strings.HasPrefix(s.ID, cmdArgs) {
							foundID = s.ID
							break
						}
					}
				}

				if foundID != "" {
					sess, loadErr := LoadSession(foundID)
					if loadErr == nil {
						m.ActiveSessionID = sess.ID
						m.ActiveSessionTitle = sess.Title
						m.ActiveSessionCreatedAt = sess.CreatedAt
						m.LastSavedSeq = len(sess.Messages) - 1
						m.Messages = sess.Messages
						m.TokenCount = sess.TokenCount
						m.CompressedTurns = sess.CompressedTurns

						if sess.Provider != "" {
							mgr := provider.NewManager("")
							if p, ok := mgr.Profiles[sess.Provider]; ok {
								mgr.ActiveProfile = sess.Provider
								mgr.Save()

								m.Cfg.Endpoint = p.GetEndpoint()
								m.Cfg.APIKey = p.GetAPIKey()
								m.Cfg.Model = p.GetModel()
								m.Cfg.Save()

								if m.Loop != nil {
									m.Loop.SetClient(client.New(p))
								}
							}
						} else {
							mgr := provider.NewManager("")
							if mgr.ActiveProfile != "github" {
								m.Cfg.Model = sess.Model
								m.Cfg.Save()
							}
						}

						m.Cfg.Mode = sess.Mode
						m.Cfg.Save()

						if m.Loop != nil {
							m.Loop.SetHistory(sess.History)
						}
						tools.SetCurrentSessionID(sess.ID)
						updateViewportContent(m)
						recalculateViewportHeight(m)
						m.Messages = append(m.Messages, UIMessage{
							Role:    "system",
							Content: fmt.Sprintf("Resumed session: '%s'", sess.Title),
						})
						updateViewportContent(m)
						return nil
					}
				}
			}
			m.Messages = append(m.Messages, UIMessage{
				Role:    "system",
				Content: "Session not found: " + cmdArgs,
			})
			updateViewportContent(m)
		} else {
			list, _ := ListSessions()
			m.Sessions = list
			m.FilteredSessions = list
			m.HistorySearchInput.SetValue("")
			m.HistoryRenameMode = false
			m.HistoryCursor = 0
			m.HistoryPage = 0
			m.State = StateHistoryView
			m.HistorySearchInput.Focus()
		}
	case "/indexing":
		if m.Loop != nil {
			result := m.Loop.BuildIndexManual()
			m.Messages = append(m.Messages, UIMessage{
				Role:    "system",
				Content: result,
			})
		}
		updateViewportContent(m)
	case "/gateway":
		if m.GatewayMgr == nil {
			m.Messages = append(m.Messages, UIMessage{
				Role:    "system",
				Content: "Gateway manager not initialized.",
			})
		} else {
			args := strings.Fields(cmdArgs)
			sub := ""
			if len(args) > 0 {
				sub = args[0]
			}
			switch sub {
			case "", "list", "status":
				var daemonInfo string
				if gateway.IsRunning() {
					pid, _ := gateway.ReadPID()
					daemonInfo = fmt.Sprintf("✦ Gateway daemon is running in the background (PID %d).", pid)
				}

				status := m.GatewayMgr.Status()
				if len(status) == 0 {
					msg := "No local gateways running."
					if daemonInfo != "" {
						msg = daemonInfo + "\nManage it using: systemctl --user status awas-gateway"
					} else {
						msg = msg + " Use /gateway start [platform] to start one."
					}
					m.Messages = append(m.Messages, UIMessage{
						Role:    "system",
						Content: msg,
					})
				} else {
					var lines []string
					if daemonInfo != "" {
						lines = append(lines, daemonInfo, "")
					}
					lines = append(lines, "Local Gateway Status:")
					for name, s := range status {
						icon := "🔴"
						if s.Running {
							icon = "🟢"
						}
						lines = append(lines, fmt.Sprintf("  %s %s — %s", icon, name, s.Info))
					}
					m.Messages = append(m.Messages, UIMessage{
						Role:    "system",
						Content: strings.Join(lines, "\n"),
					})
				}
			case "start":
				platform := ""
				if len(args) > 1 {
					platform = args[1]
				}
				if platform == "" {
					m.Messages = append(m.Messages, UIMessage{
						Role:    "system",
						Content: "Usage: /gateway start [platform]\nPlatforms: telegram, discord",
					})
				} else {
					if gateway.IsRunning() {
						m.Messages = append(m.Messages, UIMessage{
							Role:    "system",
							Content: "⚠️ Gateway daemon is already running. Please manage it using 'awas gateway stop' or stop the system service before running gateways locally.",
						})
					} else {
						err := m.GatewayMgr.Start(platform)
						if err != nil {
							m.Messages = append(m.Messages, UIMessage{
								Role:    "system",
								Content: fmt.Sprintf("✘ Failed to start gateway '%s': %v", platform, err),
							})
						} else {
							m.Messages = append(m.Messages, UIMessage{
								Role:    "system",
								Content: fmt.Sprintf("✔ Gateway '%s' started.", platform),
							})
						}
					}
				}
			case "stop":
				platform := ""
				if len(args) > 1 {
					platform = args[1]
				}
				if platform == "" {
					m.Messages = append(m.Messages, UIMessage{
						Role:    "system",
						Content: "Usage: /gateway stop [platform]",
					})
				} else {
					err := m.GatewayMgr.Stop(platform)
					if err != nil {
						m.Messages = append(m.Messages, UIMessage{
							Role:    "system",
							Content: fmt.Sprintf("✘ Failed to stop gateway '%s': %v", platform, err),
						})
					} else {
						m.Messages = append(m.Messages, UIMessage{
							Role:    "system",
							Content: fmt.Sprintf("✔ Gateway '%s' stopped.", platform),
						})
					}
				}
			case "users":
				users := m.GatewayMgr.GetUsers()
				if len(users) == 0 {
					m.Messages = append(m.Messages, UIMessage{
						Role:    "system",
						Content: "No active remote users.",
					})
				} else {
					var lines []string
					lines = append(lines, "Active Remote Users:")
					for platform, sessions := range users {
						for _, s := range sessions {
							lines = append(lines, fmt.Sprintf("  [%s] %s (last active: %s)", platform, s.DisplayName, s.LastActive.Format("15:04")))
						}
					}
					m.Messages = append(m.Messages, UIMessage{
						Role:    "system",
						Content: strings.Join(lines, "\n"),
					})
				}
			case "setup":
				gwCfg := m.GatewayMgr.Load()
				if gwCfg.Platforms == nil {
					gwCfg.Platforms = make(map[string]gateway.Platform)
				}
				plat := gwCfg.Platforms["telegram"]
				plat.Type = "telegram"
				plat.Enabled = true
				gwCfg.Platforms["telegram"] = plat

				dPlat := gwCfg.Platforms["discord"]
				dPlat.Type = "discord"
				dPlat.Enabled = false
				dPlat.Extra = map[string]string{
					"guild_id": "",
				}
				gwCfg.Platforms["discord"] = dPlat
				gwCfg.Enabled = true
				m.GatewayMgr.Save(gwCfg)
				m.Messages = append(m.Messages, UIMessage{
					Role:    "system",
					Content: "Gateway config created at ~/.awas/gateways.json\nEdit the file to add your bot token, then use /gateway start telegram or /gateway start discord.",
				})
			default:
				m.Messages = append(m.Messages, UIMessage{
					Role:    "system",
					Content: "Unknown gateway subcommand. Available: start, stop, status, users, setup",
				})
			}
		}
		updateViewportContent(m)
	default:
		m.Messages = append(m.Messages, UIMessage{
			Role:    "system",
			Content: "Unknown command: " + cmdName + ". Type /help for available commands.",
		})
		updateViewportContent(m)
	}
	return nil
}
