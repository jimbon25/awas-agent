package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func resolvePath(workDir, path string) (string, error) {
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return "", err
	}
	var absPath string
	if filepath.IsAbs(path) {
		absPath = filepath.Clean(path)
	} else {
		absPath = filepath.Clean(filepath.Join(absWorkDir, path))
	}
	finalAbs, err := filepath.Abs(absPath)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absWorkDir, finalAbs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path traversal detected: path %s is outside workspace %s", path, workDir)
	}
	return finalAbs, nil
}


func ReadFile(workDir, path string) string {
	absPath, err := resolvePath(workDir, path)
	if err != nil {
		return fmt.Sprintf("[Error] failed to resolve path: %v", err)
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Sprintf("[Error] failed to read file: %v", err)
	}
	return string(data)
}

func WriteFile(workDir, path, content string) string {
	absPath, err := resolvePath(workDir, path)
	if err != nil {
		return fmt.Sprintf("[Error] failed to resolve path: %v", err)
	}

	rel, err := filepath.Rel(workDir, absPath)
	if err != nil {
		rel = path
	}

	var oldContent []byte
	action := "created"
	perm := os.FileMode(0644) 
	if data, err := os.ReadFile(absPath); err == nil {
		oldContent = data
		action = "modified"
		if info, err := os.Stat(absPath); err == nil {
			perm = info.Mode().Perm()
		}
	}

	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Sprintf("[Error] failed to create directory: %v", err)
	}
	if err := os.WriteFile(absPath, []byte(content), perm); err != nil {
		return fmt.Sprintf("[Error] failed to write file: %v", err)
	}

	_ = RecordChange(workDir, rel, "write_file", action, oldContent, []byte(content))

	return "Successfully wrote file."
}

func EditFile(workDir, filePath, oldString, newString string, replaceAll bool) string {
	absPath, err := resolvePath(workDir, filePath)
	if err != nil {
		return fmt.Sprintf("[Error] failed to resolve path: %v", err)
	}

	rel, err := filepath.Rel(workDir, absPath)
	if err != nil {
		rel = filePath
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Sprintf("[Error] failed to read file: %v", err)
	}
	content := string(data)

	count := strings.Count(content, oldString)
	if count == 0 {
		return "[Error] old_string not found in file"
	}

	var newContent string
	if replaceAll {
		newContent = strings.ReplaceAll(content, oldString, newString)
	} else {
		if count > 1 {
			return fmt.Sprintf("[Error] old_string found %d times, please make old_string more unique or set replace_all=true", count)
		}
		newContent = strings.Replace(content, oldString, newString, 1)
	}

	perm := os.FileMode(0644)
	if info, err := os.Stat(absPath); err == nil {
		perm = info.Mode().Perm()
	}
	if err := os.WriteFile(absPath, []byte(newContent), perm); err != nil {
		return fmt.Sprintf("[Error] failed to write file: %v", err)
	}

	_ = RecordChange(workDir, rel, "edit_file", "modified", data, []byte(newContent))

	return fmt.Sprintf("Successfully edited file. Replaced %d occurrence(s).", count)
}

func DeleteFile(workDir, path string) string {
	absPath, err := resolvePath(workDir, path)
	if err != nil {
		return fmt.Sprintf("[Error] failed to resolve path: %v", err)
	}

	rel, err := filepath.Rel(workDir, absPath)
	if err != nil {
		rel = path
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Sprintf("[Error] failed to read file: %v", err)
	}

	if err := os.Remove(absPath); err != nil {
		return fmt.Sprintf("[Error] failed to delete file: %v", err)
	}

	_ = RecordChange(workDir, rel, "delete_file", "deleted", data, nil)

	return "Successfully deleted file."
}

type PatchChunk struct {
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

func PatchFile(workDir, filePath string, patches []PatchChunk) string {
	absPath, err := resolvePath(workDir, filePath)
	if err != nil {
		return fmt.Sprintf("[Error] failed to resolve path: %v", err)
	}

	rel, err := filepath.Rel(workDir, absPath)
	if err != nil {
		rel = filePath
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Sprintf("[Error] failed to read file: %v", err)
	}
	content := string(data)

	for idx, patch := range patches {
		if patch.OldString == "" {
			return fmt.Sprintf("[Error] patch chunk %d: old_string cannot be empty", idx)
		}
		count := strings.Count(content, patch.OldString)
		if count == 0 {
			return fmt.Sprintf("[Error] patch chunk %d: old_string not found in file", idx)
		}
		if count > 1 {
			return fmt.Sprintf("[Error] patch chunk %d: old_string found %d times, please make it more unique", idx, count)
		}
	}

	for _, patch := range patches {
		content = strings.Replace(content, patch.OldString, patch.NewString, 1)
	}

	perm := os.FileMode(0644)
	if info, err := os.Stat(absPath); err == nil {
		perm = info.Mode().Perm()
	}
	if err := os.WriteFile(absPath, []byte(content), perm); err != nil {
		return fmt.Sprintf("[Error] failed to write file: %v", err)
	}

	_ = RecordChange(workDir, rel, "patch_file", "modified", data, []byte(content))

	return fmt.Sprintf("Successfully applied %d patch chunk(s).", len(patches))
}
