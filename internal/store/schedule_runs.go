package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// CreateScheduleRun inserts a run record in the running state. If ID is empty a
// UUID is assigned. The session linkage is filled in by FinishScheduleRun once
// the run's session exists.
func (s *Store) CreateScheduleRun(ctx context.Context, run ScheduleRun) (ScheduleRun, error) {
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
	if run.Status == "" {
		run.Status = RunRunning
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO schedule_runs
		(id, schedule_name, session_id, trigger, status, error)
		VALUES (?, ?, NULLIF(?, ''), ?, ?, ?)`,
		run.ID, run.ScheduleName, run.SessionID, run.Trigger, run.Status, run.Error,
	)
	if err != nil {
		return ScheduleRun{}, fmt.Errorf("create schedule run %q: %w", run.ID, err)
	}
	return s.GetScheduleRun(ctx, run.ID)
}

// FinishScheduleRun records the terminal status, the session it produced, and any
// error for a run.
func (s *Store) FinishScheduleRun(ctx context.Context, id, sessionID string, status RunStatus, runErr string) (ScheduleRun, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE schedule_runs
		SET session_id = NULLIF(?, ''), status = ?, error = ?, finished_at = datetime('now')
		WHERE id = ?`, sessionID, status, runErr, id)
	if err != nil {
		return ScheduleRun{}, fmt.Errorf("finish schedule run %q: %w", id, err)
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return ScheduleRun{}, fmt.Errorf("finish schedule run %q rows affected: %w", id, err)
	}
	if changed == 0 {
		return ScheduleRun{}, fmt.Errorf("schedule run %q: %w", id, ErrNotFound)
	}
	return s.GetScheduleRun(ctx, id)
}

// GetScheduleRun fetches a single run by ID.
func (s *Store) GetScheduleRun(ctx context.Context, id string) (ScheduleRun, error) {
	row := s.db.QueryRowContext(ctx, scheduleRunSelect+` WHERE id = ?`, id)
	run, err := scanScheduleRun(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ScheduleRun{}, fmt.Errorf("schedule run %q: %w", id, ErrNotFound)
		}
		return ScheduleRun{}, err
	}
	return run, nil
}

// ListScheduleRuns returns the most recent runs for a schedule, newest first.
// A limit <= 0 returns all runs.
func (s *Store) ListScheduleRuns(ctx context.Context, scheduleName string, limit int) ([]ScheduleRun, error) {
	query := scheduleRunSelect + ` WHERE schedule_name = ? ORDER BY started_at DESC, id DESC`
	args := []any{scheduleName}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list schedule runs for %q: %w", scheduleName, err)
	}
	defer rows.Close()

	var runs []ScheduleRun
	for rows.Next() {
		run, err := scanScheduleRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

// DeleteScheduleRuns removes all run-history rows for a schedule. The sessions
// those runs produced are left intact; only the run linkage is dropped, which is
// used when a schedule file is deleted.
func (s *Store) DeleteScheduleRuns(ctx context.Context, scheduleName string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM schedule_runs WHERE schedule_name = ?`, scheduleName); err != nil {
		return fmt.Errorf("delete schedule runs for %q: %w", scheduleName, err)
	}
	return nil
}

const scheduleRunSelect = `SELECT id, schedule_name, COALESCE(session_id, ''), trigger, status, error,
	started_at, COALESCE(finished_at, '') FROM schedule_runs`

func scanScheduleRun(row scanner) (ScheduleRun, error) {
	var run ScheduleRun
	if err := row.Scan(
		&run.ID,
		&run.ScheduleName,
		&run.SessionID,
		&run.Trigger,
		&run.Status,
		&run.Error,
		&run.StartedAt,
		&run.FinishedAt,
	); err != nil {
		return ScheduleRun{}, err
	}
	return run, nil
}
