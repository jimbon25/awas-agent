package agent

import (
	"awas/internal/client"
	"awas/internal/index"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (l *Loop) UpdateSystemPromptWithContext() {
	if l.Index == nil || len(l.history) == 0 {
		return
	}

	totalFiles := len(l.Index.Files)
	totalLines := 0
	for _, f := range l.Index.Files {
		totalLines += f.Lines
	}
	totalSymbols := len(l.Index.Symbols)
	totalDirs := len(l.Index.Dirs)

	pkgsMap := make(map[string]bool)
	for _, f := range l.Index.Files {
		if f.Package != "" {
			pkgsMap[f.Package] = true
		}
	}
	totalPackages := len(pkgsMap)

	contextSnippet := fmt.Sprintf("\n\nPROJECT CONTEXT:\n- %d files detected (%d lines of code)\n- %d packages/modules across %d directories\n- %d symbols indexed (use search_symbols to look up definitions)",
		totalFiles, totalLines, totalPackages, totalDirs, totalSymbols)

	instructions := ""
	guideFiles := []string{"AGENTS.md", "CLAUDE.md", ".HERMES.md", ".AWAS.md"}
	for _, filename := range guideFiles {
		filePath := filepath.Join(l.cfg.WorkDir, filename)
		if data, err := os.ReadFile(filePath); err == nil {
			instructions += fmt.Sprintf("\n\n=== Project Rulebook: %s ===\n%s\n", filename, string(data))
		}
	}

	l.history[0].Content = SystemPrompt + loadLocalSkills() + loadLocalMemories(l.cfg) + contextSnippet + instructions
}

func (l *Loop) BuildIndexManual() string {
	idx, err := index.LoadIndex(l.cfg.WorkDir)
	if err != nil {
		idx, err = index.BuildIndex(l.cfg.WorkDir)
		if err != nil {
			return fmt.Sprintf("[Error] Failed to build index: %v", err)
		}
		index.SaveIndex(l.cfg.WorkDir, idx)
	}
	l.Index = idx
	l.UpdateSystemPromptWithContext()
	return fmt.Sprintf("Index built: %d files, %d symbols, %d directories",
		len(idx.Files), len(idx.Symbols), len(idx.Dirs))
}

const PlanningSystemPrompt = `You are a professional software engineering planner.
Your goal is to split the user request into a step-by-step plan.
Respond ONLY with a JSON object matching the following structure:
{
  "goal": "Brief description of what we want to achieve",
  "steps": [
    {
      "id": "step-1",
      "description": "Short explanation of this step",
      "tool": "The name of the tool to execute. Choose from: read_file, write_to_file, replace_file_content, multi_replace_file_content, execute_command, manage_cron, ask_user, git_ops, lint_and_format, find_files, system_env.",
      "args": { ... arguments for the tool ... },
      "depends_on": []
    }
  ]
}
Do not include markdown tags, formatting, or extra text outside the JSON block.`

const ReviewSystemPrompt = `You are an AI code reviewer.
Your goal is to inspect the output of the executed tool and determine if it succeeded in its objective or if there are issues.
Respond ONLY with a JSON object matching the following structure:
{
  "success": true,
  "issues": ["list of issues found, if any"],
  "should_retry": false,
  "strategy": "retry_same"
}
Or if it failed and should retry:
{
  "success": false,
  "issues": ["compilation failed on main.go:12"],
  "should_retry": true,
  "strategy": "modify_args",
  "args": { "path": "main.go" }
}
Available strategies are: "retry_same", "modify_args", "try_alternative", "abort".
If strategy is "modify_args", you MUST provide the corrected arguments in the "args" field (matching the schema of the tool).
If strategy is "try_alternative", you MUST specify the new tool name in the "tool" field and its arguments in the "args" field.
Do not include markdown tags or extra text outside the JSON block.`

func (l *Loop) generatePlan(ctx context.Context, userInput string) (*Plan, error) {
	planningHistory := []client.Message{
		{
			Role:    "system",
			Content: PlanningSystemPrompt,
		},
		{
			Role:    "user",
			Content: fmt.Sprintf("Please create a plan to achieve this goal: %s", userInput),
		},
	}

	choice, _, err := l.cli.Send(ctx, planningHistory, nil)
	if err != nil {
		return nil, err
	}

	rawJSON := choice.Message.Content
	rawJSON = cleanJSONString(rawJSON)

	var plan Plan
	err = json.Unmarshal([]byte(rawJSON), &plan)
	if err != nil {
		return nil, fmt.Errorf("failed to parse plan JSON: %v. Raw response: %s", err, rawJSON)
	}

	if len(plan.Steps) == 0 {
		return nil, fmt.Errorf("plan has 0 steps")
	}

	for i := range plan.Steps {
		plan.Steps[i].Status = StepPending
	}

	return &plan, nil
}

func (l *Loop) reviewStepResult(ctx context.Context, goal string, step *PlanStep, result string) *ReviewResult {
	reviewHistory := []client.Message{
		{
			Role:    "system",
			Content: ReviewSystemPrompt,
		},
		{
			Role:    "user",
			Content: fmt.Sprintf("Original Goal: %s\nExecuted Step: %s\nTool Used: %s\nArguments: %+v\nResult Output:\n%s\n\nPlease evaluate if this step succeeded or if we need to retry.", goal, step.Description, step.Tool, step.Args, result),
		},
	}

	choice, _, err := l.cli.Send(ctx, reviewHistory, nil)
	if err != nil {
		return &ReviewResult{Success: false, ShouldRetry: false, Strategy: StrategyAbort}
	}

	rawJSON := cleanJSONString(choice.Message.Content)
	var rev ReviewResult
	err = json.Unmarshal([]byte(rawJSON), &rev)
	if err != nil {
		return &ReviewResult{Success: false, ShouldRetry: false, Strategy: StrategyAbort}
	}

	return &rev
}

func cleanJSONString(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimSuffix(s, "```")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}

func (l *Loop) printInterruptOrTimeout(ctx context.Context) {
	if ctx.Err() == context.DeadlineExceeded {
		l.UI.PrintMessage("system", "Process timed out (exceeded limit).")
	} else {
		l.UI.PrintMessage("system", "Process interrupted by user.")
	}
}
