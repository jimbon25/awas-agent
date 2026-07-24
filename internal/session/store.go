package session

import (
	"awas/internal/client"
	"awas/internal/sessionlock"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	dbPool   map[string]*sql.DB
	dbPoolMu sync.Mutex
}

var defaultStore *Store
var defaultStoreOnce sync.Once

func Default() *Store {
	defaultStoreOnce.Do(func() {
		defaultStore = &Store{
			dbPool: make(map[string]*sql.DB),
		}
	})
	return defaultStore
}

func (s *Store) CloseAll() {
	s.dbPoolMu.Lock()
	defer s.dbPoolMu.Unlock()
	for id, db := range s.dbPool {
		db.Close()
		delete(s.dbPool, id)
	}
}

type SessionData struct {
	ID               string
	Title            string
	WorkDir          string
	Provider         string
	Model            string
	Mode             string
	AgentMode        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	History          []client.Message
	CompressedTurns  int
}

func (s *Store) getDB(id string) (*sql.DB, error) {
	s.dbPoolMu.Lock()
	if db, ok := s.dbPool[id]; ok {
		s.dbPoolMu.Unlock()
		return db, nil
	}
	s.dbPoolMu.Unlock()

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".awas", "sessions")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	path := filepath.Join(dir, id+".db")
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

	CREATE TABLE IF NOT EXISTS subagent_logs (
		id         TEXT PRIMARY KEY,
		role       TEXT NOT NULL,
		prompt     TEXT NOT NULL,
		status     TEXT NOT NULL,
		result     TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		ended_at   TEXT NOT NULL DEFAULT ''
	);
	`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}

	db.Exec("ALTER TABLE session ADD COLUMN agent_mode TEXT NOT NULL DEFAULT ''")

	s.dbPoolMu.Lock()
	s.dbPool[id] = db
	s.dbPoolMu.Unlock()

	return db, nil
}

func (s *Store) Save(data *SessionData) error {
	if data == nil || data.ID == "" {
		return fmt.Errorf("invalid session")
	}
	if len(data.History) == 0 {
		return nil
	}

	sessionlock.LockWrite(data.ID)
	defer sessionlock.UnlockWrite(data.ID)

	db, err := s.getDB(data.ID)
	if err != nil {
		return err
	}

	data.UpdatedAt = time.Now()
	if data.Title == "" {
		for _, msg := range data.History {
			if msg.Role == "user" {
				title := msg.Content
				if len(title) > 40 {
					title = title[:37] + "..."
				}
				data.Title = title
				break
			}
		}
	}
	if data.Title == "" {
		data.Title = "Gateway Session"
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT OR REPLACE INTO session
		(id, title, workdir, provider, model, mode, agent_mode, created_at, updated_at, token_count, compressed_turns)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		data.ID, data.Title, data.WorkDir, data.Provider, data.Model, data.Mode, data.AgentMode,
		data.CreatedAt.Format(time.RFC3339), data.UpdatedAt.Format(time.RFC3339),
		0, data.CompressedTurns,
	)
	if err != nil {
		return err
	}

	if _, err := tx.Exec("DELETE FROM messages WHERE msg_type='client'"); err != nil {
		return err
	}
	if len(data.History) > 0 {
		stmt, err := tx.Prepare(`
			INSERT INTO messages (msg_type, role, content, name, tool_call_id, tool_calls_json, seq)
			VALUES ('client', ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for i, msg := range data.History {
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

func (s *Store) Load(id string) (*SessionData, error) {
	sessionlock.LockRead(id)
	defer sessionlock.UnlockRead(id)

	db, err := s.getDB(id)
	if err != nil {
		return nil, err
	}

	data := &SessionData{}
	var createdAt, updatedAt string
	err = db.QueryRow(`
		SELECT id, title, workdir, provider, model, mode, agent_mode,
		       created_at, updated_at, compressed_turns
		FROM session WHERE id = ?`, id,
	).Scan(&data.ID, &data.Title, &data.WorkDir, &data.Provider, &data.Model, &data.Mode, &data.AgentMode,
		&createdAt, &updatedAt, &data.CompressedTurns)
	if err != nil {
		return nil, err
	}

	data.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	data.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	rows, err := db.Query(`
		SELECT role, content, name, tool_call_id, tool_calls_json
		FROM messages
		WHERE msg_type = 'client'
		ORDER BY seq`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var msg client.Message
		var toolCallsJSON string
		if err := rows.Scan(&msg.Role, &msg.Content, &msg.Name, &msg.ToolCallID, &toolCallsJSON); err != nil {
			return nil, err
		}
		if toolCallsJSON != "[]" && toolCallsJSON != "" {
			json.Unmarshal([]byte(toolCallsJSON), &msg.ToolCalls)
		}
		data.History = append(data.History, msg)
	}

	return data, nil
}

func (s *Store) Delete(id string) error {
	s.dbPoolMu.Lock()
	if db, ok := s.dbPool[id]; ok {
		db.Close()
		delete(s.dbPool, id)
	}
	s.dbPoolMu.Unlock()

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".awas", "sessions", id+".db")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *Store) SaveSubagentLog(sessionID string, id string, role string, prompt string, status string, result string, createdAt time.Time, endedAt time.Time) error {
	if sessionID == "" {
		sessionID = "global"
	}
	sessionlock.LockWrite(sessionID)
	defer sessionlock.UnlockWrite(sessionID)

	db, err := s.getDB(sessionID)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT OR REPLACE INTO subagent_logs (id, role, prompt, status, result, created_at, ended_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, role, prompt, status, result, createdAt.Format(time.RFC3339), endedAt.Format(time.RFC3339))
	return err
}
