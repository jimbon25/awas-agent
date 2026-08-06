package gateway

import (
	"awas/internal/cron"
	"fmt"
	"strings"
	"time"
)

func HandleCronCommand(store *cron.Store, scheduler *cron.Scheduler, platform string, chatID string, guildID string, args []string) string {
	if len(args) == 0 {
		return showCronHelp()
	}

	subCmd := strings.ToLower(args[0])
	switch subCmd {
	case "create":
		return handleCronCreate(store, platform, chatID, guildID, args[1:])

	case "list":
		return handleCronList(store, chatID)

	case "delete":
		if len(args) < 2 {
			return "✘ Please specify the job name to delete. Example: `/cron delete my-job`"
		}
		name := args[1]
		err := store.DeleteJob(name)
		if err != nil {
			return fmt.Sprintf("✘ Failed to delete job %q: %v", name, err)
		}
		return fmt.Sprintf("✔ Job %q deleted successfully.", name)

	case "enable", "disable":
		if len(args) < 2 {
			return fmt.Sprintf("✘ Please specify the job name. Example: `/cron %s my-job`", subCmd)
		}
		name := args[1]
		job, err := store.GetJob(name)
		if err != nil {
			return fmt.Sprintf("✘ Job %q not found.", name)
		}
		job.Enabled = (subCmd == "enable")
		err = store.SaveJob(job)
		if err != nil {
			return fmt.Sprintf("✘ Failed to update job status: %v", err)
		}
		status := "enabled"
		if !job.Enabled {
			status = "disabled"
		}
		return fmt.Sprintf("✔ Job %q has been %s.", name, status)

	case "run":
		if len(args) < 2 {
			return "✘ Please specify the job name to run. Example: `/cron run my-job`"
		}
		name := args[1]
		if scheduler == nil {
			return "✘ Cron scheduler is not running (gateway daemon offline). Start the gateway first, or wait for the scheduled time."
		}
		if _, err := store.GetJob(name); err != nil {
			return fmt.Sprintf("✘ Job %q not found.", name)
		}
		if _, err := scheduler.RunJob(name); err != nil {
			return fmt.Sprintf("✘ Failed to trigger job %q: %v", name, err)
		}
		return fmt.Sprintf("✦ Job %q triggered. It will run in the background shortly.", name)

	default:
		query := strings.Join(args, " ")
		return handleCronNaturalLanguage(store, platform, chatID, guildID, query)
	}
}

func showCronHelp() string {
	return `⎔ *Cron Scheduler Commands:*
• ` + "`" + `/cron create "<schedule>" "<prompt>" [--name <name>]` + "`" + `
  Create a new job (e.g. ` + "`" + `/cron create "every 30m" "Cek status server"` + "`" + `).
• ` + "`" + `/cron list` + "`" + ` - List your scheduled jobs.
• ` + "`" + `/cron enable <name>` + "`" + ` - Enable a job.
• ` + "`" + `/cron disable <name>` + "`" + ` - Disable a job.
• ` + "`" + `/cron delete <name>` + "`" + ` - Delete a job.
• ` + "`" + `/cron run <name>` + "`" + ` - Trigger a job immediately in background.

*Tip:* You can also use natural language: ` + "`" + `jadwalin setiap jam 9 pagi cek website` + "`" + `.`
}

func handleCronCreate(store *cron.Store, platform string, chatID string, guildID string, args []string) string {
	fullText := strings.Join(args, " ")
	parts := parseQuotes(fullText)

	if len(parts) < 2 {
		return "✘ Invalid syntax. Use: `/cron create \"<schedule>\" \"<prompt>\" [--name <name>]`"
	}

	scheduleSpec := parts[0]
	prompt := parts[1]

	normalized, err := cron.NormalizeSchedule(scheduleSpec)
	if err != nil {
		return fmt.Sprintf("✘ Invalid schedule spec %q: %v", scheduleSpec, err)
	}

	name := ""
	for i := 2; i < len(parts); i++ {
		if parts[i] == "--name" && i+1 < len(parts) {
			name = parts[i+1]
			break
		}
	}
	if name == "" {
		name = fmt.Sprintf("job-%d", time.Now().UnixNano()%100000)
	}

	job := &cron.CronJob{
		Name:     name,
		Schedule: normalized,
		Prompt:   prompt,
		Platform: platform,
		ChatID:   chatID,
		GuildID:  guildID,
		Enabled:  true,
	}

	err = store.SaveJob(job)
	if err != nil {
		return fmt.Sprintf("✘ Failed to save job: %v", err)
	}

	return fmt.Sprintf("✔ Cron job %q created successfully!\n• Schedule: %q (parsed: `%s`)\n• Prompt: %q",
		name, scheduleSpec, normalized, prompt)
}

