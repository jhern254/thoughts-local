package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type SQLiteGoalProgressStore struct{ db *sql.DB }

func NewSQLiteGoalProgressStore(db *sql.DB) *SQLiteGoalProgressStore {
	return &SQLiteGoalProgressStore{db: db}
}

// GoalProgressSummary counts each completed Event once across the Goal and its
// current descendants. Event intervals remain the sole source of earned time.
type GoalProgressSummary struct{ LifetimeEarnedSeconds int64 }

func (s *SQLiteGoalProgressStore) GetGoalProgressSummary(ctx context.Context, userID string, goalID int64) (GoalProgressSummary, error) {
	var summary GoalProgressSummary
	var invalidIntervals int64
	err := s.db.QueryRowContext(ctx, `
    WITH RECURSIVE requested(goal_id) AS (
        SELECT g.goal_id
        FROM goals g
        JOIN users u ON u.user_id = g.user_id
        WHERE g.user_id = ? AND g.goal_id = ?
            AND g.deleted_at IS NULL AND u.deleted_at IS NULL
    ), reachable(goal_id) AS (
        SELECT goal_id FROM requested
        UNION
        SELECT p.goal_id
        FROM goal_parents p
        JOIN reachable r ON r.goal_id = p.parent_goal_id
        WHERE p.user_id = ?
    ), earned_events(event_id) AS (
        SELECT DISTINCT p.event_id
        FROM goal_progress p
        JOIN reachable r ON r.goal_id = p.goal_id
        WHERE p.user_id = ?
    ), intervals AS (
        SELECT
            e.ended_at - e.started_at AS seconds,
            typeof(e.started_at) = 'integer' AND typeof(e.ended_at) = 'integer'
                AND typeof(e.ended_at - e.started_at) = 'integer'
                AND e.ended_at >= e.started_at AS valid
        FROM earned_events p
        JOIN events e ON e.event_id = p.event_id
        WHERE e.user_id = ? AND e.deleted_at IS NULL AND e.ended_at IS NOT NULL
    )
    SELECT
        COALESCE((SELECT SUM(CASE WHEN valid THEN seconds ELSE 0 END) FROM intervals), 0),
        (SELECT COUNT(*) FROM intervals WHERE NOT valid)
    FROM requested`, userID, goalID, userID, userID, userID).Scan(&summary.LifetimeEarnedSeconds, &invalidIntervals)
	if errors.Is(err, sql.ErrNoRows) {
		return GoalProgressSummary{}, ErrRecordNotFound
	}
	if err != nil {
		return GoalProgressSummary{}, fmt.Errorf("read Goal progress summary: %w", TranslateSQLiteError(err))
	}
	if invalidIntervals != 0 {
		return GoalProgressSummary{}, ErrInvalidEventInterval
	}
	return summary, nil
}
