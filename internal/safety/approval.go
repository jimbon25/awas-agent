package safety

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type DangerLevel int

const (
	LevelSafe DangerLevel = iota
	LevelMedium
	LevelDangerous
)

func GetDangerLevel(toolName string) DangerLevel {
	switch toolName {
	case "read_file", "search_code", "list_directory", "invoke_subagent", "send_message", "manage_subagents", "todo_list", "manage_cron", "manage_memory", "manage_skills", "session_search", "system_env", "find_files":
		return LevelSafe
	case "write_file", "edit_file", "patch_file", "git_ops":
		return LevelMedium
	case "execute_command", "delete_file":
		return LevelDangerous
	default:
		return LevelSafe
	}
}

func CheckApproval(toolName string, args string, mode string) bool {
	level := GetDangerLevel(toolName)
	if level == LevelSafe {
		return true
	}

	if level == LevelMedium && mode == "autonomous" {
		fmt.Printf("\u26a0\ufe0f  [Auto-Approved] Tool: %s %s\n", toolName, args)
		return true
	}

	levelStr := "MEDIUM"
	if level == LevelDangerous {
		levelStr = "DANGEROUS"
	}

	fmt.Printf("\n\U0001f6e1\ufe0f  [Approval Required] Level: %s\n", levelStr)
	fmt.Printf("\U0001f449 Tool: %s\n", toolName)
	fmt.Printf("\U0001f4dd Args: %s\n", args)
	fmt.Print("Approve? (y/N): ")

	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	text = strings.TrimSpace(strings.ToLower(text))
	return text == "y" || text == "yes"
}
