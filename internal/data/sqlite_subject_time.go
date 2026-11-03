package data

import (
	"context"
	"fmt"
	"time"
)

// SubjectTimeView sums assigned event time within a requested interval.
type SubjectTimeView struct {
	SubjectID        int64
	SubjectName      string
	TimeSpentSeconds int64
}

func (s *SQLiteMetricsStore) SubjectTimeInRange(ctx context.Context, userID string, from, until, ongoingUntil time.Time) ([]SubjectTimeView, error) {
	rows, err := s.db.QueryContext(ctx, `
	    SELECT
	        subjects.subject_id,
	        subjects.subject_name,
	        SUM(max(0, min(COALESCE(events.ended_at, ?), ?) - max(events.started_at, ?))) AS seconds
	    FROM event_subjects
	    JOIN events ON events.event_id = event_subjects.event_id
	        AND events.user_id = event_subjects.user_id
	    JOIN subjects ON subjects.subject_id = event_subjects.subject_id
	        AND subjects.user_id = event_subjects.user_id
	    JOIN users ON users.user_id = events.user_id
	    WHERE events.user_id = ?
	        AND event_subjects.deleted_at IS NULL
	        AND events.deleted_at IS NULL
	        AND subjects.deleted_at IS NULL
	        AND users.deleted_at IS NULL
	        AND events.started_at < ?
	        AND COALESCE(events.ended_at, ?) > ?
	    GROUP BY
	        subjects.subject_id,
	        subjects.subject_name
	    HAVING seconds > 0
	    ORDER BY
	        seconds DESC,
	        subjects.subject_name,
	        subjects.subject_id
	`, ongoingUntil.Unix(), until.Unix(), from.Unix(), userID, until.Unix(), ongoingUntil.Unix(), from.Unix())
	if err != nil {
		return nil, fmt.Errorf("read subject time: %w", TranslateSQLiteError(err))
	}
	defer rows.Close()
	items := []SubjectTimeView{}
	for rows.Next() {
		var item SubjectTimeView
		if err := rows.Scan(&item.SubjectID, &item.SubjectName, &item.TimeSpentSeconds); err != nil {
			return nil, fmt.Errorf("scan subject time: %w", TranslateSQLiteError(err))
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read subject time rows: %w", TranslateSQLiteError(err))
	}
	return items, nil
}
