package tools

import (
	"awas/internal/cron"
	"fmt"
	"strings"
	"time"
)

func ManageCron(action, name, schedule, prompt, platform, chatID, guildID string) string {
	store, err := cron.NewStore()
	if err != nil {
		return fmt.Sprintf("[Error] failed to open cron database: %v", err)
	}
	defer store.Close()

	action = strings.ToLower(strings.TrimSpace(action))
	switch action {
	case "create":
		if schedule == "" || prompt == "" {
			return "[Error] schedule and prompt are required to create a job"
		}
		normalized, err := cron.NormalizeSchedule(schedule)
		if err != nil {
			return fmt.Sprintf("[Error] invalid schedule: %v", err)
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
			return fmt.Sprintf("[Error] failed to save job: %v", err)
		}
		return fmt.Sprintf("✔ Cron job %q created successfully! Schedule: %q (parsed: `%s`)", name, schedule, normalized)

	case "list":
		jobs, err := store.ListJobs()
		if err != nil {
			return fmt.Sprintf("[Error] failed to list jobs: %v", err)
		}

		var userJobs []*cron.CronJob
		for _, j := range jobs {
			if j.ChatID == chatID {
				userJobs = append(userJobs, j)
			}
		}
		if len(userJobs) == 0 {
			return "No active cron jobs found for this chat."
		}

		var sb strings.Builder
		sb.WriteString("Active Cron Jobs:\n")
		for _, j := range userJobs {
			status := "Enabled"
			if !j.Enabled {
				status = "Disabled"
			}
			lastRun := "Never"
			if j.LastRun != nil {
				lastRun = j.LastRun.Format("2006-01-02 15:04:05")
			}
			sb.WriteString(fmt.Sprintf("- **%s** (%s): schedule=%s, prompt=%q, last_run=%s, runs=%d\n",
				j.Name, status, j.Schedule, j.Prompt, lastRun, j.RunCount))
		}
		return sb.String()

	case "delete":
		if name == "" {
			return "[Error] job name is required to delete"
		}
		err := store.DeleteJob(name)
		if err != nil {
			return fmt.Sprintf("[Error] failed to delete job: %v", err)
		}
		return fmt.Sprintf("✔ Cron job %q deleted successfully.", name)

	case "enable", "disable":
		if name == "" {
			return "[Error] job name is required"
		}
		job, err := store.GetJob(name)
		if err != nil {
			return fmt.Sprintf("[Error] job %q not found", name)
		}
		job.Enabled = (action == "enable")
		err = store.SaveJob(job)
		if err != nil {
			return fmt.Sprintf("[Error] failed to update status: %v", err)
		}
		return fmt.Sprintf("✔ Cron job %q has been %sd.", name, action)

	default:
		return fmt.Sprintf("[Error] unknown action: %q", action)
	}
}
