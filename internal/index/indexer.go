package index

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var extLangMap = map[string]string{
	".go":   "Go",
	".js":   "JavaScript",
	".ts":   "TypeScript",
	".jsx":  "JavaScript JSX",
	".tsx":  "TypeScript JSX",
	".py":   "Python",
	".rs":   "Rust",
	".java": "Java",
	".c":    "C",
	".cpp":  "C++",
	".h":    "C Header",
	".sh":   "Shell",
	".md":   "Markdown",
	".json": "JSON",
	".yaml": "YAML",
	".yml":  "YAML",
	".xml":  "XML",
	".html": "HTML",
	".css":  "CSS",
}

func BuildIndex(workDir string) (*Index, error) {
	absRoot, err := filepath.Abs(workDir)
	if err != nil {
		absRoot = workDir
	}

	idx := &Index{
		Root:    absRoot,
		BuiltAt: time.Now(),
	}

	gitignorePatterns := loadGitignore(absRoot)
	var dirsMap = make(map[string]bool)

	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}

		relPath, relErr := filepath.Rel(absRoot, path)
		if relErr != nil {
			relPath = path
		}

		if d.IsDir() {
			name := d.Name()
			if (strings.HasPrefix(name, ".") && name != "." && name != "..") ||
				name == "node_modules" ||
				name == "vendor" ||
				name == "build" ||
				name == "dist" {
				return filepath.SkipDir
			}

			if isGitignored(relPath, gitignorePatterns) {
				return filepath.SkipDir
			}

			return nil
		}

		if isGitignored(relPath, gitignorePatterns) {
			return nil
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		lang, exists := extLangMap[ext]
		if !exists {
			return nil
		}

		dirPath := filepath.Dir(relPath)
		if dirPath != "." {
			dirsMap[dirPath] = true
		}

		linesCount := countFileLines(path)

		fileInfo := FileInfo{
			Path:  relPath,
			Lang:  lang,
			Size:  info.Size(),
			Lines: linesCount,
		}

		if ext == ".go" {
			syms, pkg, symErr := ExtractSymbols(absRoot, path)
			if symErr == nil {
				fileInfo.Package = pkg
				idx.Symbols = append(idx.Symbols, syms...)
			}
		}

		idx.Files = append(idx.Files, fileInfo)
		return nil
	})

	if err != nil {
		return nil, err
	}

	for d := range dirsMap {
		idx.Dirs = append(idx.Dirs, d)
	}

	return idx, nil
}

func countFileLines(filePath string) int {
	file, err := os.Open(filePath)
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		count++
	}
	return count
}

func loadGitignore(root string) []string {
	var patterns []string
	gitignorePath := filepath.Join(root, ".gitignore")
	file, err := os.Open(gitignorePath)
	if err != nil {
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

func isGitignored(relPath string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	
	normalizedPath := filepath.ToSlash(relPath)

	for _, pattern := range patterns {
		pattern = filepath.ToSlash(pattern)
		
		isDirPattern := strings.HasSuffix(pattern, "/")
		cleanPattern := strings.TrimSuffix(pattern, "/")

		if strings.Contains(cleanPattern, "*") {
			matched, _ := filepath.Match(cleanPattern, normalizedPath)
			if matched {
				return true
			}
			dirPattern := cleanPattern
			if !strings.HasSuffix(dirPattern, "/*") {
				dirPattern = dirPattern + "/*"
			}
			matchedDir, _ := filepath.Match(dirPattern, normalizedPath)
			if matchedDir {
				return true
			}
		} else {
			if normalizedPath == cleanPattern || strings.HasPrefix(normalizedPath, cleanPattern+"/") {
				return true
			}
			if isDirPattern && strings.Contains(normalizedPath, "/"+cleanPattern+"/") {
				return true
			}
		}
	}
	return false
}