func parseQuotes(s string) []string {
	var res []string
	var current strings.Builder
	inQuotes := false
	escaped := false

	for _, r := range s {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '"' {
			inQuotes = !inQuotes
			if !inQuotes {
				res = append(res, current.String())
				current.Reset()
			}
			continue
		}
		if r == ' ' && !inQuotes {
			if current.Len() > 0 {
				res = append(res, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		res = append(res, current.String())
	}
	return res
}

func handleCronList(store *cron.Store, chatID string) string {
	jobs, err := store.ListJobs()
	if err != nil {
		return fmt.Sprintf("✘ Failed to list jobs: %v", err)
	}

	var userJobs []*cron.CronJob
	for _, j := range jobs {
		if j.ChatID == chatID {
			userJobs = append(userJobs, j)
		}
	}

	if len(userJobs) == 0 {
		return "ℹ You have no scheduled cron jobs. Create one using `/cron create`!"
	}

	var sb strings.Builder
	sb.WriteString("⎔ *Active Cron Jobs:*\n")
	for _, j := range userJobs {
		status := "✔ Enabled"
		if !j.Enabled {
			status = "✘ Disabled"
		}
		lastRun := "Never"
		if j.LastRun != nil {
			lastRun = j.LastRun.Format("2006-01-02 15:04:05")
		}
		sb.WriteString(fmt.Sprintf("\n• **%s** (%s)\n  Schedule: `%s`\n  Prompt: %q\n  Last Run: %s (Run Count: %d)\n",
			j.Name, status, j.Schedule, j.Prompt, lastRun, j.RunCount))
	}
	return sb.String()
}

func handleCronNaturalLanguage(store *cron.Store, platform string, chatID string, guildID string, query string) string {
	queryLower := strings.ToLower(query)

	prefixes := []string{"jadwalin ", "buat cron ", "buat jadwal ", "schedule ", "tambahkan cron "}
	for _, p := range prefixes {
		if strings.HasPrefix(queryLower, p) {
			query = query[len(p):]
			queryLower = queryLower[len(p):]
			break
		}
	}

	scheduleSpec := ""
	prompt := ""

	if idx := strings.Index(queryLower, "menit"); idx != -1 {
		digits := extractDigitsBefore(queryLower[:idx])
		if digits != "" {
			scheduleSpec = "every " + digits + "m"
			prompt = strings.TrimSpace(query[idx+len("menit"):])
		}
	} else if idx := strings.Index(queryLower, "jam"); idx != -1 {
		digits := extractDigitsBefore(queryLower[:idx])
		if digits != "" {
			scheduleSpec = "every " + digits + "h"
			prompt = strings.TrimSpace(query[idx+len("jam"):])
		} else {
			rem := queryLower[idx+len("jam"):]
			timePart, promptPart := extractDailyTimePart(rem)
			if timePart != "" {
				scheduleSpec = "daily at " + timePart
				promptIdx := strings.Index(queryLower, promptPart)
				if promptIdx != -1 {
					prompt = strings.TrimSpace(query[promptIdx:])
				}
			}
		}
	} else if idx := strings.Index(queryLower, "tiap hari "); idx != -1 {
		rem := queryLower[idx+len("tiap hari "):]
		if strings.HasPrefix(rem, "jam ") {
			rem = rem[len("jam "):]
		}
		timePart, promptPart := extractDailyTimePart(rem)
		if timePart != "" {
			scheduleSpec = "daily at " + timePart
			promptIdx := strings.Index(queryLower, promptPart)
			if promptIdx != -1 {
				prompt = strings.TrimSpace(query[promptIdx:])
			}
		}
	} else if idx := strings.Index(queryLower, "setiap hari "); idx != -1 {
		rem := queryLower[idx+len("setiap hari "):]
		if strings.HasPrefix(rem, "jam ") {
			rem = rem[len("jam "):]
		}
		timePart, promptPart := extractDailyTimePart(rem)
		if timePart != "" {
			scheduleSpec = "daily at " + timePart
			promptIdx := strings.Index(queryLower, promptPart)
			if promptIdx != -1 {
				prompt = strings.TrimSpace(query[promptIdx:])
			}
		}
	}

	if scheduleSpec == "" || prompt == "" {
		return "✘ I couldn't parse the natural language query. Please use standard command format:\n`/cron create \"<schedule>\" \"<prompt>\"`"
	}

	normalized, err := cron.NormalizeSchedule(scheduleSpec)
	if err != nil {
		return fmt.Sprintf("✘ Failed to parse schedule %q from natural language: %v", scheduleSpec, err)
	}

	name := fmt.Sprintf("job-%d", time.Now().UnixNano()%100000)
	job := &cron.CronJob{
		Name:     name,
		Schedule: normalized,
		Prompt:   prompt,
		Platform: platform,
		ChatID:   chatID,
		GuildID:  guildID,
		Enabled:  true,
	}

	err = store.SaveJob(job)
	if err != nil {
		return fmt.Sprintf("✘ Failed to save job: %v", err)
	}

	return fmt.Sprintf("⎔ *Cron Created via Natural Language!*\n• Name: **%s**\n• Schedule: %q (parsed: `%s`)\n• Prompt: %q",
		name, scheduleSpec, normalized, prompt)
}

func extractDigitsBefore(s string) string {
	s = strings.TrimSpace(s)
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return ""
	}
	last := parts[len(parts)-1]
	var res []rune
	for _, r := range last {
		if r >= '0' && r <= '9' {
			res = append(res, r)
		}
	}
	return string(res)
}

func extractDailyTimePart(s string) (timePart string, promptPart string) {
	s = strings.TrimSpace(s)
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return "", ""
	}
	first := parts[0]
	if len(parts) > 1 {
		next := parts[1]
		if next == "pagi" || next == "am" {
			return first + "am", parts[2]
		}
		if next == "malam" || next == "sore" || next == "pm" {
			return first + "pm", parts[2]
		}
	}
	if len(parts) > 1 {
		return first, parts[1]
	}
	return first, ""
}
