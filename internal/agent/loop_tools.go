package agent

import "awas/internal/client"

func getTools() []client.Tool {
	return []client.Tool{
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "execute_command",
				Description: "Run a bash command in the workspace directory",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command": map[string]any{
							"type":        "string",
							"description": "The command to run",
						},
					},
					"required": []string{"command"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "read_file",
				Description: "Read file contents relative to the workspace directory",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "Path to the file to read",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "edit_file",
				Description: "Find and replace text in a file. If old_string matches multiple places and replace_all is false, it returns an error.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"file_path": map[string]any{
							"type":        "string",
							"description": "Path to the file to edit",
						},
						"old_string": map[string]any{
							"type":        "string",
							"description": "The exact string content to be replaced",
						},
						"new_string": map[string]any{
							"type":        "string",
							"description": "The new replacement content",
						},
						"replace_all": map[string]any{
							"type":        "boolean",
							"description": "If true, replace all occurrences of old_string",
							"default":     false,
						},
					},
					"required": []string{"file_path", "old_string", "new_string"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "write_file",
				Description: "Create or overwrite a file relative to workspace",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "Path to file to write",
						},
						"content": map[string]any{
							"type":        "string",
							"description": "Text content to write into file",
						},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "search_code",
				Description: "Search code patterns inside workspace using grep/ripgrep",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"pattern": map[string]any{
							"type":        "string",
							"description": "The text pattern to search for",
						},
						"path": map[string]any{
							"type":        "string",
							"description": "Optional subdirectory path to search within",
						},
						"include": map[string]any{
							"type":        "string",
							"description": "Optional glob pattern for file matching (e.g. *.go)",
						},
						"max_results": map[string]any{
							"type":        "integer",
							"description": "Maximum match items to return (default 10)",
						},
					},
					"required": []string{"pattern"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "list_directory",
				Description: "List files and directories in path relative to workspace",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "Directory path to list",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "delete_file",
				Description: "Delete a file relative to workspace",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "Path to file to delete",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "search_symbols",
				Description: "Search function/type/struct/interface definitions across the entire workspace directory",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{
							"type":        "string",
							"description": "The symbol name substring to search for (case-insensitive)",
						},
						"kind": map[string]any{
							"type":        "string",
							"description": "Optional filter by symbol kind (struct, interface, function, method, const, var)",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "restore_file",
				Description: "Restore a file to a previous version using undo/redo history",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "The relative path to the file to restore",
						},
						"revision": map[string]any{
							"type":        "integer",
							"description": "How many steps back to undo, default is 1",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "http_request",
				Description: "Send HTTP requests to APIs and web services",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"method": map[string]any{
							"type":        "string",
							"description": "HTTP method (GET, POST, PUT, DELETE, PATCH)",
						},
						"url": map[string]any{
							"type":        "string",
							"description": "The URL to send request to",
						},
						"headers": map[string]any{
							"type":        "string",
							"description": "Optional headers as JSON string (e.g. {\"Authorization\": \"Bearer xxx\"})",
						},
						"body": map[string]any{
							"type":        "string",
							"description": "Optional request body",
						},
					},
					"required": []string{"method", "url"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "sql_query",
				Description: "Execute SQL queries on databases (SQLite, PostgreSQL, MySQL)",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"driver": map[string]any{
							"type":        "string",
							"description": "Database driver (sqlite, postgres, mysql)",
						},
						"dsn": map[string]any{
							"type":        "string",
							"description": "Data source name (connection string)",
						},
						"query": map[string]any{
							"type":        "string",
							"description": "SQL query to execute",
						},
					},
					"required": []string{"driver", "dsn", "query"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "web_search",
				Description: "Search the web for information using DuckDuckGo or SearXNG",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{
							"type":        "string",
							"description": "The search query",
						},
						"max_results": map[string]any{
							"type":        "integer",
							"description": "Maximum number of results (default 5)",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "download_file",
				Description: "Download a file from a URL to a local path",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"url": map[string]any{
							"type":        "string",
							"description": "The URL to download from",
						},
						"path": map[string]any{
							"type":        "string",
							"description": "Local file path to save to",
						},
					},
					"required": []string{"url", "path"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "web_fetch",
				Description: "Fetch a web page and return its clean, readable text content. Use this to read the actual content of a URL (strips scripts/styles/boilerplate). Prefer web_extract when you need specific data by CSS selector.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"url": map[string]any{
							"type":        "string",
							"description": "The URL to fetch",
						},
					},
					"required": []string{"url"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "web_extract",
				Description: "Fetch a web page and extract the visible text of every element matching a CSS selector. Use this to scrape specific data (e.g. prices, headings, links) from a page.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"url": map[string]any{
							"type":        "string",
							"description": "The URL to fetch and extract from",
						},
						"selector": map[string]any{
							"type":        "string",
							"description": "CSS selector to match elements, e.g. \"h1\", \".price\", \"a.title\", \"table tr td\"",
						},
					},
					"required": []string{"url", "selector"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "manage_cron",
				Description: "Manage scheduled cron jobs for the current user. Allows creating, listing, deleting, enabling, or disabling automated tasks.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"action": map[string]any{
							"type":        "string",
							"description": "The action to perform: 'create', 'list', 'delete', 'enable', or 'disable'",
							"enum":        []string{"create", "list", "delete", "enable", "disable"},
						},
						"name": map[string]any{
							"type":        "string",
							"description": "The name of the cron job (required for delete, enable, disable; optional/autogenerated for create)",
						},
						"schedule": map[string]any{
							"type":        "string",
							"description": "The execution schedule (required for create). Supports standard 5-field cron (e.g. '0 9 * * *') or human-readable formats (e.g. 'every 30m', 'every 2h', 'daily at 9am', 'daily at 14:30', 'today at 14:20', 'tomorrow at 9am')",
						},
						"prompt": map[string]any{
							"type":        "string",
							"description": "The instruction prompt that the agent will run on the schedule (required for create)",
						},
					},
					"required": []string{"action"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "ask_user",
				Description: "Ask the user a clarifying question or request sensitive input (such as API keys). Blocks until the user replies.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"question": map[string]any{
							"type":        "string",
							"description": "The question or request to send to the user",
						},
					},
					"required": []string{"question"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "git_ops",
				Description: "Perform common Git operations in the workspace repository safely.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"action": map[string]any{
							"type":        "string",
							"enum":        []string{"status", "diff", "commit", "push", "pull", "checkout", "log"},
							"description": "The Git action to perform",
						},
						"message": map[string]any{
							"type":        "string",
							"description": "Commit message (required for 'commit')",
						},
						"branch": map[string]any{
							"type":        "string",
							"description": "Target branch name (for 'checkout', 'push', 'pull')",
						},
					},
					"required": []string{"action"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "lint_and_format",
				Description: "Format files or check syntax/errors for languages like Go, JSON, Python, etc.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "Relative path of the file to lint or format",
						},
						"action": map[string]any{
							"type":        "string",
							"enum":        []string{"format", "lint"},
							"description": "Whether to format the code or check for syntax errors",
						},
					},
					"required": []string{"path", "action"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "find_files",
				Description: "Search for files in the workspace matching a glob pattern (excludes vendor and node_modules).",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"pattern": map[string]any{
							"type":        "string",
							"description": "The search pattern (e.g., '**/*.go', 'config/*.json', 'main.*')",
						},
					},
					"required": []string{"pattern"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "system_env",
				Description: "Query the system environment to list available compilers (go, node, python), active ports, and OS details.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "send_file",
				Description: "Upload a local file (e.g., screenshot, code file, log, or zip archive) directly to the Telegram/Discord chat so the user can download or view it.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "Path to the file to upload",
						},
						"caption": map[string]any{
							"type":        "string",
							"description": "Optional description or message to accompany the file",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "patch_file",
				Description: "Apply multiple search-and-replace changes (patches) to a file. Useful for multi-block modifications.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "Path to the file to patch",
						},
						"patches": map[string]any{
							"type": "array",
							"description": "List of patch chunks to apply",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"old_string": map[string]any{
										"type":        "string",
										"description": "The exact string segment to replaceSegment. Must be unique in the file.",
									},
									"new_string": map[string]any{
										"type":        "string",
										"description": "The replacement string content.",
									},
								},
								"required": []string{"old_string", "new_string"},
							},
						},
					},
					"required": []string{"path", "patches"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "session_search",
				Description: "Search message history across all past conversation sessions using a keywords query.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{
							"type":        "string",
							"description": "Keywords to search in conversation histories",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "todo_list",
				Description: "Manage a persistent TODO task list for tracking goals, steps, and progress in the current workspace.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"action": map[string]any{
							"type":        "string",
							"description": "Action to perform: 'add', 'update', or 'list'",
						},
						"id": map[string]any{
							"type":        "integer",
							"description": "ID of the task to update (required only for 'update' action)",
						},
						"task": map[string]any{
							"type":        "string",
							"description": "The task description text",
						},
						"status": map[string]any{
							"type":        "string",
							"description": "The status of the task to set: 'pending' or 'completed' (required only for 'update' action)",
						},
					},
					"required": []string{"action"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "manage_memory",
				Description: "Read, add, replace, or remove persistent memories (environment facts, rules, conventions, or user profile preferences) to maintain state across conversation sessions.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"action": map[string]any{
							"type":        "string",
							"description": "Action to perform: 'add' (append a memory), 'replace' (update a memory), 'remove' (delete a memory), or 'read' (read current memory files)",
						},
						"target": map[string]any{
							"type":        "string",
							"description": "Target memory file: 'memory' (MEMORY.md) or 'user' (USER.md)",
						},
						"content": map[string]any{
							"type":        "string",
							"description": "The memory content text to add or use as a replacement",
						},
						"old_text": map[string]any{
							"type":        "string",
							"description": "The exact old text segment to replace or remove (required only for 'replace' and 'remove' actions)",
						},
					},
					"required": []string{"action", "target"},
				},
			},
		},
		{
			Type: "function",
			Function: client.ToolDefinitionInfo{
				Name:        "manage_skills",
				Description: "Manage, create, or patch local skill guideline markdown files stored in ~/.awas/skills/ to adapt coding standards, logging styles, or workflows.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"action": map[string]any{
							"type":        "string",
							"description": "Action to perform: 'create' (create new skill file), 'patch' (modify/replace content in an existing skill), or 'list' (list installed skills)",
						},
						"name": map[string]any{
							"type":        "string",
							"description": "The filename of the skill (e.g., 'go_patterns.md')",
						},
						"content": map[string]any{
							"type":        "string",
							"description": "The markdown content to write or the replacement text",
						},
						"old_text": map[string]any{
							"type":        "string",
							"description": "The exact old text block to replace (required only for 'patch' action)",
						},
					},
					"required": []string{"action"},
				},
			},
		},
	}
}

func getChatTools() []client.Tool {
	var chatTools []client.Tool
	allTools := getTools()
	for _, t := range allTools {
		if t.Function.Name == "web_search" ||
			t.Function.Name == "web_fetch" ||
			t.Function.Name == "web_extract" ||
			t.Function.Name == "http_request" ||
			t.Function.Name == "system_env" ||
			t.Function.Name == "send_file" ||
			t.Function.Name == "session_search" ||
			t.Function.Name == "todo_list" ||
			t.Function.Name == "manage_memory" ||
			t.Function.Name == "manage_skills" ||
			t.Function.Name == "ask_user" {
			chatTools = append(chatTools, t)
		}
	}
	return chatTools
}
