package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var memoryMu sync.Mutex

func getMemoriesDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".awas", "memories")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

func initMemoryFile(path, defaultContent string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.WriteFile(path, []byte(defaultContent), 0644)
	}
	return nil
}

func ReadMemories() (string, string) {
	memoryMu.Lock()
	defer memoryMu.Unlock()

	dir, err := getMemoriesDir()
	if err != nil {
		return "", ""
	}

	memoryPath := filepath.Join(dir, "MEMORY.md")
	userPath := filepath.Join(dir, "USER.md")

	_ = initMemoryFile(memoryPath, "# PERSISTENT SYSTEM MEMORY\n- Environment: unknown\n- Conventions: unknown\n")
	_ = initMemoryFile(userPath, "# USER PROFILE & PREFERENCES\n- User: unknown\n- Preferences: unknown\n")

	memBytes, _ := os.ReadFile(memoryPath)
	userBytes, _ := os.ReadFile(userPath)

	return string(memBytes), string(userBytes)
}

func ManageMemory(action, target, content, oldText string) string {
	memoryMu.Lock()
	defer memoryMu.Unlock()

	dir, err := getMemoriesDir()
	if err != nil {
		return fmt.Sprintf("[Error] failed to locate memories directory: %v", err)
	}

	var filename string
	if target == "memory" {
		filename = "MEMORY.md"
	} else if target == "user" {
		filename = "USER.md"
	} else {
		return fmt.Sprintf("[Error] invalid target: '%s'. Must be 'memory' or 'user'", target)
	}

	filePath := filepath.Join(dir, filename)
	
	if target == "memory" {
		_ = initMemoryFile(filePath, "# PERSISTENT SYSTEM MEMORY\n- Environment: unknown\n- Conventions: unknown\n")
	} else {
		_ = initMemoryFile(filePath, "# USER PROFILE & PREFERENCES\n- User: unknown\n- Preferences: unknown\n")
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Sprintf("[Error] failed to read memory file: %v", err)
	}
	fileContent := string(data)

	switch action {
	case "read":
		return fmt.Sprintf("=== Current %s ===\n%s", filename, fileContent)

	case "add":
		if strings.TrimSpace(content) == "" {
			return "[Error] content cannot be empty for 'add' action"
		}
		line := strings.TrimSpace(content)
		if !strings.HasPrefix(line, "- ") {
			line = "- " + line
		}
		if !strings.HasSuffix(fileContent, "\n") {
			fileContent += "\n"
		}
		fileContent += line + "\n"

	case "replace":
		if oldText == "" {
			return "[Error] old_text segment is required for 'replace' action"
		}
		count := strings.Count(fileContent, oldText)
		if count == 0 {
			return "[Error] old_text not found in memory file"
		}
		if count > 1 {
			return fmt.Sprintf("[Error] old_text found %d times, make it more unique", count)
		}
		fileContent = strings.Replace(fileContent, oldText, content, 1)

	case "remove":
		if oldText == "" {
			return "[Error] old_text segment is required for 'remove' action"
		}
		count := strings.Count(fileContent, oldText)
		if count == 0 {
			return "[Error] old_text not found in memory file"
		}
		if count > 1 {
			return fmt.Sprintf("[Error] old_text found %d times, make it more unique", count)
		}
		fileContent = strings.Replace(fileContent, oldText, "", 1)

	default:
		return fmt.Sprintf("[Error] invalid action: '%s'. Must be 'read', 'add', 'replace', or 'remove'", action)
	}

	if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
		return fmt.Sprintf("[Error] failed to write memory file: %v", err)
	}

	return fmt.Sprintf("Successfully performed '%s' action on persistent %s.", action, target)
}
