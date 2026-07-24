package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"awas/internal/client"
	"awas/internal/index"
	"awas/internal/tools"
)

func (l *Loop) executeTool(ctx context.Context, toolCall client.ToolCall) string {
	var args map[string]any
	err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args)
	if err != nil {
		return fmt.Sprintf("[Error] failed to parse arguments: %v", err)
	}

	var result string
	switch toolCall.Function.Name {
	case "manage_cron":
		action, _ := args["action"].(string)
		name, _ := args["name"].(string)
		schedule, _ := args["schedule"].(string)
		prompt, _ := args["prompt"].(string)

		platform, _ := ctx.Value("platform").(string)
		chatID, _ := ctx.Value("chat_id").(string)
		guildID, _ := ctx.Value("guild_id").(string)

		if platform == "" {
			platform = "all"
		}

		result = tools.ManageCron(action, name, schedule, prompt, platform, chatID, guildID)
	case "ask_user":
		question, _ := args["question"].(string)
		res, err := l.UI.AskUser(ctx, question)
		if err != nil {
			result = fmt.Sprintf("[Error] failed to get user response: %v", err)
		} else {
			result = res
		}
	case "git_ops":
		action, _ := args["action"].(string)
		msg, _ := args["message"].(string)
		branch, _ := args["branch"].(string)
		result = tools.GitOps(l.cfg.WorkDir, action, msg, branch)
	case "lint_and_format":
		path, _ := args["path"].(string)
		action, _ := args["action"].(string)
		result = tools.LintAndFormat(l.cfg.WorkDir, resolveHomePath(path), action)
	case "find_files":
		pattern, _ := args["pattern"].(string)
		result = tools.FindFiles(l.cfg.WorkDir, pattern)
	case "system_env":
		result = tools.SystemEnv()
	case "execute_command":
		cmd, _ := args["command"].(string)
		result = tools.ExecuteCommand(l.cfg.WorkDir, cmd)
	case "read_file":
		path, _ := args["path"].(string)
		result = tools.ReadFile(l.cfg.WorkDir, resolveHomePath(path))
	case "edit_file":
		filePath, _ := args["file_path"].(string)
		oldStr, _ := args["old_string"].(string)
		newStr, _ := args["new_string"].(string)
		replaceAll, _ := args["replace_all"].(bool)
		result = tools.EditFile(l.cfg.WorkDir, resolveHomePath(filePath), oldStr, newStr, replaceAll)
	case "write_file":
		path, _ := args["path"].(string)
		content, _ := args["content"].(string)
		result = tools.WriteFile(l.cfg.WorkDir, resolveHomePath(path), content)
	case "search_code":
		pattern, _ := args["pattern"].(string)
		path, _ := args["path"].(string)
		include, _ := args["include"].(string)
		maxResultsVal, hasMaxResults := args["max_results"]
		maxResults := 10
		if hasMaxResults {
			if mr, ok := maxResultsVal.(float64); ok {
				maxResults = int(mr)
			}
		}
		result = tools.SearchCode(l.cfg.WorkDir, pattern, resolveHomePath(path), include, maxResults)
	case "list_directory":
		path, _ := args["path"].(string)
		result = tools.ListDirectory(l.cfg.WorkDir, resolveHomePath(path))
	case "delete_file":
		path, _ := args["path"].(string)
		result = tools.DeleteFile(l.cfg.WorkDir, resolveHomePath(path))
	case "search_symbols":
		query, _ := args["query"].(string)
		kind, _ := args["kind"].(string)
		if l.Index == nil {
			return "[Error] index is not ready yet. Please wait a few seconds and try again."
		}
		syms := index.SearchSymbols(l.Index, query, kind)
		if len(syms) == 0 {
			return "No matching symbols found."
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Found %d matching symbols:\n", len(syms)))
		for _, s := range syms {
			if s.Kind == "method" {
				sb.WriteString(fmt.Sprintf("- %s (method on %s) in %s:%d\n", s.Name, s.Receiver, s.File, s.Line))
			} else {
				sb.WriteString(fmt.Sprintf("- %s (%s) in %s:%d\n", s.Name, s.Kind, s.File, s.Line))
			}
			if s.Doc != "" {
				sb.WriteString(fmt.Sprintf("  Doc: %s\n", s.Doc))
			}
		}
		result = sb.String()
	case "restore_file":
		path, _ := args["path"].(string)
		revisionVal, hasRevision := args["revision"]
		revision := 1
		if hasRevision {
			if r, ok := revisionVal.(float64); ok {
				revision = int(r)
			}
		}
		res, err := tools.RestoreFile(l.cfg.WorkDir, path, revision)
		if err != nil {
			result = fmt.Sprintf("[Error] failed to restore file: %v", err)
		} else {
			result = res
		}
	case "http_request":
		method, _ := args["method"].(string)
		url, _ := args["url"].(string)
		headers, _ := args["headers"].(string)
		body, _ := args["body"].(string)
		result = tools.HTTPRequest(method, url, headers, body)
	case "sql_query":
		driver, _ := args["driver"].(string)
		dsn, _ := args["dsn"].(string)
		query, _ := args["query"].(string)
		result = tools.SQLQuery(driver, dsn, query)
	case "web_search":
		query, _ := args["query"].(string)
		maxResultsVal, hasMax := args["max_results"]
		maxResults := 5
		if hasMax {
			if mr, ok := maxResultsVal.(float64); ok {
				maxResults = int(mr)
			}
		}
		result = tools.WebSearch(query, maxResults)
	case "download_file":
		dlURL, _ := args["url"].(string)
		path, _ := args["path"].(string)
		result = tools.DownloadFile(dlURL, path)
	case "web_fetch":
		url, _ := args["url"].(string)
		result = tools.WebFetch(url)
	case "web_extract":
		url, _ := args["url"].(string)
		selector, _ := args["selector"].(string)
		result = tools.WebExtract(url, selector)
	case "send_file":
		path, _ := args["path"].(string)
		caption, _ := args["caption"].(string)
		err := l.UI.SendFile(resolveHomePath(path), caption)
		if err != nil {
			result = fmt.Sprintf("[Error] failed to send file: %v", err)
		} else {
			result = "File sent successfully."
		}
	case "patch_file":
		path, _ := args["path"].(string)
		patchesVal, _ := args["patches"]
		var patches []tools.PatchChunk
		if data, err := json.Marshal(patchesVal); err == nil {
			json.Unmarshal(data, &patches)
		}
		result = tools.PatchFile(l.cfg.WorkDir, resolveHomePath(path), patches)
	case "session_search":
		query, _ := args["query"].(string)
		result = tools.SessionSearch(query)
	case "todo_list":
		action, _ := args["action"].(string)
		idVal, hasID := args["id"]
		var id int
		if hasID {
			if idF, ok := idVal.(float64); ok {
				id = int(idF)
			}
		}
		task, _ := args["task"].(string)
		status, _ := args["status"].(string)
		result = tools.ManageTodo(l.cfg.WorkDir, action, id, task, status)
	case "manage_memory":
		action, _ := args["action"].(string)
		target, _ := args["target"].(string)
		content, _ := args["content"].(string)
		oldText, _ := args["old_text"].(string)
		result = tools.ManageMemory(action, target, content, oldText)
	case "manage_skills":
		action, _ := args["action"].(string)
		name, _ := args["name"].(string)
		content, _ := args["content"].(string)
		oldText, _ := args["old_text"].(string)
		result = tools.ManageSkills(action, name, content, oldText)
	default:
		result = fmt.Sprintf("[Error] unknown tool: %s", toolCall.Function.Name)
	}

	return result
}

