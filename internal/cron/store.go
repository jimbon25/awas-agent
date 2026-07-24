package cron

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type CronJob struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Schedule  string     `json:"schedule"` 
	Prompt    string     `json:"prompt"`
	Platform  string     `json:"platform"` 
	ChatID    string     `json:"chat_id"`
	GuildID   string     `json:"guild_id"` 
	Enabled   bool       `json:"enabled"`
	Skills    []string   `json:"skills"`   
	Model     string     `json:"model"`     
	WorkDir   string     `json:"workdir"`   
	CreatedAt time.Time  `json:"created_at"`
	LastRun   *time.Time `json:"last_run"`
	RunCount  int        `json:"run_count"`
}

type Store struct {
	db *sql.DB
}

func NewStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	dir := filepath.Join(home, ".awas")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	dbPath := filepath.Join(dir, "cron.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	schema := `
	CREATE TABLE IF NOT EXISTS cron_jobs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		schedule TEXT NOT NULL,
		prompt TEXT NOT NULL,
		platform TEXT NOT NULL,
		chat_id TEXT NOT NULL,
		guild_id TEXT,
		enabled INTEGER DEFAULT 1,
		skills TEXT,
		model TEXT,
		workdir TEXT,
		created_at TEXT DEFAULT CURRENT_TIMESTAMP,
		last_run TEXT,
		run_count INTEGER DEFAULT 0
	);`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) SaveJob(job *CronJob) error {
	skillsJSON, err := json.Marshal(job.Skills)
	if err != nil {
		return err
	}

	var lastRunStr string
	if job.LastRun != nil {
		lastRunStr = job.LastRun.Format(time.RFC3339)
	}

	enabledInt := 0
	if job.Enabled {
		enabledInt = 1
	}

	if job.ID == 0 {
		res, err := s.db.Exec(`
			INSERT INTO cron_jobs 
			(name, schedule, prompt, platform, chat_id, guild_id, enabled, skills, model, workdir, last_run, run_count)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			job.Name, job.Schedule, job.Prompt, job.Platform, job.ChatID, job.GuildID,
			enabledInt, string(skillsJSON), job.Model, job.WorkDir, lastRunStr, job.RunCount,
		)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err == nil {
			job.ID = id
		}
	} else {
		_, err = s.db.Exec(`
			UPDATE cron_jobs SET
				name = ?, schedule = ?, prompt = ?, platform = ?, chat_id = ?, guild_id = ?,
				enabled = ?, skills = ?, model = ?, workdir = ?, last_run = ?, run_count = ?
			WHERE id = ?`,
			job.Name, job.Schedule, job.Prompt, job.Platform, job.ChatID, job.GuildID,
			enabledInt, string(skillsJSON), job.Model, job.WorkDir, lastRunStr, job.RunCount,
			job.ID,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) DeleteJob(name string) error {
	_, err := s.db.Exec("DELETE FROM cron_jobs WHERE name = ?", name)
	return err
}

func (s *Store) GetJob(name string) (*CronJob, error) {
	rows, err := s.db.Query(`
		SELECT id, name, schedule, prompt, platform, chat_id, guild_id, enabled, skills, model, workdir, created_at, last_run, run_count
		FROM cron_jobs WHERE name = ?`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if rows.Next() {
		return scanJob(rows)
	}
	return nil, sql.ErrNoRows
}

func (s *Store) ListJobs() ([]*CronJob, error) {
	rows, err := s.db.Query(`
		SELECT id, name, schedule, prompt, platform, chat_id, guild_id, enabled, skills, model, workdir, created_at, last_run, run_count
		FROM cron_jobs ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*CronJob
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func scanJob(rows *sql.Rows) (*CronJob, error) {
	var job CronJob
	var enabledInt int
	var skillsStr, createdAtStr string
	var guildIDNull, modelNull, workdirNull, lastRunNull sql.NullString

	err := rows.Scan(
		&job.ID, &job.Name, &job.Schedule, &job.Prompt, &job.Platform, &job.ChatID,
		&guildIDNull, &enabledInt, &skillsStr, &modelNull, &workdirNull, &createdAtStr, &lastRunNull, &job.RunCount,
	)
	if err != nil {
		return nil, err
	}

	job.GuildID = guildIDNull.String
	job.Model = modelNull.String
	job.WorkDir = workdirNull.String
	job.Enabled = enabledInt == 1

	if skillsStr != "" {
		_ = json.Unmarshal([]byte(skillsStr), &job.Skills)
	}

	createdAtStr = strings.ReplaceAll(createdAtStr, " ", "T")
	if t, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
		job.CreatedAt = t
	} else if t, err := time.Parse("2006-01-02T15:04:05", createdAtStr); err == nil {
		job.CreatedAt = t
	} else if t, err := time.Parse("2006-01-02 15:04:05", createdAtStr); err == nil {
		job.CreatedAt = t
	}

	if lastRunNull.Valid && lastRunNull.String != "" {
		lastRunNullStr := strings.ReplaceAll(lastRunNull.String, " ", "T")
		if t2, err := time.Parse(time.RFC3339, lastRunNullStr); err == nil {
			job.LastRun = &t2
		} else if t2, err := time.Parse("2006-01-02T15:04:05", lastRunNullStr); err == nil {
			job.LastRun = &t2
		} else if t2, err := time.Parse("2006-01-02 15:04:05", lastRunNullStr); err == nil {
			job.LastRun = &t2
		}
	}

	return &job, nil
}
