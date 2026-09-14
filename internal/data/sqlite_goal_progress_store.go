package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type SQLiteGoalProgressStore struct{ db *sql.DB }

func NewSQLiteGoalProgressStore(db *sql.DB) *SQLiteGoalProgressStore {
	return &SQLiteGoalProgressStore{db: db}
}

const goalProgressColumns = `progress_id, goal_id, user_id, occurred_at, time_spent_sec, event_id, thought_id, progress_note, created_at`

// CreateGoalProgress appends explicit seconds and timestamps without deriving them
// from provenance. New contributions require undeleted same-owner references.
func (s *SQLiteGoalProgressStore) CreateGoalProgress(ctx context.Context, item *GoalProgress) (*GoalProgress, error) {
	created, err := scanGoalProgress(s.db.QueryRowContext(ctx, `INSERT INTO goal_progress
 (goal_id,user_id,occurred_at,time_spent_sec,event_id,thought_id,progress_note,created_at)
 SELECT ?,?,?,?,?,?,?,?
 WHERE EXISTS (SELECT 1 FROM users WHERE user_id=? AND deleted_at IS NULL)
 AND EXISTS (SELECT 1 FROM goals WHERE goal_id=? AND user_id=? AND deleted_at IS NULL)
 AND (? IS NULL OR EXISTS (SELECT 1 FROM events WHERE event_id=? AND user_id=? AND deleted_at IS NULL))
 AND (? IS NULL OR EXISTS (SELECT 1 FROM thoughts WHERE thought_id=? AND user_id=? AND deleted_at IS NULL))
 RETURNING `+goalProgressColumns,
		item.GoalID, item.UserID, item.OccurredAt.Unix(), item.TimeSpentSec, item.EventID, item.ThoughtID, item.ProgressNote, item.CreatedAt.Unix(),
		item.UserID, item.GoalID, item.UserID, item.EventID, item.EventID, item.UserID, item.ThoughtID, item.ThoughtID, item.UserID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRecordNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("create goal progress: %w", TranslateSQLiteError(err))
	}
	return created, nil
}

// GetGoalProgress includes retained history and provenance for an undeleted owner.
func (s *SQLiteGoalProgressStore) GetGoalProgress(ctx context.Context, userID string, progressID int64) (*GoalProgress, error) {
	item, err := scanGoalProgress(s.db.QueryRowContext(ctx, `SELECT `+goalProgressColumns+` FROM goal_progress
 WHERE user_id=? AND progress_id=? AND EXISTS (SELECT 1 FROM users WHERE user_id=? AND deleted_at IS NULL)`, userID, progressID, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRecordNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get goal progress: %w", TranslateSQLiteError(err))
	}
	return item, nil
}

// ListGoalProgress lists direct entries in [from, until), including retained history.
func (s *SQLiteGoalProgressStore) ListGoalProgress(ctx context.Context, userID string, goalID int64, from, until time.Time) (_ []GoalProgress, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("list goal progress: %w", TranslateSQLiteError(err))
		}
	}()
	rows, err := s.db.QueryContext(ctx, `SELECT `+goalProgressColumns+` FROM goal_progress
 WHERE user_id=? AND goal_id=? AND occurred_at>=? AND occurred_at<?
 AND EXISTS (SELECT 1 FROM users WHERE user_id=? AND deleted_at IS NULL)
 ORDER BY occurred_at,progress_id`, userID, goalID, from.Unix(), until.Unix(), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]GoalProgress, 0)
	for rows.Next() {
		item, err := scanGoalProgress(rows)
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

func scanGoalProgress(row interface{ Scan(...any) error }) (*GoalProgress, error) {
	var item GoalProgress
	var occurred, created int64
	err := row.Scan(&item.ProgressID, &item.GoalID, &item.UserID, &occurred, &item.TimeSpentSec, &item.EventID, &item.ThoughtID, &item.ProgressNote, &created)
	if err != nil {
		return nil, err
	}
	item.OccurredAt, item.CreatedAt = TimeFromUnixSec(occurred), TimeFromUnixSec(created)
	return &item, nil
}
