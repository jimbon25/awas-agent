package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var skillsMu sync.Mutex

func getSkillsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".awas", "skills")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

func ManageSkills(action, name, content, oldText string) string {
	skillsMu.Lock()
	defer skillsMu.Unlock()

	dir, err := getSkillsDir()
	if err != nil {
		return fmt.Sprintf("[Error] failed to locate skills directory: %v", err)
	}

	switch action {
	case "list":
		files, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Sprintf("[Error] failed to read skills directory: %v", err)
		}
		var list []string
		for _, f := range files {
			if !f.IsDir() && filepath.Ext(f.Name()) == ".md" {
				list = append(list, f.Name())
			}
		}
		if len(list) == 0 {
			return "No skill files found. Local skills can be created using the 'create' action."
		}
		return "Available local skill files:\n- " + strings.Join(list, "\n- ")

	case "create":
		if name == "" {
			return "[Error] skill file name is required for 'create' action"
		}
		if !strings.HasSuffix(name, ".md") {
			name += ".md"
		}
		filePath := filepath.Join(dir, name)
		if _, err := os.Stat(filePath); err == nil {
			return fmt.Sprintf("[Error] skill file '%s' already exists, use 'patch' to update it", name)
		}
		if strings.TrimSpace(content) == "" {
			return "[Error] skill content cannot be empty"
		}
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			return fmt.Sprintf("[Error] failed to write skill file: %v", err)
		}
		return fmt.Sprintf("Successfully created skill file '%s'.", name)

	case "patch":
		if name == "" {
			return "[Error] skill file name is required for 'patch' action"
		}
		if !strings.HasSuffix(name, ".md") {
			name += ".md"
		}
		filePath := filepath.Join(dir, name)
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Sprintf("[Error] failed to read skill file '%s': %v", name, err)
		}
		fileContent := string(data)

		if oldText == "" {
			if strings.TrimSpace(content) == "" {
				return "[Error] content cannot be empty for patch overwrite"
			}
			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				return fmt.Sprintf("[Error] failed to write skill file: %v", err)
			}
			return fmt.Sprintf("Successfully patched (overwrote) skill file '%s'.", name)
		}

		count := strings.Count(fileContent, oldText)
		if count == 0 {
			return fmt.Sprintf("[Error] old_text segment not found in skill file '%s'", name)
		}
		if count > 1 {
			return fmt.Sprintf("[Error] old_text found %d times, make it more unique", count)
		}

		newContent := strings.Replace(fileContent, oldText, content, 1)
		if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
			return fmt.Sprintf("[Error] failed to write skill file: %v", err)
		}
		return fmt.Sprintf("Successfully patched skill file '%s'.", name)

	default:
		return fmt.Sprintf("[Error] invalid action: '%s'. Must be 'list', 'create', or 'patch'", action)
	}
}
