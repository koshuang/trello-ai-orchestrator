package database

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

// InitDB initializes SQLite connection and runs simple migrations to ensure tables exist.
func InitDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping sqlite database: %w", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS processed_events (
		id TEXT PRIMARY KEY,
		provider TEXT NOT NULL DEFAULT 'trello',
		event_id TEXT NOT NULL UNIQUE,
		trello_card_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		processed_at DATETIME NOT NULL,
		status TEXT NOT NULL,
		error TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_processed_events_event_id ON processed_events(event_id);

	CREATE TABLE IF NOT EXISTS workflow_states (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		trello_card_id TEXT NOT NULL UNIQUE,
		trello_card_url TEXT NOT NULL,
		trello_card_title TEXT NOT NULL,
		status TEXT NOT NULL,
		last_processed_action_id TEXT NOT NULL,
		github_issue_url TEXT NOT NULL DEFAULT '',
		github_issue_number INTEGER NOT NULL DEFAULT 0,
		plan_path TEXT NOT NULL DEFAULT '',
		summary TEXT NOT NULL DEFAULT '',
		current_understanding TEXT NOT NULL DEFAULT '[]',
		decisions_json TEXT NOT NULL DEFAULT '[]',
		open_questions_json TEXT NOT NULL DEFAULT '[]',
		next_action TEXT NOT NULL DEFAULT '',
		updated_by TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_workflow_states_card_id ON workflow_states(trello_card_id);
	`

	_, err = db.Exec(schema)
	if err != nil {
		return nil, fmt.Errorf("failed to execute database schema: %w", err)
	}

	log.Printf("[Database] SQLite successfully initialized at %s", dbPath)
	return db, nil
}
