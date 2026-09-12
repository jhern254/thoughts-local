package data

import (
	"context"
	"fmt"
	"time"
)

type EventThoughtCount struct {
	EventID int64
	Count   int64
}

// ThoughtCountsByEvent selects events intersecting the day, but counts their
// whole intervals. ongoingUntil is an exclusive, captured cutoff, not SQL now.
func (s *SQLiteMetricsStore) ThoughtCountsByEvent(ctx context.Context, userID string, from, until, ongoingUntil time.Time) ([]EventThoughtCount, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.event_id, COUNT(t.thought_id)
		FROM events e JOIN users u ON u.user_id = e.user_id
		LEFT JOIN thoughts t ON t.user_id = e.user_id AND t.deleted_at IS NULL
		 AND t.observed_at >= e.started_at AND t.observed_at < COALESCE(e.ended_at, ?)
		WHERE e.user_id = ? AND e.deleted_at IS NULL AND u.deleted_at IS NULL
		 AND e.started_at < ? AND (e.ended_at IS NULL OR e.ended_at > ? OR (e.ended_at=e.started_at AND e.started_at >= ?))
		GROUP BY e.event_id ORDER BY e.event_id`, ongoingUntil.Unix(), userID, until.Unix(), from.Unix(), from.Unix())
	if err != nil {
		return nil, fmt.Errorf("count event thoughts: %w", TranslateSQLiteError(err))
	}
	defer rows.Close()
	counts := []EventThoughtCount{}
	for rows.Next() {
		var item EventThoughtCount
		if err := rows.Scan(&item.EventID, &item.Count); err != nil {
			return nil, fmt.Errorf("scan event count: %w", TranslateSQLiteError(err))
		}
		counts = append(counts, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read event counts: %w", TranslateSQLiteError(err))
	}
	return counts, nil
}

func (s *SQLiteMetricsStore) CountThoughtsInRange(ctx context.Context, userID string, from, until time.Time) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM thoughts t JOIN users u ON u.user_id=t.user_id
		WHERE t.user_id=? AND t.deleted_at IS NULL AND u.deleted_at IS NULL AND t.observed_at >= ? AND t.observed_at < ?`, userID, from.Unix(), until.Unix()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count interval thoughts: %w", TranslateSQLiteError(err))
	}
	return count, nil
}
