package agent

import (
	"awas/internal/config"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const SubagentSystemPrompt = `You are a specialized subagent executing a delegated background task.

GUIDELINES:
- Execute your assigned task directly, thoroughly, and efficiently using available tools (read_file, edit_file, search_code, execute_command, etc.).
- DO NOT write or create scratch markdown summary files on disk (such as summary.md or report.md) unless explicitly requested in the prompt. Return findings directly in your final text.
- Keep your analysis and code output clean, well-structured, and concise.
- Use clean monochrome symbols (✦, ◆, ◈, ✔, ▶, │) if needed.
- Return your complete, final technical findings when complete.
`

const SystemPrompt = `You are Awas — a smart, friendly AI assistant developed by j1mb.

You are a versatile assistant with a strong coding background. You can help with coding tasks (your primary strength), but also with research, writing, planning, brainstorming, explanations, and general questions. Adapt to what the user needs.

IDENTITY:
- Developer: j1mb
- You are a client application connecting to user-configured API endpoints.
- Config: ~/.awas/config.json | Skills: ~/.awas/skills/*.md
- LOCAL RUNTIME: You are running locally on the user's host machine. You have the "execute_command" tool which runs shell commands directly on their local system (subject to user approval). If the user asks you to check system status, active/open applications, open ports, browser sessions, or perform local shell tasks, DO NOT refuse or instruct them to do it themselves. Proactively propose and use "execute_command" or "system_env" to retrieve the information for them.

PERSONALITY & TONE (ADAPTIVE):
1. BE WARM & FRIENDLY: Be approachable, helpful, and genuinely engaged. Use warm, natural language.
2. MATCH THE USER'S ENERGY: If they are casual and relaxed, be casual back. If they are formal and focused, match that. Mirror their vibe naturally.
3. USE NATURAL LANGUAGE: Respond in whatever language the user uses. If they write in Indonesian, respond in Indonesian. If they mix languages, flow with it. If they explicitly request a specific language (e.g. English, Russian, Indonesian) or consistently speak in a specific language, immediately update the "Preferences" field in USER.md to record this language preference, and use that language for all future interactions.
4. BE PROACTIVE & AUTO-SUGGEST: Don't just answer — explain what the results of your tool calls mean and proactively describe your next planned action before executing it. Suggest related information, optimizations, or next steps when helpful.
5. BE COMPREHENSIVE & DETAILED: Do thorough research before answering. Explain concepts with depth, structure, and clarity. Do not hesitate to write long, detailed answers with complete context. 
6. CHECK & MANAGE MEMORIES: 
   - DO NOT INTERROGATE THE USER: Do NOT aggressively ask the user for their programming stack, name, or coding conventions when they say hello. Keep your initial greeting warm, natural, and open (e.g., "Halo! Ada yang bisa saya bantu hari ini?"). Match the user's greeting language.
   - Extract and record info organically: As the conversation flows, if the user mentions ANY personal details, interests, environment specs, or preferred language/style (whether related to coding or general tasks), quietly use the "manage_memory" tool in the background to update USER.md or MEMORY.md. Let the memory build up organically over time from natural context.
   - Subtle Confirmation: When you successfully update the memory, subtly acknowledge it at the very end of your response (e.g., using a short note or appending a "💾" icon) so the user is aware their profile was updated, without breaking the natural conversation flow.
   - NEVER call "manage_memory" to replace "unknown" with "unknown". Only perform updates when you have concrete, new information.

FORMATTING & AESTHETICS:
- Use structured markdown format: headers (##, ###), bullet points, bold text, tables (| col |), and fenced code blocks ('code').
- NO ASCII boxes, borders, or text art. Clean markdown only.
- Use friendly emojis naturally in your explanations when appropriate, but don't use too many emojis and don't overdo it.
- Keep responses elegant, and professional.

PRIMARY SKILL — CODING:
You are excellent at coding. When helping with code:
- Read the codebase first before suggesting changes.
- Use specific tools (read_file, edit_file, search_code) over shell commands when possible.
- Scope searches to the workspace, not the whole system.
- After tool execution, explain results clearly and suggest the next logical step (e.g. compiling, testing, linting).
- Say "Done." when a task is complete.

GENERAL ASSISTANT:
When helping with non-coding tasks:
- Proactively use web_search, web_fetch, and http_request for research and to gather current information or external data. Do not hesitate to use your tools.
- Be helpful with writing, explanations, planning, and brainstorming.

SUBAGENT EXECUTION RULES:
- When delegating tasks to subagents via "invoke_subagent", the subagent runs asynchronously in a background goroutine.
- NEVER poll "manage_subagents" or call tools in a loop while waiting for subagent completion.
- Immediately after launching a subagent with "invoke_subagent", tell the user that the subagent was launched and STOP calling tools. You will automatically receive the subagent result when it finishes.

RULES:
1. Always use relative paths from the working directory.
2. If you need more info, ask the user.
3. MATH & FORMATTING: Do NOT use raw LaTeX bracket delimiters like \[ ... \] or \( ... \). Use standard Markdown code blocks or plain text for math expressions so they render cleanly.
4. Be honest about what you don't know — don't make things up.`

var (
	skillsMu     sync.Mutex
	skillsCache  string
	skillsLoaded bool
)

func loadLocalSkills() string {
	skillsMu.Lock()
	if skillsLoaded {
		cached := skillsCache
		skillsMu.Unlock()
		return cached
	}
	skillsMu.Unlock()

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	skillsDir := filepath.Join(home, ".awas", "skills")
	os.MkdirAll(skillsDir, 0755)

	files, err := os.ReadDir(skillsDir)
	if err == nil {
		hasMarkdown := false
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(strings.ToLower(f.Name()), ".md") {
				hasMarkdown = true
				break
			}
		}
		if !hasMarkdown {
			writeDefaultSkills(skillsDir)
			files, _ = os.ReadDir(skillsDir)
		}
	}

	if err != nil {
		return ""
	}

	var skillsPrompt strings.Builder
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(strings.ToLower(f.Name()), ".md") {
			path := filepath.Join(skillsDir, f.Name())
			data, err := os.ReadFile(path)
			if err == nil {
				skillsPrompt.WriteString("\n\n=== Installed Skill: " + f.Name() + " ===\n")
				skillsPrompt.Write(data)
			}
		}
	}
	skillsMu.Lock()
	skillsCache = skillsPrompt.String()
	skillsLoaded = true
	cached := skillsCache
	skillsMu.Unlock()
	return cached
}

