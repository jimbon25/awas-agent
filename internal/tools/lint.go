package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func LintAndFormat(workDir string, relativePath string, action string) string {
	absPath := filepath.Join(workDir, relativePath)
	action = strings.ToLower(strings.TrimSpace(action))

	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Sprintf("[Error] file does not exist: %s", relativePath)
	}
	if info.IsDir() {
		return fmt.Sprintf("[Error] %s is a directory, not a file", relativePath)
	}

	ext := strings.ToLower(filepath.Ext(absPath))
	switch ext {
	case ".go":
		if action == "format" {
			cmd := exec.Command("go", "fmt", absPath)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			err := cmd.Run()
			if err != nil {
				return fmt.Sprintf("[Error] go fmt failed: %v\n%s", err, stderr.String())
			}
			return fmt.Sprintf(" Go file %s formatted successfully.", relativePath)
		} else if action == "lint" {
			cmd := exec.Command("go", "vet", absPath)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			err := cmd.Run()
			if err != nil {
				return fmt.Sprintf("✗ Syntax/Lint errors found:\n%s", stderr.String())
			}
			return fmt.Sprintf("✔ Go file %s syntax check passed (go vet).", relativePath)
		}

	case ".json":
		data, err := os.ReadFile(absPath)
		if err != nil {
			return fmt.Sprintf("[Error] failed to read file: %v", err)
		}

		var js any
		if err := json.Unmarshal(data, &js); err != nil {
			return fmt.Sprintf("✗ Invalid JSON syntax: %v", err)
		}

		if action == "format" {
			var prettyJSON bytes.Buffer
			if err := json.Indent(&prettyJSON, data, "", "  "); err != nil {
				return fmt.Sprintf("[Error] JSON formatting failed: %v", err)
			}
			if err := os.WriteFile(absPath, prettyJSON.Bytes(), 0644); err != nil {
				return fmt.Sprintf("[Error] failed to save formatted JSON: %v", err)
			}
			return fmt.Sprintf("✔ JSON file %s formatted successfully.", relativePath)
		} else if action == "lint" {
			return fmt.Sprintf("✔ JSON file %s syntax check passed.", relativePath)
		}

	default:
		return fmt.Sprintf("✦ Language not supported for formatting/linting: %s. File exists and is readable.", ext)
	}

	return fmt.Sprintf("[Error] invalid action: %q", action)
}
