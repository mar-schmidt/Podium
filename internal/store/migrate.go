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