func resolveHomePath(p string) string {
	if p == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}
	}
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func ClassifyToolError(toolName string, result string) ErrorType {
	if result == "" || !strings.HasPrefix(result, "[Error]") {
		return ErrNoError
	}
	lower := strings.ToLower(result)
	switch toolName {
	case "execute_command":
		if strings.Contains(lower, "not found") || strings.Contains(lower, "no such file or directory") {
			return ErrCommandNotFound
		}
		if strings.Contains(lower, "permission denied") {
			return ErrPermission
		}
		if strings.Contains(lower, "killed") || strings.Contains(lower, "out of memory") {
			return ErrOOM
		}
		if strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded") {
			return ErrTimeout
		}
	case "read_file":
		if strings.Contains(lower, "no such file") || strings.Contains(lower, "not found") || strings.Contains(lower, "does not exist") {
			return ErrFileNotFound
		}
		if strings.Contains(lower, "permission denied") || strings.Contains(lower, "access denied") {
			return ErrPermission
		}
	case "edit_file":
		if strings.Contains(lower, "no such file") || strings.Contains(lower, "not found") || strings.Contains(lower, "does not exist") {
			return ErrFileNotFound
		}
		if strings.Contains(lower, "pattern not found") || strings.Contains(lower, "old_string not found") || strings.Contains(lower, "no match") {
			return ErrPatternNotFound
		}
		if strings.Contains(lower, "permission denied") {
			return ErrPermission
		}
	case "write_file":
		if strings.Contains(lower, "permission denied") || strings.Contains(lower, "read-only") {
			return ErrPermission
		}
		if strings.Contains(lower, "no space left") {
			return ErrOOM
		}
	case "search_code":
		if strings.Contains(lower, "no such file") || strings.Contains(lower, "does not exist") {
			return ErrFileNotFound
		}
	case "http_request":
		if strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded") {
			return ErrTimeout
		}
		if strings.Contains(lower, "connection refused") {
			return ErrCommandNotFound
		}
	}
	if strings.Contains(lower, "undefined") || strings.Contains(lower, "syntax error") ||
		strings.Contains(lower, "cannot find symbol") || strings.Contains(lower, "unresolved reference") {
		return ErrCompilation
	}
	return ErrUnknown
}

