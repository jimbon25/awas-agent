package tools

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
)

func SearchCode(workDir string, pattern string, searchPath string, include string, maxResults int) string {
	if searchPath == "" {
		searchPath = workDir
	} else {
		var err error
		searchPath, err = resolvePath(workDir, searchPath)
		if err != nil {
			return fmt.Sprintf("[Error] failed to resolve path: %v", err)
		}
	}

	if maxResults <= 0 {
		maxResults = 10
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Sprintf("[Error] invalid regex pattern: %v", err)
	}

	gitignorePatterns := loadGitignorePatterns(searchPath)

	type fileTask struct {
		path    string
		relPath string
	}
	var tasks []fileTask

	walkErr := filepath.WalkDir(searchPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if (strings.HasPrefix(name, ".") && name != ".") || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			if isGitignored(name, true, gitignorePatterns) {
				return filepath.SkipDir
			}
			return nil
		}

		if include != "" {
			matched, err := filepath.Match(include, d.Name())
			if err != nil || !matched {
				return nil
			}
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		var header [512]byte
		n, _ := f.Read(header[:])
		for i := 0; i < n; i++ {
			if header[i] == 0 {
				f.Close()
				return nil
			}
		}
		f.Close()

		relPath, _ := filepath.Rel(workDir, path)
		tasks = append(tasks, fileTask{path: path, relPath: relPath})
		return nil
	})
	if walkErr != nil && walkErr != filepath.SkipAll {
		return fmt.Sprintf("[Error] search failed: %v", walkErr)
	}

	if len(tasks) == 0 {
		return "No matches found."
	}

	var mu sync.Mutex
	var results []string

	taskCh := make(chan fileTask, len(tasks))
	for _, t := range tasks {
		taskCh <- t
	}
	close(taskCh)

	numWorkers := runtime.NumCPU()
	if numWorkers < 2 {
		numWorkers = 2
	}
	if numWorkers > len(tasks) {
		numWorkers = len(tasks)
	}

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range taskCh {
				mu.Lock()
				if len(results) >= maxResults {
					mu.Unlock()
					return
				}
				mu.Unlock()

				f, err := os.Open(t.path)
				if err != nil {
					continue
				}

				scanner := bufio.NewScanner(f)
				buf := make([]byte, 64*1024)
				scanner.Buffer(buf, 1024*1024)

				lineNum := 1
				for scanner.Scan() {
					line := scanner.Text()
					if re.MatchString(line) {
						matchLine := fmt.Sprintf("%s:%d: %s", t.relPath, lineNum, strings.TrimSpace(line))
						mu.Lock()
						if len(results) < maxResults {
							results = append(results, matchLine)
						}
						mu.Unlock()
					}
					lineNum++
				}
				f.Close()
			}
		}()
	}
	wg.Wait()

	sort.Strings(results)

	if len(results) > maxResults {
		results = results[:maxResults]
	}

	if len(results) == 0 {
		return "No matches found."
	}
	return strings.Join(results, "\n")
}

func loadGitignorePatterns(dir string) []string {
	f, err := os.Open(filepath.Join(dir, ".gitignore"))
	if err != nil {
		return nil
	}
	defer f.Close()

	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

func isGitignored(name string, isDir bool, patterns []string) bool {
	for _, p := range patterns {
		if strings.HasSuffix(p, "/") && !isDir {
			continue
		}
		trimmed := strings.TrimSuffix(p, "/")
		matched, _ := filepath.Match(trimmed, name)
		if matched {
			return true
		}
	}
	return false
}

func ListDirectory(workDir string, path string) string {
	absPath, err := resolvePath(workDir, path)
	if err != nil {
		return fmt.Sprintf("[Error] failed to resolve path: %v", err)
	}
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return fmt.Sprintf("[Error] failed to list directory: %v", err)
	}

	var results []string
	for _, entry := range entries {
		info, err := entry.Info()
		typeStr := "file"
		sizeStr := ""
		if err == nil {
			if info.IsDir() {
				typeStr = "dir"
			} else {
				sizeStr = fmt.Sprintf(" (%d bytes)", info.Size())
			}
		}
		results = append(results, fmt.Sprintf("[%s] %s%s", typeStr, entry.Name(), sizeStr))
	}

	if len(results) == 0 {
		return "Directory is empty."
	}

	return strings.Join(results, "\n")
}