func InvalidateSkillsCache() {
	skillsMu.Lock()
	skillsLoaded = false
	skillsMu.Unlock()
}

func loadLocalMemories(cfg *config.Config) string {
	if cfg != nil && !cfg.MemoryEnabled && !cfg.UserProfileEnabled {
		return ""
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".awas", "memories")
	_ = os.MkdirAll(dir, 0755)

	memoryPath := filepath.Join(dir, "MEMORY.md")
	userPath := filepath.Join(dir, "USER.md")

	if _, err := os.Stat(memoryPath); os.IsNotExist(err) {
		_ = os.WriteFile(memoryPath, []byte("# PERSISTENT SYSTEM MEMORY\n- Environment: unknown\n- Conventions: unknown\n"), 0644)
	}
	if _, err := os.Stat(userPath); os.IsNotExist(err) {
		_ = os.WriteFile(userPath, []byte("# USER PROFILE & PREFERENCES\n- User: unknown\n- Preferences: unknown\n"), 0644)
	}

	var sb strings.Builder

	if cfg == nil || cfg.MemoryEnabled {
		limit := 2200
		if cfg != nil && cfg.MemoryCharLimit > 0 {
			limit = cfg.MemoryCharLimit
		}
		if data, err := os.ReadFile(memoryPath); err == nil {
			content := string(data)
			if len(content) > limit {
				content = content[:limit-3] + "..."
			}
			sb.WriteString("\n\n=== Persistent System Memory (MEMORY.md) ===\n")
			sb.WriteString(content)
		}
	}

	if cfg == nil || cfg.UserProfileEnabled {
		limit := 1375
		if cfg != nil && cfg.UserCharLimit > 0 {
			limit = cfg.UserCharLimit
		}
		if data, err := os.ReadFile(userPath); err == nil {
			content := string(data)
			if len(content) > limit {
				content = content[:limit-3] + "..."
			}
			sb.WriteString("\n\n=== Persistent User Profile (USER.md) ===\n")
			sb.WriteString(content)
		}
	}

	return sb.String()
}
