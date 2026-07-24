package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type TodoTask struct {
	ID     int    `json:"id"`
	Task   string `json:"task"`
	Status string `json:"status"` 
}

func ManageTodo(workDir, action string, taskID int, taskDesc, status string) string {
	todoPath := filepath.Join(workDir, ".awas", "todo.json")

	var tasks []TodoTask
	if data, err := os.ReadFile(todoPath); err == nil {
		json.Unmarshal(data, &tasks)
	}

	switch action {
	case "add":
		if strings.TrimSpace(taskDesc) == "" {
			return "[Error] task description cannot be empty for 'add' action"
		}
		newID := 1
		for _, t := range tasks {
			if t.ID >= newID {
				newID = t.ID + 1
			}
		}
		tasks = append(tasks, TodoTask{
			ID:     newID,
			Task:   strings.TrimSpace(taskDesc),
			Status: "pending",
		})

	case "update":
		found := false
		for i, t := range tasks {
			if t.ID == taskID {
				if strings.TrimSpace(taskDesc) != "" {
					tasks[i].Task = strings.TrimSpace(taskDesc)
				}
				if status == "pending" || status == "completed" {
					tasks[i].Status = status
				}
				found = true
				break
			}
		}
		if !found {
			return fmt.Sprintf("[Error] todo task with ID %d not found", taskID)
		}

	case "list":
	default:
		return fmt.Sprintf("[Error] unknown todo action: %s. Use 'add', 'update', or 'list'", action)
	}

	if action == "add" || action == "update" {
		os.MkdirAll(filepath.Dir(todoPath), 0755)
		if data, err := json.MarshalIndent(tasks, "", "  "); err == nil {
			os.WriteFile(todoPath, data, 0644)
		}
	}

	if len(tasks) == 0 {
		return "Your TODO list is currently empty. Use the 'add' action to add tasks!"
	}

	var sb strings.Builder
	sb.WriteString("###  Workspace TODO List\n")
	for _, t := range tasks {
		box := "[ ]"
		if t.Status == "completed" {
			box = "[x]"
		}
		sb.WriteString(fmt.Sprintf("%s ID %d: %s\n", box, t.ID, t.Task))
	}
	return sb.String()
}
