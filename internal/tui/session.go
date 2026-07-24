package tui

import (
	"awas/internal/client"
	"awas/internal/sessionlock"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var (
	sessionDBPool   = make(map[string]*sql.DB)
	sessionDBPoolMu sync.Mutex
)

func CloseAllSessionDB() {
	sessionDBPoolMu.Lock()
	defer sessionDBPoolMu.Unlock()
	for id, db := range sessionDBPool {
		db.Close()
		delete(sessionDBPool, id)
	}
}

type Session struct {
	ID               string
	Title            string
	WorkDir          string
	Provider         string
	Model            string
	Mode             string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Messages         []UIMessage
	History          []client.Message
	TokenCount       int
	CompressedTurns  int
}

type SessionMeta struct {
	ID        string
	Title     string
	WorkDir   string
	Steps     int
	UpdatedAt time.Time
}

func getSessionPath(id string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".awas", "sessions")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, id+".db"), nil
}

func getSessionDB(id string) (*sql.DB, error) {
	sessionDBPoolMu.Lock()
	if db, ok := sessionDBPool[id]; ok {
		sessionDBPoolMu.Unlock()
		return db, nil
	}
	sessionDBPoolMu.Unlock()

	path, err := getSessionPath(id)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA synchronous=NORMAL")
	db.Exec("PRAGMA cache_size=-8000")
	db.Exec("PRAGMA foreign_keys=ON")
	db.Exec("PRAGMA busy_timeout=5000")
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	schema := `
	CREATE TABLE IF NOT EXISTS session (
		id               TEXT PRIMARY KEY,
		title            TEXT NOT NULL DEFAULT 'New Conversation',
		workdir          TEXT NOT NULL DEFAULT '',
		provider         TEXT NOT NULL DEFAULT '',
		model            TEXT NOT NULL DEFAULT '',
		mode             TEXT NOT NULL DEFAULT '',
		created_at       TEXT NOT NULL,
		updated_at       TEXT NOT NULL,
		token_count      INTEGER NOT NULL DEFAULT 0,
		compressed_turns INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS messages (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		msg_type        TEXT NOT NULL CHECK(msg_type IN ('ui', 'client')),
		role            TEXT NOT NULL,
		content         TEXT NOT NULL DEFAULT '',
		name            TEXT NOT NULL DEFAULT '',
		success         INTEGER NOT NULL DEFAULT 1,
		tool_call_id    TEXT NOT NULL DEFAULT '',
		tool_calls_json TEXT NOT NULL DEFAULT '[]',
		seq             INTEGER NOT NULL DEFAULT 0,
		created_at      TEXT NOT NULL DEFAULT (datetime('now'))
	);

	CREATE INDEX IF NOT EXISTS idx_messages_seq ON messages(msg_type, seq);

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

	sessionDBPoolMu.Lock()
	sessionDBPool[id] = db
	sessionDBPoolMu.Unlock()

	return db, nil
}

func SaveSession(s *Session, lastSavedSeq int) error {
	if s == nil || s.ID == "" {
		return fmt.Errorf("invalid session")
	}
	if len(s.Messages) == 0 {
		return nil
	}

	sessionlock.LockWrite(s.ID)
	defer sessionlock.UnlockWrite(s.ID)

	db, err := getSessionDB(s.ID)
	if err != nil {
		return err
	}

	s.UpdatedAt = time.Now()
	if s.Title == "" {
		for _, msg := range s.Messages {
			if msg.Role == "user" {
				title := msg.Content
				if len(title) > 40 {
					title = title[:37] + "..."
				}
				s.Title = title
				break
			}
		}
	}
	if s.Title == "" {
		s.Title = "Untitled Conversation"
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT OR REPLACE INTO session 
		(id, title, workdir, provider, model, mode, created_at, updated_at, token_count, compressed_turns)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Title, s.WorkDir, s.Provider, s.Model, s.Mode,
		s.CreatedAt.Format(time.RFC3339), s.UpdatedAt.Format(time.RFC3339),
		s.TokenCount, s.CompressedTurns,
	)
	if err != nil {
		return err
	}

	if len(s.Messages) > 0 && lastSavedSeq < len(s.Messages)-1 {
		if _, err := tx.Exec("DELETE FROM messages WHERE msg_type='ui' AND seq > ?", lastSavedSeq); err != nil {
			return err
		}

		stmt, err := tx.Prepare(`
			INSERT INTO messages (msg_type, role, content, name, success, seq)
			VALUES ('ui', ?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}

		for i, msg := range s.Messages {
			if i <= lastSavedSeq {
				continue
			}
			success := 0
			if msg.Success {
				success = 1
			}
			if _, err := stmt.Exec(msg.Role, msg.Content, msg.Name, success, i); err != nil {
				stmt.Close()
				return err
			}
		}
		stmt.Close()
	}

	if _, err := tx.Exec("DELETE FROM messages WHERE msg_type='client'"); err != nil {
		return err
	}
	if len(s.History) > 0 {
		stmt, err := tx.Prepare(`
			INSERT INTO messages (msg_type, role, content, name, tool_call_id, tool_calls_json, seq)
			VALUES ('client', ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for i, msg := range s.History {
			toolCallsJSON := "[]"
			if len(msg.ToolCalls) > 0 {
				data, _ := json.Marshal(msg.ToolCalls)
				toolCallsJSON = string(data)
			}
			if _, err := stmt.Exec(msg.Role, msg.Content, msg.Name, msg.ToolCallID, toolCallsJSON, i); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func LoadSession(id string) (*Session, error) {
	sessionlock.LockRead(id)
	defer sessionlock.UnlockRead(id)

	db, err := getSessionDB(id)
	if err != nil {
		return nil, err
	}

	s := &Session{}
	var createdAt, updatedAt string
	err = db.QueryRow(`
		SELECT id, title, workdir, provider, model, mode, 
		       created_at, updated_at, token_count, compressed_turns
		FROM session WHERE id = ?`, id,
	).Scan(&s.ID, &s.Title, &s.WorkDir, &s.Provider, &s.Model, &s.Mode,
		&createdAt, &updatedAt, &s.TokenCount, &s.CompressedTurns)
	if err != nil {
		return nil, err
	}

	s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	s.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	rows, err := db.Query(`
		SELECT role, content, name, success
		FROM messages 
		WHERE msg_type = 'ui'
		ORDER BY seq`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var msg UIMessage
		var success int
		if err := rows.Scan(&msg.Role, &msg.Content, &msg.Name, &success); err != nil {
			return nil, err
		}
		msg.Success = success == 1
		s.Messages = append(s.Messages, msg)
	}

	rows2, err := db.Query(`
		SELECT role, content, name, tool_call_id, tool_calls_json
		FROM messages 
		WHERE msg_type = 'client'
		ORDER BY seq`)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()

	for rows2.Next() {
		var msg client.Message
		var toolCallsJSON string
		if err := rows2.Scan(&msg.Role, &msg.Content, &msg.Name, &msg.ToolCallID, &toolCallsJSON); err != nil {
			return nil, err
		}
		if toolCallsJSON != "[]" && toolCallsJSON != "" {
			json.Unmarshal([]byte(toolCallsJSON), &msg.ToolCalls)
		}
		s.History = append(s.History, msg)
	}

	return s, nil
}

func ListSessions() ([]SessionMeta, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	dir := filepath.Join(home, ".awas", "sessions")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.db"))
	if err != nil {
		return nil, err
	}

	var list []SessionMeta
	for _, file := range files {
		meta, err := getSessionMeta(file)
		if err != nil {
			continue
		}
		list = append(list, meta)
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].UpdatedAt.After(list[j].UpdatedAt)
	})

	return list, nil
}

func getSessionMeta(dbPath string) (SessionMeta, error) {
	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return SessionMeta{}, err
	}
	defer db.Close()

	var meta SessionMeta
	var updatedAt string

	err = db.QueryRow(`
		SELECT s.id, s.title, s.workdir,
		       (SELECT COUNT(*) FROM messages m WHERE m.msg_type = 'ui') as steps,
		       s.updated_at
		FROM session s
		LIMIT 1`,
	).Scan(&meta.ID, &meta.Title, &meta.WorkDir, &meta.Steps, &updatedAt)
	if err != nil {
		return SessionMeta{}, err
	}

	meta.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return meta, nil
}

func DeleteSession(id string) error {
	sessionlock.LockWrite(id)
	defer sessionlock.UnlockWrite(id)

	path, err := getSessionPath(id)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func RenameSession(id string, newTitle string) error {
	sessionlock.LockWrite(id)
	defer sessionlock.UnlockWrite(id)

	db, err := getSessionDB(id)
	if err != nil {
		return err
	}

	_, err = db.Exec("UPDATE session SET title = ? WHERE id = ?", newTitle, id)
	return err
}


