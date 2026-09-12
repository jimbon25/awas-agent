package safety

import "testing"

func TestGetDangerLevel(t *testing.T) {
	// Fail-secure: unknown tools must default to LevelDangerous
	unknowns := []string{"unknown_tool", "random_bash", "", "eval"}
	for _, tool := range unknowns {
		if got := GetDangerLevel(tool); got != LevelDangerous {
			t.Errorf("GetDangerLevel(%q) = %v; want LevelDangerous (%v)", tool, got, LevelDangerous)
		}
	}

	// Explicit dangerous tools
	dangerous := []string{"execute_command", "delete_file"}
	for _, tool := range dangerous {
		if got := GetDangerLevel(tool); got != LevelDangerous {
			t.Errorf("GetDangerLevel(%q) = %v; want LevelDangerous (%v)", tool, got, LevelDangerous)
		}
	}

	// Explicit medium tools
	medium := []string{"download_file", "sql_query", "http_request", "lint_and_format", "git_ops", "write_file", "edit_file", "patch_file"}
	for _, tool := range medium {
		if got := GetDangerLevel(tool); got != LevelMedium {
			t.Errorf("GetDangerLevel(%q) = %v; want LevelMedium (%v)", tool, got, LevelMedium)
		}
	}

	// Explicit safe tools
	safe := []string{"read_file", "search_code", "list_directory", "invoke_subagent", "send_message", "manage_subagents", "todo_list", "manage_cron", "manage_memory", "manage_skills", "session_search", "system_env", "find_files"}
	for _, tool := range safe {
		if got := GetDangerLevel(tool); got != LevelSafe {
			t.Errorf("GetDangerLevel(%q) = %v; want LevelSafe (%v)", tool, got, LevelSafe)
		}
	}
}
