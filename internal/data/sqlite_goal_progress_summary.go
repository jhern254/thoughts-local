package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// GoalProgressSummary separates explicit contributions from automatic Event
// attribution. Effective totals include the current reachable descendant set.
type GoalProgressSummary struct {
	DirectAutomaticSeconds    int64
	EffectiveAutomaticSeconds int64
	DirectManualSeconds       int64
	EffectiveManualSeconds    int64
}

func (s *SQLiteGoalProgressStore) GetGoalProgressSummary(ctx context.Context, userID string, goalID int64) (GoalProgressSummary, error) {
	var summary GoalProgressSummary
	// Pick the complete Event snapshot before selecting recipients. An empty
	// latest revision must suppress every recipient in an earlier revision.
	err := s.db.QueryRowContext(ctx, `
    WITH RECURSIVE requested(goal_id) AS (
        SELECT goal_id
        FROM goals
        WHERE user_id = ? AND goal_id = ? AND deleted_at IS NULL
            AND EXISTS (SELECT 1 FROM users WHERE user_id = ? AND deleted_at IS NULL)
    ), reachable(goal_id) AS (
        SELECT goal_id FROM requested
        UNION
        SELECT p.goal_id
        FROM goal_parents p
        JOIN reachable r ON r.goal_id = p.parent_goal_id
        WHERE p.user_id = ?
    ), latest(event_id, event_version) AS (
        SELECT event_id, MAX(event_version)
        FROM event_credit_revisions
        WHERE user_id = ?
        GROUP BY event_id
    ), recipient_events AS (
        SELECT c.goal_id, r.event_id, r.credited_end_at - r.credited_start_at AS seconds
        FROM latest l
        JOIN event_credit_revisions r ON r.event_id = l.event_id AND r.event_version = l.event_version
        JOIN event_goal_credits c ON c.event_id = r.event_id AND c.event_version = r.event_version
            AND c.user_id = r.user_id
        JOIN reachable g ON g.goal_id = c.goal_id
        JOIN events e ON e.event_id = r.event_id AND e.user_id = r.user_id
        WHERE r.user_id = ? AND e.deleted_at IS NULL
    ), unique_events AS (
        SELECT DISTINCT event_id, seconds FROM recipient_events
    ), manual AS (
        SELECT p.progress_id, p.goal_id, p.time_spent_sec
        FROM goal_progress p
        JOIN reachable r ON r.goal_id = p.goal_id
        WHERE p.user_id = ?
    )
    SELECT
        COALESCE((SELECT SUM(seconds) FROM recipient_events WHERE goal_id = requested.goal_id), 0),
        COALESCE((SELECT SUM(seconds) FROM unique_events), 0),
        COALESCE((SELECT SUM(time_spent_sec) FROM manual WHERE goal_id = requested.goal_id), 0),
        COALESCE((SELECT SUM(time_spent_sec) FROM manual), 0)
    FROM requested`, userID, goalID, userID, userID, userID, userID, userID).Scan(&summary.DirectAutomaticSeconds, &summary.EffectiveAutomaticSeconds, &summary.DirectManualSeconds, &summary.EffectiveManualSeconds)
	if errors.Is(err, sql.ErrNoRows) {
		return GoalProgressSummary{}, ErrRecordNotFound
	}
	if err != nil {
		return GoalProgressSummary{}, fmt.Errorf("read Goal progress summary: %w", TranslateSQLiteError(err))
	}
	return summary, nil
}
