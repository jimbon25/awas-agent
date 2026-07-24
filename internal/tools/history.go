package tools

import (
	"awas/internal/sessionlock"
	"crypto/rand"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type HistoryEntry struct {
	ID           string
	Timestamp    time.Time
	Workspace    string
	ToolName     string
	FilePath     string
	BackupBefore string
	BackupAfter  string
	Action       string
	Undone       bool
}

type HistoryStore struct {
	Entries []HistoryEntry
}

var (
	currentSessionID string
	sessionIDMu      sync.RWMutex
)

func SetCurrentSessionID(id string) {
	sessionIDMu.Lock()
	defer sessionIDMu.Unlock()
	currentSessionID = id
}

func GetCurrentSessionID() string {
	sessionIDMu.RLock()
	defer sessionIDMu.RUnlock()
	return currentSessionID
}

func getSessionPath(id string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".awas", "sessions")
	return filepath.Join(dir, id+".db"), nil
}

func getSessionHistoryDB(sessionID string) (*sql.DB, error) {
	path, err := getSessionPath(sessionID)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA foreign_keys=ON")
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	schema := `
	CREATE TABLE IF NOT EXISTS history (
		id            TEXT PRIMARY KEY,
		timestamp     TEXT NOT NULL,
		workspace     TEXT NOT NULL,
		tool_name     TEXT NOT NULL,
		file_path     TEXT NOT NULL,
		backup_before TEXT NOT NULL DEFAULT '',
		backup_after  TEXT NOT NULL DEFAULT '',
		action        TEXT NOT NULL,
		undone        INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_history_workspace ON history(workspace, timestamp);
	`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func getBackupDir(sessionID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".awas", "sessions", sessionID+"_backups")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

func writeBackup(sessionID, id, prefix string, content []byte) (string, error) {
	if content == nil {
		return "", nil
	}

	dir, err := getBackupDir(sessionID)
	if err != nil {
		return "", err
	}

	timestamp := time.Now().Format("2006-01-02_15-04-05")
	backupFileName := fmt.Sprintf("%s_%s_%s.bak", timestamp, id, prefix)
	backupPath := filepath.Join(dir, backupFileName)

	if err := os.WriteFile(backupPath, content, 0644); err != nil {
		return "", err
	}
	return backupPath, nil
}

func generateID() string {
	b := make([]byte, 8)
	_, _ = io.ReadFull(rand.Reader, b)
	return fmt.Sprintf("%x", b)
}

func loadHistoryInternal(sessionID string) (*HistoryStore, error) {
	if sessionID == "" {
		return &HistoryStore{Entries: []HistoryEntry{}}, nil
	}

	db, err := getSessionHistoryDB(sessionID)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, timestamp, workspace, tool_name, file_path, 
		       backup_before, backup_after, action, undone
		FROM history
		ORDER BY timestamp ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	store := &HistoryStore{}
	for rows.Next() {
		var entry HistoryEntry
		var timestamp string
		var undone int
		if err := rows.Scan(&entry.ID, &timestamp, &entry.Workspace, &entry.ToolName,
			&entry.FilePath, &entry.BackupBefore, &entry.BackupAfter, &entry.Action, &undone); err != nil {
			return nil, err
		}
		entry.Timestamp, _ = time.Parse(time.RFC3339, timestamp)
		entry.Undone = undone == 1
		store.Entries = append(store.Entries, entry)
	}

	return store, nil
}

func saveHistoryInternal(sessionID string, store *HistoryStore) error {
	if sessionID == "" {
		return nil
	}

	db, err := getSessionHistoryDB(sessionID)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tx.Exec("DELETE FROM history")

	stmt, err := tx.Prepare(`
		INSERT INTO history (id, timestamp, workspace, tool_name, file_path, backup_before, backup_after, action, undone)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, entry := range store.Entries {
		undone := 0
		if entry.Undone {
			undone = 1
		}
		_, err := stmt.Exec(entry.ID, entry.Timestamp.Format(time.RFC3339), entry.Workspace,
			entry.ToolName, entry.FilePath, entry.BackupBefore, entry.BackupAfter, entry.Action, undone)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func RecordChange(workDir, relPath, toolName, action string, oldContent, newContent []byte) error {
	sessionID := GetCurrentSessionID()
	if sessionID == "" {
		return nil
	}

	sessionlock.LockWrite(sessionID)
	defer sessionlock.UnlockWrite(sessionID)

	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		absWorkDir = workDir
	}

	store, err := loadHistoryInternal(sessionID)
	if err != nil {
		return err
	}

	var activeEntries []HistoryEntry
	for _, entry := range store.Entries {
		if entry.Workspace == absWorkDir && entry.Undone {
			if entry.BackupBefore != "" {
				_ = os.Remove(entry.BackupBefore)
			}
			if entry.BackupAfter != "" {
				_ = os.Remove(entry.BackupAfter)
			}
			continue
		}
		activeEntries = append(activeEntries, entry)
	}
	store.Entries = activeEntries

	id := generateID()

	backupBeforePath, err := writeBackup(sessionID, id, "before", oldContent)
	if err != nil {
		return err
	}

	backupAfterPath, err := writeBackup(sessionID, id, "after", newContent)
	if err != nil {
		return err
	}

	entry := HistoryEntry{
		ID:           id,
		Timestamp:    time.Now(),
		Workspace:    absWorkDir,
		ToolName:     toolName,
		FilePath:     relPath,
		BackupBefore: backupBeforePath,
		BackupAfter:  backupAfterPath,
		Action:       action,
		Undone:       false,
	}

	store.Entries = append(store.Entries, entry)

	if len(store.Entries) > 100 {
		removed := store.Entries[0]
		if removed.BackupBefore != "" {
			_ = os.Remove(removed.BackupBefore)
		}
		if removed.BackupAfter != "" {
			_ = os.Remove(removed.BackupAfter)
		}
		store.Entries = store.Entries[1:]
	}

	return saveHistoryInternal(sessionID, store)
}

func Undo(workDir string, steps int) (string, error) {
	sessionID := GetCurrentSessionID()
	if sessionID == "" {
		return "No active session.", nil
	}

	sessionlock.LockWrite(sessionID)
	defer sessionlock.UnlockWrite(sessionID)

	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		absWorkDir = workDir
	}

	store, err := loadHistoryInternal(sessionID)
	if err != nil {
		return "", err
	}

	undoneCount := 0
	var undoneFiles []string

	for i := len(store.Entries) - 1; i >= 0; i-- {
		if undoneCount >= steps {
			break
		}

		entry := &store.Entries[i]
		if entry.Workspace != absWorkDir || entry.Undone {
			continue
		}

		absFilePath := filepath.Join(absWorkDir, entry.FilePath)

		if _, err := resolvePath(absWorkDir, entry.FilePath); err != nil {
			return "", err
		}

		if entry.BackupBefore == "" {
			_ = os.Remove(absFilePath)
		} else {
			data, err := os.ReadFile(entry.BackupBefore)
			if err != nil {
				return "", fmt.Errorf("failed to read backup file: %w", err)
			}
			dir := filepath.Dir(absFilePath)
			_ = os.MkdirAll(dir, 0755)
			if err := os.WriteFile(absFilePath, data, 0644); err != nil {
				return "", fmt.Errorf("failed to restore file: %w", err)
			}
		}

		entry.Undone = true
		undoneCount++
		undoneFiles = append(undoneFiles, fmt.Sprintf("%s (%s)", entry.FilePath, entry.ToolName))
	}

	if undoneCount == 0 {
		return "No actions to undo in this workspace.", nil
	}

	if err := saveHistoryInternal(sessionID, store); err != nil {
		return "", err
	}

	return fmt.Sprintf("Successfully undone %d action(s): %v", undoneCount, undoneFiles), nil
}

func Redo(workDir string, steps int) (string, error) {
	sessionID := GetCurrentSessionID()
	if sessionID == "" {
		return "No active session.", nil
	}

	sessionlock.LockWrite(sessionID)
	defer sessionlock.UnlockWrite(sessionID)

	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		absWorkDir = workDir
	}

	store, err := loadHistoryInternal(sessionID)
	if err != nil {
		return "", err
	}

	redoneCount := 0
	var redoneFiles []string

	for i := 0; i < len(store.Entries); i++ {
		if redoneCount >= steps {
			break
		}

		entry := &store.Entries[i]
		if entry.Workspace != absWorkDir || !entry.Undone {
			continue
		}

		absFilePath := filepath.Join(absWorkDir, entry.FilePath)

		if _, err := resolvePath(absWorkDir, entry.FilePath); err != nil {
			return "", err
		}

		if entry.BackupAfter == "" {
			_ = os.Remove(absFilePath)
		} else {
			data, err := os.ReadFile(entry.BackupAfter)
			if err != nil {
				return "", fmt.Errorf("failed to read backup file: %w", err)
			}
			dir := filepath.Dir(absFilePath)
			_ = os.MkdirAll(dir, 0755)
			if err := os.WriteFile(absFilePath, data, 0644); err != nil {
				return "", fmt.Errorf("failed to redo file: %w", err)
			}
		}

		entry.Undone = false
		redoneCount++
		redoneFiles = append(redoneFiles, fmt.Sprintf("%s (%s)", entry.FilePath, entry.ToolName))
	}

	if redoneCount == 0 {
		return "No actions to redo in this workspace.", nil
	}

	if err := saveHistoryInternal(sessionID, store); err != nil {
		return "", err
	}

	return fmt.Sprintf("Successfully redone %d action(s): %v", redoneCount, redoneFiles), nil
}

func RestoreFile(workDir, relPath string, steps int) (string, error) {
	sessionID := GetCurrentSessionID()
	if sessionID == "" {
		return "No active session.", nil
	}

	sessionlock.LockWrite(sessionID)
	defer sessionlock.UnlockWrite(sessionID)

	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		absWorkDir = workDir
	}

	store, err := loadHistoryInternal(sessionID)
	if err != nil {
		return "", err
	}

	undoneCount := 0
	absFilePath := filepath.Join(absWorkDir, relPath)

	if _, err := resolvePath(absWorkDir, relPath); err != nil {
		return "", err
	}

	for i := len(store.Entries) - 1; i >= 0; i-- {
		if undoneCount >= steps {
			break
		}

		entry := &store.Entries[i]
		if entry.Workspace != absWorkDir || filepath.Clean(entry.FilePath) != filepath.Clean(relPath) || entry.Undone {
			continue
		}

		if entry.BackupBefore == "" {
			_ = os.Remove(absFilePath)
		} else {
			data, err := os.ReadFile(entry.BackupBefore)
			if err != nil {
				return "", fmt.Errorf("failed to read backup: %w", err)
			}
			dir := filepath.Dir(absFilePath)
			_ = os.MkdirAll(dir, 0755)
			if err := os.WriteFile(absFilePath, data, 0644); err != nil {
				return "", fmt.Errorf("failed to restore file: %w", err)
			}
		}

		entry.Undone = true
		undoneCount++
	}

	if undoneCount == 0 {
		return fmt.Sprintf("No active history entries found for file '%s'.", relPath), nil
	}

	if err := saveHistoryInternal(sessionID, store); err != nil {
		return "", err
	}

	return fmt.Sprintf("Successfully restored file '%s' back %d revision(s).", relPath, undoneCount), nil
}

func GetHistoryList(workDir string) ([]string, error) {
	sessionID := GetCurrentSessionID()
	if sessionID == "" {
		return nil, nil
	}

	sessionlock.LockRead(sessionID)
	defer sessionlock.UnlockRead(sessionID)

	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		absWorkDir = workDir
	}

	store, err := loadHistoryInternal(sessionID)
	if err != nil {
		return nil, err
	}

	var list []string
	for _, entry := range store.Entries {
		if entry.Workspace != absWorkDir {
			continue
		}
		status := "Active"
		if entry.Undone {
			status = "Undone"
		}
		list = append(list, fmt.Sprintf("[%s] %s: %s - %s", entry.Timestamp.Format("15:04:05"), status, entry.ToolName, entry.FilePath))
	}
	return list, nil
}

func ClearHistory() error {
	sessionID := GetCurrentSessionID()
	if sessionID == "" {
		return nil
	}

	sessionlock.LockWrite(sessionID)
	defer sessionlock.UnlockWrite(sessionID)

	db, err := getSessionHistoryDB(sessionID)
	if err != nil {
		return err
	}
	defer db.Close()

	db.Exec("DELETE FROM history")

	backupDir, err := getBackupDir(sessionID)
	if err == nil {
		files, err := os.ReadDir(backupDir)
		if err == nil {
			for _, f := range files {
				if !f.IsDir() && filepath.Ext(f.Name()) == ".bak" {
					_ = os.Remove(filepath.Join(backupDir, f.Name()))
				}
			}
		}
	}

	return nil
}

func SessionSearch(query string) string {
	if query == "" {
		return "[Error] query cannot be empty"
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Sprintf("[Error] failed to get user home directory: %v", err)
	}
	sessionsDir := filepath.Join(home, ".awas", "sessions")

	files, err := os.ReadDir(sessionsDir)
	if err != nil {
		return fmt.Sprintf("[Error] failed to read sessions directory: %v", err)
	}

	type SearchResult struct {
		SessionID string
		Title     string
		Role      string
		Content   string
		CreatedAt string
	}
	var results []SearchResult

	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".db" {
			continue
		}
		dbPath := filepath.Join(sessionsDir, f.Name())
		sessionID := strings.TrimSuffix(f.Name(), ".db")

		sessionlock.LockRead(sessionID)
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			sessionlock.UnlockRead(sessionID)
			continue
		}

		var title string
		_ = db.QueryRow("SELECT title FROM session WHERE id = ?", sessionID).Scan(&title)
		if title == "" {
			title = "Untitled Session"
		}

		rows, err := db.Query(`
			SELECT role, content, created_at
			FROM messages
			WHERE content LIKE ?
			ORDER BY seq ASC`, "%"+query+"%")
		if err == nil {
			for rows.Next() {
				var role, content, createdAt string
				if errScan := rows.Scan(&role, &content, &createdAt); errScan == nil {
					results = append(results, SearchResult{
						SessionID: sessionID,
						Title:     title,
						Role:      role,
						Content:   content,
						CreatedAt: createdAt,
					})
				}
			}
			rows.Close()
		}
		db.Close()
		sessionlock.UnlockRead(sessionID)
	}

	if len(results) == 0 {
		return "No messages found matching query: " + query
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d matching messages across sessions:\n\n", len(results)))
	for _, res := range results {
		snippet := res.Content
		snippet = strings.ReplaceAll(snippet, "\n", " ")
		if len(snippet) > 120 {
			snippet = snippet[:117] + "..."
		}
		sb.WriteString(fmt.Sprintf("- Session: %s (ID: %s)\n", res.Title, res.SessionID))
		sb.WriteString(fmt.Sprintf("  [%s] %s: %s\n\n", res.CreatedAt, res.Role, snippet))
	}
	return sb.String()
}


