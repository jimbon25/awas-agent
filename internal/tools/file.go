package tools

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
)

type fileReadEntry struct {
	Hash      string
	Timestamp time.Time
}

var (
	fileReadCache = make(map[string]fileReadEntry)
	fileCacheMu   sync.RWMutex
)

func cacheFileRead(absPath, content string) {
	fileCacheMu.Lock()
	defer fileCacheMu.Unlock()
	h := sha256.Sum256([]byte(content))
	fileReadCache[absPath] = fileReadEntry{
		Hash:      fmt.Sprintf("%x", h),
		Timestamp: time.Now(),
	}
}

func checkStaleRead(absPath string) (bool, string) {
	fileCacheMu.RLock()
	defer fileCacheMu.RUnlock()

	entry, ok := fileReadCache[absPath]
	if !ok {
		return false, "" 
	}

	currentData, err := os.ReadFile(absPath)
	if err != nil {
		return false, ""
	}
	h := sha256.Sum256(currentData)
	currentHash := fmt.Sprintf("%x", h)

	if currentHash != entry.Hash {
		elapsed := time.Since(entry.Timestamp).Truncate(time.Millisecond)
		return true, fmt.Sprintf(
			"[Warning] File modified externally since last read (%v ago).\n"+
				"  Your old_string may be stale. Consider re-reading the file before editing.",
			elapsed,
		)
	}
	return false, ""
}

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

