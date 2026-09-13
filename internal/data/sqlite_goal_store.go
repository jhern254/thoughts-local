package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// SQLiteGoalStore persists goal records without changing progress or parent links.
// Callers supply explicit settings and timestamps; normalization and defaults
// belong to the service. Returned records never include soft-deleted goals.
type SQLiteGoalStore struct{ db *sql.DB }

func NewSQLiteGoalStore(db *sql.DB) *SQLiteGoalStore { return &SQLiteGoalStore{db: db} }

const goalColumns = `goal_id, user_id, goal_name, target_seconds, goal_start_date,
    goal_end_date, goal_is_active, cadence, tz, week_start, default_cadence,
    version, created_at, updated_at`

const visibleGoals = `deleted_at IS NULL AND EXISTS
    (SELECT 1 FROM users WHERE users.user_id = goals.user_id AND users.deleted_at IS NULL)`

func (s *SQLiteGoalStore) CreateGoal(ctx context.Context, item *Goal) (*Goal, error) {
	created, err := scanGoal(s.db.QueryRowContext(ctx, `INSERT INTO goals
        (user_id, goal_name, target_seconds, goal_start_date, goal_end_date,
         goal_is_active, cadence, tz, week_start, default_cadence, created_at, updated_at)
        SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ? WHERE EXISTS
        (SELECT 1 FROM users WHERE user_id = ? AND deleted_at IS NULL)
        RETURNING `+goalColumns, item.UserID, item.GoalName, item.TargetSeconds,
		item.StartDate, item.EndDate, item.IsActive, item.Cadence, item.TZ, item.WeekStart,
		item.DefaultCadence, item.CreatedAt.Unix(), item.UpdatedAt.Unix(), item.UserID))
	if err != nil {
		return nil, fmt.Errorf("create goal: %w", translateGoalError(err))
	}
	return created, nil
}

func (s *SQLiteGoalStore) GetGoal(ctx context.Context, userID string, id int64) (*Goal, error) {
	item, err := scanGoal(s.db.QueryRowContext(ctx, `SELECT `+goalColumns+
		` FROM goals WHERE user_id = ? AND goal_id = ? AND `+visibleGoals, userID, id))
	if err != nil {
		return nil, fmt.Errorf("get goal: %w", TranslateSQLiteError(err))
	}
	return item, nil
}

// ListGoals includes both active and inactive undeleted goals in ID order.
func (s *SQLiteGoalStore) ListGoals(ctx context.Context, userID string) ([]Goal, error) {
	return s.listGoals(ctx, userID, nil)
}

func (s *SQLiteGoalStore) ListActiveGoals(ctx context.Context, userID string) ([]Goal, error) {
	active := true
	return s.listGoals(ctx, userID, &active)
}

func (s *SQLiteGoalStore) ListInactiveGoals(ctx context.Context, userID string) ([]Goal, error) {
	active := false
	return s.listGoals(ctx, userID, &active)
}

func (s *SQLiteGoalStore) listGoals(ctx context.Context, userID string, active *bool) (_ []Goal, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("list goals: %w", TranslateSQLiteError(err))
		}
	}()
	rows, err := s.db.QueryContext(ctx, `SELECT `+goalColumns+` FROM goals
        WHERE user_id = ? AND `+visibleGoals+`
        AND (? IS NULL OR goal_is_active = ?) ORDER BY goal_id`, userID, active, active)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Goal, 0)
	for rows.Next() {
		item, err := scanGoal(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// UpdateGoal replaces editable settings, requiring the caller's current version.
// Ownership, creation time, and related history are not editable here.
func (s *SQLiteGoalStore) UpdateGoal(ctx context.Context, item *Goal) (_ *Goal, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("update goal: %w", translateGoalError(err))
		}
	}()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE goals SET goal_name = ?, target_seconds = ?,
        goal_start_date = ?, goal_end_date = ?, goal_is_active = ?, cadence = ?, tz = ?,
        week_start = ?, default_cadence = ?, version = version + 1, updated_at = max(updated_at, ?)
        WHERE user_id = ? AND goal_id = ? AND version = ? AND `+visibleGoals,
		item.GoalName, item.TargetSeconds, item.StartDate, item.EndDate, item.IsActive,
		item.Cadence, item.TZ, item.WeekStart, item.DefaultCadence, item.UpdatedAt.Unix(),
		item.UserID, item.GoalID, item.Version)
	if err != nil {
		return nil, err
	}
	if err := checkGoalMutation(ctx, tx, result, item.UserID, item.GoalID); err != nil {
		return nil, err
	}
	updated, err := scanGoal(tx.QueryRowContext(ctx, `SELECT `+goalColumns+
		` FROM goals WHERE user_id = ? AND goal_id = ?`, item.UserID, item.GoalID))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return updated, nil
}

// DeleteGoal retains the row, active flag, progress, and parent links.
func (s *SQLiteGoalStore) DeleteGoal(ctx context.Context, userID string, id, version int64) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("delete goal: %w", TranslateSQLiteError(err))
		}
	}()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE goals
        SET deleted_at = max(unixepoch('now'), updated_at),
            updated_at = max(unixepoch('now'), updated_at), version = version + 1
        WHERE user_id = ? AND goal_id = ? AND version = ? AND `+visibleGoals, userID, id, version)
	if err != nil {
		return err
	}
	if err := checkGoalMutation(ctx, tx, result, userID, id); err != nil {
		return err
	}
	return tx.Commit()
}

// Inspect a missed conditional write in the same transaction so another writer
// cannot change whether the result is not-found or a version conflict.
func checkGoalMutation(ctx context.Context, tx *sql.Tx, result sql.Result, userID string, id int64) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 0 {
		return nil
	}
	var visible bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM goals
        WHERE user_id = ? AND goal_id = ? AND `+visibleGoals+`)`, userID, id).Scan(&visible)
	if err != nil {
		return err
	}
	if !visible {
		return ErrRecordNotFound
	}
	return ErrVersionConflict
}

func scanGoal(row interface{ Scan(...any) error }) (*Goal, error) {
	var item Goal
	var created, updated int64
	err := row.Scan(&item.GoalID, &item.UserID, &item.GoalName, &item.TargetSeconds,
		&item.StartDate, &item.EndDate, &item.IsActive, &item.Cadence, &item.TZ,
		&item.WeekStart, &item.DefaultCadence, &item.Version, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRecordNotFound
	}
	if err != nil {
		return nil, err
	}
	item.CreatedAt, item.UpdatedAt = TimeFromUnixSec(created), TimeFromUnixSec(updated)
	return &item, nil
}

func translateGoalError(err error) error {
	if isSQLiteUniqueConstraint(err) {
		return &databaseError{cause: err, kind: ErrDuplicateRecord}
	}
	return TranslateSQLiteError(err)
}
