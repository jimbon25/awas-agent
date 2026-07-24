package agent

import (
	"os"
	"path/filepath"
)

func writeDefaultSkills(dir string) {
	codeOptContent := `# Skill: Go Error Handling & Logging
Guidelines:
- Always handle returned errors explicitly; never use '_' to discard errors unless in defer.
- Return early on error, don't nest success path inside 'if err == nil'.
- Use 'fmt.Errorf("context: %w", err)' for error wrapping to preserve error chain.
- Don't panic in library code; panic only for fatal startup failures.
- Log via structured logger, not 'fmt.Println'.`

	gitHelperContent := `# Skill: Git Workflow & Collaboration
Guidelines:
- Commit messages: use imperative mood ("Add feature", not "Added feature").
- Follow Conventional Commits: 'feat:', 'fix:', 'refactor:', 'test:', 'docs:', 'chore:'.
- Subject line max 50 characters, body wrap at 72 characters.
- Separate subject and body with a blank line if additional context is needed.
- Don't commit irrelevant files (.env, binary, node_modules, etc.) — use .gitignore.
- Keep commits small and focused: one logical change per commit.
- When rebasing, squash fixup/WIP commits before merging.
- Branch naming: 'feat/nama-fitur', 'fix/apa-yang-diperbaiki', 'refactor/bagian'.
- Write PR descriptions: what problem, how it's solved, screenshot for UI changes.
- Avoid merge commits; prefer rebase or squash merge for clean history.`

	tuiContent := `# Skill: TUI Development (Bubble Tea)
Guidelines:
- Keep Model, Update, and View clearly separated.
- Don't block in Update; send commands for async work.
- Handle window resize via tea.WindowSizeMsg.
- Avoid goroutine leaks; always clean up via model.Cancel or tea.Quit.
- Use tea.Batch to run multiple commands concurrently.`

	safeExecContent := `# Skill: Safe Command Execution
Guidelines:
- Avoid destructive commands ('rm -rf', 'curl | bash') without user confirmation.
- Always validate or escape user input before passing into shell commands.
- Use timeouts to prevent hanging processes.
- Prefer specific tools (git, go, npm) over raw shell piped chains.`

	codeReviewContent := `# Skill: Code Review & Refactoring
Guidelines:
- Prioritize readability over cleverness: clear code beats clever code.
- Replace magic numbers with named constants.
- Keep functions under 50 lines; extract into smaller descriptive functions.
- Identify tight coupling and suggest dependency injection.
- Look for unused code, dead parameters, and redundant comments.`

	os.WriteFile(filepath.Join(dir, "code_optimizer.md"), []byte(codeOptContent), 0644)
	os.WriteFile(filepath.Join(dir, "git_helper.md"), []byte(gitHelperContent), 0644)
	os.WriteFile(filepath.Join(dir, "tui_patterns.md"), []byte(tuiContent), 0644)
	os.WriteFile(filepath.Join(dir, "safe_exec.md"), []byte(safeExecContent), 0644)
	os.WriteFile(filepath.Join(dir, "code_review.md"), []byte(codeReviewContent), 0644)
}
