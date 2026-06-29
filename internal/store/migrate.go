package store

import "fmt"

// migration is one forward-only schema change. Migrations run in order and each
// runs at most once; their cumulative effect is recorded in schema_migrations.
type migration struct {
	version int
	name    string
	sql     string
}

// migrations is the ordered list of schema changes. Append new migrations here —
// never edit or reorder an already-shipped one. Phase 0 only establishes the
// bookkeeping table; Phase 1 adds agents, sessions, and message history.
var migrations = []migration{
	{
		version: 1,
		name:    "create_schema_migrations",
		sql: `CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY,
			name       TEXT    NOT NULL,
			applied_at TEXT    NOT NULL DEFAULT (datetime('now'))
		);`,
	},
	{
		version: 2,
		name:    "core_domain",
		sql: `CREATE TABLE agents (
			name            TEXT PRIMARY KEY,
			provider        TEXT NOT NULL CHECK (provider IN ('claude', 'codex')),
			profile         TEXT NOT NULL DEFAULT '',
			model           TEXT NOT NULL DEFAULT '',
			effort          TEXT NOT NULL DEFAULT '',
			permission_mode TEXT NOT NULL CHECK (permission_mode IN ('approve', 'yolo')),
			fallback_json   TEXT NOT NULL DEFAULT '[]',
			mcp_config      TEXT NOT NULL DEFAULT '',
			created_at      TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE TABLE sessions (
			id               TEXT PRIMARY KEY,
			agent_name       TEXT NOT NULL REFERENCES agents(name) ON UPDATE CASCADE ON DELETE RESTRICT,
			provider         TEXT NOT NULL CHECK (provider IN ('claude', 'codex')),
			profile          TEXT NOT NULL DEFAULT '',
			model            TEXT NOT NULL DEFAULT '',
			effort           TEXT NOT NULL DEFAULT '',
			permission_mode  TEXT NOT NULL CHECK (permission_mode IN ('approve', 'yolo')),
			origin           TEXT NOT NULL CHECK (origin IN ('web', 'cli', 'schedule', 'roadmap')),
			schedule_id      TEXT,
			run_id           TEXT,
			rolling_summary  TEXT NOT NULL DEFAULT '',
			provider_handle  TEXT NOT NULL DEFAULT '',
			created_at       TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at       TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE TRIGGER sessions_origin_immutable
		BEFORE UPDATE OF origin ON sessions
		BEGIN
			SELECT RAISE(ABORT, 'session origin is immutable');
		END;

		CREATE TABLE messages (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
			seq        INTEGER NOT NULL,
			role       TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
			content    TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			UNIQUE (session_id, seq)
		);

		CREATE INDEX idx_sessions_agent_name ON sessions(agent_name);
		CREATE INDEX idx_sessions_origin ON sessions(origin);
		CREATE INDEX idx_messages_session_seq ON messages(session_id, seq);`,
	},
}

// migrate applies every migration whose version has not yet been recorded. Each
// migration plus its bookkeeping insert runs in a single transaction, so a crash
// mid-migration leaves the schema consistent (no half-applied versions).
func (s *Store) migrate() error {
	// Ensure the bookkeeping table exists before we query it (the first migration
	// creates it; running its DDL up front is harmless and idempotent).
	if _, err := s.db.Exec(migrations[0].sql); err != nil {
		return fmt.Errorf("bootstrap schema_migrations: %w", err)
	}

	applied, err := s.appliedVersions()
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if applied[m.version] {
			continue
		}
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", m.version, err)
		}
		if _, err := tx.Exec(m.sql); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %d (%s): %w", m.version, m.name, err)
		}
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO schema_migrations (version, name) VALUES (?, ?)`,
			m.version, m.name,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.version, err)
		}
	}
	return nil
}

func (s *Store) appliedVersions() (map[int]bool, error) {
	rows, err := s.db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}
