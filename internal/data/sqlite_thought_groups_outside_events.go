package data

import (
	"context"
	"fmt"
	"time"
)

// ThoughtGroupOutsideEvents counts thoughts sharing an observation timestamp
// outside recorded event intervals. Missing event coverage means unknown activity;
// the group is a read projection, not a persisted event.
type ThoughtGroupOutsideEvents struct {
	ObservedAt   time.Time
	ThoughtCount int64
}

// ListThoughtGroupsOutsideEvents groups thoughts by observation timestamp outside
// recorded event intervals. Coverage follows the same half-open intervals and
// captured ongoing cutoff as event counts.
func (s *SQLiteMetricsStore) ListThoughtGroupsOutsideEvents(ctx context.Context, userID string, from, until, ongoingUntil time.Time) ([]ThoughtGroupOutsideEvents, error) {
	// Materialize only overlapping Events once. Without this boundary SQLite
	// scans historical Events again for every Thought in the requested window.
	// Keep spanning Events: their start may precede from, and their end may
	// follow until. The inner test still applies exact half-open coverage.
	rows, err := s.db.QueryContext(ctx, `
        WITH covering_events AS MATERIALIZED (
          SELECT started_at, COALESCE(ended_at,?) AS ended_at
          FROM events
          WHERE user_id=? AND deleted_at IS NULL
            AND started_at<? AND COALESCE(ended_at,?)>?
        )
        SELECT t.observed_at, COUNT(*)
        FROM thoughts t JOIN users u ON u.user_id=t.user_id
        WHERE t.user_id=? AND t.deleted_at IS NULL AND u.deleted_at IS NULL
          AND t.observed_at>=? AND t.observed_at<? AND t.observed_at<?
          AND NOT EXISTS (
            SELECT 1 FROM covering_events e
            WHERE e.started_at<=t.observed_at AND t.observed_at<e.ended_at
          )
        GROUP BY t.observed_at ORDER BY t.observed_at`, ongoingUntil.Unix(), userID, until.Unix(), ongoingUntil.Unix(), from.Unix(),
		userID, from.Unix(), until.Unix(), ongoingUntil.Unix())
	if err != nil {
		return nil, fmt.Errorf("read thought groups outside events: %w", TranslateSQLiteError(err))
	}
	defer rows.Close()
	var thoughtGroupsOutsideEvents []ThoughtGroupOutsideEvents
	for rows.Next() {
		var group ThoughtGroupOutsideEvents
		var observed int64
		if err := rows.Scan(&observed, &group.ThoughtCount); err != nil {
			return nil, fmt.Errorf("scan thought group outside events: %w", TranslateSQLiteError(err))
		}
		group.ObservedAt = TimeFromUnixSec(observed)
		thoughtGroupsOutsideEvents = append(thoughtGroupsOutsideEvents, group)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read thought groups outside events: %w", TranslateSQLiteError(err))
	}
	return thoughtGroupsOutsideEvents, nil
}
