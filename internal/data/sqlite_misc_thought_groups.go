package data

import (
	"context"
	"fmt"
	"time"
)

// MiscThoughtGroup is a point on the timeline, not a persisted event or an
// assertion about the user's activity. Missing event coverage means unknown activity.
type MiscThoughtGroup struct {
	ObservedAt time.Time
	Count      int64
}

// MiscThoughtGroups groups thoughts outside recorded event intervals. Coverage
// follows the same half-open intervals and captured ongoing cutoff as event counts.
func (s *SQLiteMetricsStore) MiscThoughtGroups(ctx context.Context, userID string, from, until, ongoingUntil time.Time) ([]MiscThoughtGroup, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT t.observed_at, COUNT(*)
        FROM thoughts t JOIN users u ON u.user_id=t.user_id
        WHERE t.user_id=? AND t.deleted_at IS NULL AND u.deleted_at IS NULL
          AND t.observed_at>=? AND t.observed_at<? AND t.observed_at<?
          AND NOT EXISTS (
            SELECT 1 FROM events e
            WHERE e.user_id=t.user_id AND e.deleted_at IS NULL
              AND e.started_at<=t.observed_at AND t.observed_at<COALESCE(e.ended_at,?)
          )
        GROUP BY t.observed_at ORDER BY t.observed_at`, userID, from.Unix(), until.Unix(), ongoingUntil.Unix(), ongoingUntil.Unix())
	if err != nil {
		return nil, fmt.Errorf("read misc thought groups: %w", TranslateSQLiteError(err))
	}
	defer rows.Close()
	var groups []MiscThoughtGroup
	for rows.Next() {
		var group MiscThoughtGroup
		var observed int64
		if err := rows.Scan(&observed, &group.Count); err != nil {
			return nil, fmt.Errorf("scan misc thought group: %w", TranslateSQLiteError(err))
		}
		group.ObservedAt = TimeFromUnixSec(observed)
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read misc thought groups: %w", TranslateSQLiteError(err))
	}
	return groups, nil
}