func ReadFile(workDir, path string, startLine, endLine int) string {
	absPath, err := resolvePath(workDir, path)
	if err != nil {
		return fmt.Sprintf("[Error] failed to resolve path: %v", err)
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Sprintf("[Error] failed to read file: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	totalLines := len(lines)

	if startLine <= 0 && endLine <= 0 {
		cacheFileRead(absPath, string(data))
		return string(data)
	}

	if startLine <= 0 {
		startLine = 1
	}
	start := startLine - 1 
	if start >= totalLines {
		return fmt.Sprintf("[Error] start_line %d exceeds file length (%d lines)", startLine, totalLines)
	}

	end := totalLines
	if endLine > 0 {
		end = endLine
		if end > totalLines {
			end = totalLines
		}
	}

	selected := lines[start:end]

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Lines %d-%d of %d total:\n\n", start+1, end, totalLines))
	for i, line := range selected {
		sb.WriteString(fmt.Sprintf("%4d | %s\n", start+i+1, line))
	}

	return sb.String()
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

type MatchResult struct {
	Found    bool
	Line     int    
	Content  string 
	Level    string 
	Distance int    
}

func findBestMatch(content, oldString string) MatchResult {
	if idx := strings.Index(content, oldString); idx != -1 {
		return MatchResult{
			Found: true,
			Level: "exact",
			Line:  lineNumberAt(content, idx),
		}
	}

	lines := strings.Split(content, "\n")

	normOld := normalizeForMatch(oldString)
	for i, line := range lines {
		if normalizeForMatch(line) == normOld {
			return MatchResult{
				Found:   true,
				Level:   "normalized",
				Line:    i + 1,
				Content: line,
			}
		}
	}

	trimmedOld := strings.TrimSpace(oldString)
	for i, line := range lines {
		if strings.TrimSpace(line) == trimmedOld {
			return MatchResult{
				Found:   true,
				Level:   "trimmed",
				Line:    i + 1,
				Content: line,
			}
		}
	}

	return MatchResult{Found: false}
}

func normalizeForMatch(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimRight(s, " \t\r\n")
	return s
}

func lineNumberAt(content string, offset int) int {
	if offset <= 0 {
		return 1
	}
	line := 1
	for i := 0; i < offset && i < len(content); i++ {
		if content[i] == '\n' {
			line++
		}
	}
	return line
}

func sanitizeEscaping(s string) (string, bool) {
	original := s
	s = strings.ReplaceAll(s, `\\n`, "\n")
	s = strings.ReplaceAll(s, `\\t`, "\t")
	s = strings.ReplaceAll(s, `\\"`, `"`)
	s = strings.ReplaceAll(s, `\\'`, "'")
	s = strings.ReplaceAll(s, `\\\\`, "\\")
	return s, s != original
}

func detectIndentStyle(content string) string {
	lines := strings.Split(content, "\n")
	tabCount, spaces2, spaces4 := 0, 0, 0

	for _, line := range lines {
		if line == "" {
			continue
		}
		trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
		if trimmed == "" {
			continue
		}
		leading := line[:len(line)-len(trimmed)]
		if strings.HasPrefix(leading, "\t") {
			tabCount++
		} else if strings.HasPrefix(leading, "    ") {
			spaces4++
		} else if len(leading) >= 2 {
			spaces2++
		}
	}

	if tabCount > spaces2 && tabCount > spaces4 {
		return "\t"
	}
	if spaces4 > spaces2 {
		return "    "
	}
	return "  "
}

func reindentToMatch(s string, targetStyle string) string {
	lines := strings.Split(s, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" {
			result = append(result, "")
			continue
		}
		leading := line[:len(line)-len(trimmed)]
		level := 0
		for _, ch := range leading {
			if ch == '\t' {
				level++
			} else {
				level++
			}
		}
		// Convert to target style
		newIndent := strings.Repeat(targetStyle, level)
		result = append(result, newIndent+trimmed)
	}
	return strings.Join(result, "\n")
}

func levenshteinDistance(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(
				prev[j]+1,      
				curr[j-1]+1,    
				prev[j-1]+cost, 
			)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

func suggestFix(content, oldString string) string {
	lines := strings.Split(content, "\n")
	bestLine := -1
	bestDist := len(oldString) * 2

	trimmedOld := strings.TrimSpace(oldString)
	if trimmedOld == "" {
		return "old_string is empty after trimming."
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		dist := levenshteinDistance(trimmed, trimmedOld)
		if dist < bestDist {
			bestDist = dist
			bestLine = i + 1
		}
	}

	if bestLine > 0 {
		maxLen := len(trimmedOld)
		if maxLen > 60 {
			maxLen = 60
		}
		similarity := int(float64(1-float64(bestDist)/float64(max(maxLen, 1))) * 100)
		if similarity < 0 {
			similarity = 0
		}
		actual := lines[bestLine-1]
		if len(actual) > 80 {
			actual = actual[:77] + "..."
		}
		return fmt.Sprintf(
			"Nearest match found around Line %d (%d%% similarity):\n"+
				"  Expected: %q\n"+
				"  File has: %q\n"+
				"  Tip: Use read_file with start_line=%d to see the exact content.",
			bestLine, similarity, trimTo(oldString, 60), actual,
			maxInt(1, bestLine-2),
		)
	}

	return "No similar content found in file. Use read_file to inspect the current state."
}

func trimTo(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func autoValidateAfterEdit(absPath string) string {
	ext := strings.ToLower(filepath.Ext(absPath))
	switch ext {
	case ".go":
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "go", "vet", absPath)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = err.Error()
			}
			return fmt.Sprintf(
				"\n[Warning] Post-edit lint failed:\n%s\n"+
					"  Tip: Review the changes at the reported line numbers.",
				msg,
			)
		}
	case ".json":
		data, err := os.ReadFile(absPath)
		if err == nil {
			var js any
			if json.Unmarshal(data, &js) != nil {
				return "\n[Warning] Post-edit JSON validation failed. Check syntax near the edit."
			}
		}
	}
	return ""
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

	escapeNote := ""
	cleanedOld, oldChanged := sanitizeEscaping(oldString)
	cleanedNew, _ := sanitizeEscaping(newString)
	if oldChanged {
		oldString = cleanedOld
		newString = cleanedNew
		escapeNote = " [Note: auto-corrected over-escaping in old_string]"
	}

	if isStale, staleMsg := checkStaleRead(absPath); isStale {
		return staleMsg
	}

	match := findBestMatch(content, oldString)

	if !match.Found {
		return fmt.Sprintf("[Error] old_string not found in file.%s\n  %s", escapeNote, suggestFix(content, oldString))
	}

	searchString := oldString
	replaceString := newString
	if match.Level == "normalized" || match.Level == "trimmed" {
		searchString = match.Content
		detectedStyle := detectIndentStyle(content)
		replaceString = reindentToMatch(newString, detectedStyle)
	}

	count := strings.Count(content, searchString)
	if count == 0 {
		searchString = oldString
		count = strings.Count(content, searchString)
	}

	if count == 0 {
		return fmt.Sprintf("[Error] old_string not found in file.%s\n  %s", escapeNote, suggestFix(content, oldString))
	}

	var newContent string
	if replaceAll {
		newContent = strings.ReplaceAll(content, searchString, replaceString)
	} else {
		if count > 1 {
			return fmt.Sprintf("[Error] old_string found %d times, please make old_string more unique or set replace_all=true%s", count, escapeNote)
		}
		newContent = strings.Replace(content, searchString, replaceString, 1)
	}

	perm := os.FileMode(0644)
	if info, err := os.Stat(absPath); err == nil {
		perm = info.Mode().Perm()
	}
	if err := atomicWrite(absPath, []byte(newContent), perm); err != nil {
		return fmt.Sprintf("[Error] failed to write file: %v", err)
	}

	_ = RecordChange(workDir, rel, "edit_file", "modified", data, []byte(newContent))

	cacheFileRead(absPath, newContent)

	lintResult := autoValidateAfterEdit(absPath)

	levelNote := ""
	if match.Level != "exact" {
		levelNote = fmt.Sprintf(" (matched via %s at line %d)", match.Level, match.Line)
	}
	result := fmt.Sprintf("Successfully edited file. Replaced %d occurrence(s).%s%s", count, levelNote, escapeNote)
	if lintResult != "" {
		result += lintResult
	}
	return result
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

	type resolvedPatch struct {
		Search  string
		Replace string
		Match   MatchResult
	}
	resolved := make([]resolvedPatch, len(patches))

	for idx, patch := range patches {
		if patch.OldString == "" {
			return fmt.Sprintf("[Error] patch %d/%d: old_string is empty", idx+1, len(patches))
		}

		cleanedOld, oldChanged := sanitizeEscaping(patch.OldString)
		cleanedNew, _ := sanitizeEscaping(patch.NewString)
		searchStr := patch.OldString
		replaceStr := patch.NewString
		if oldChanged {
			searchStr = cleanedOld
			replaceStr = cleanedNew
		}

		match := findBestMatch(content, searchStr)
		if !match.Found {
			return fmt.Sprintf(
				"[Error] patch %d/%d failed.\n  %s\n  Patches %d-%d were NOT applied (transaction rolled back).",
				idx+1, len(patches), suggestFix(content, searchStr), 1, idx,
			)
		}

		if match.Level == "normalized" || match.Level == "trimmed" {
			searchStr = match.Content
			detectedStyle := detectIndentStyle(content)
			replaceStr = reindentToMatch(replaceStr, detectedStyle)
		}

		resolved[idx] = resolvedPatch{Search: searchStr, Replace: replaceStr, Match: match}
	}

	for _, rp := range resolved {
		content = strings.Replace(content, rp.Search, rp.Replace, 1)
	}

	perm := os.FileMode(0644)
	if info, err := os.Stat(absPath); err == nil {
		perm = info.Mode().Perm()
	}
	if err := atomicWrite(absPath, []byte(content), perm); err != nil {
		return fmt.Sprintf("[Error] failed to write file: %v", err)
	}

	_ = RecordChange(workDir, rel, "patch_file", "modified", data, []byte(content))

	lintResult := autoValidateAfterEdit(absPath)

	var notes []string
	for i, rp := range resolved {
		if rp.Match.Level != "exact" {
			notes = append(notes, fmt.Sprintf("patch %d: %s match", i+1, rp.Match.Level))
		}
	}
	successMsg := fmt.Sprintf("Successfully applied %d patch chunk(s).", len(patches))
	if len(notes) > 0 {
		successMsg += " [" + strings.Join(notes, ", ") + "]"
	}
	if lintResult != "" {
		successMsg += lintResult
	}

	return successMsg
}

func atomicWrite(absPath string, content []byte, perm os.FileMode) error {
	tmpPath := absPath + ".awas_tmp"
	if err := os.WriteFile(tmpPath, content, perm); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := os.Rename(tmpPath, absPath); err != nil {
		data, readErr := os.ReadFile(tmpPath)
		if readErr != nil {
			return fmt.Errorf("rename failed and temp read failed: %w", err)
		}
		os.Remove(tmpPath)
		return os.WriteFile(absPath, data, perm)
	}
	return nil
}
