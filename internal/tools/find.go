package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func FindFiles(workDir string, pattern string) string {
	var matches []string

	err := filepath.Walk(workDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		rel, err := filepath.Rel(workDir, path)
		if err != nil {
			return nil
		}

		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || (strings.HasPrefix(name, ".") && name != ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.Contains(pattern, "/") {
			matched, _ := filepath.Match(pattern, info.Name())
			if matched {
				matches = append(matches, rel)
			}
			return nil
		}

		matched := matchGlob(pattern, rel)
		if matched {
			matches = append(matches, rel)
		}

		return nil
	})

	if err != nil {
		return fmt.Sprintf("[Error] failed to search files: %v", err)
	}

	if len(matches) == 0 {
		return "No matching files found."
	}

	limit := 100
	if len(matches) > limit {
		return fmt.Sprintf("Found %d matching files (showing first %d):\n- %s\n... and %d more files.",
			len(matches), limit, strings.Join(matches[:limit], "\n- "), len(matches)-limit)
	}

	return fmt.Sprintf("Found %d matching files:\n- %s", len(matches), strings.Join(matches, "\n- "))
}

func matchGlob(pattern, path string) bool {
	pattern = filepath.ToSlash(pattern)
	path = filepath.ToSlash(path)

	parts := strings.Split(pattern, "**")
	if len(parts) == 1 {
		matched, _ := filepath.Match(pattern, path)
		return matched
	}

	if !strings.HasPrefix(path, strings.TrimPrefix(parts[0], "/")) {
		return false
	}
	lastPart := parts[len(parts)-1]
	if strings.ContainsAny(lastPart, "*?[]") {
		tailPattern := strings.TrimPrefix(lastPart, "/")
		if len(path) < len(tailPattern) {
			return false
		}
		matched, _ := filepath.Match("*"+tailPattern, path)
		return matched
	}

	return strings.HasSuffix(path, lastPart)
}
