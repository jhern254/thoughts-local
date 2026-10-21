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
SELECT s.subject_id, s.subject_name,
 SUM(max(0, min(COALESCE(e.ended_at, ?), ?) - max(e.started_at, ?))) AS seconds
FROM events e
JOIN subjects s ON s.subject_id=e.subject_id AND s.user_id=e.user_id
JOIN users u ON u.user_id=e.user_id
WHERE e.user_id=? AND e.deleted_at IS NULL AND s.deleted_at IS NULL AND u.deleted_at IS NULL
 AND e.started_at < ? AND COALESCE(e.ended_at, ?) > ?
GROUP BY s.subject_id, s.subject_name
HAVING seconds > 0
ORDER BY seconds DESC, s.subject_name, s.subject_id`, ongoingUntil.Unix(), until.Unix(), from.Unix(), userID, until.Unix(), ongoingUntil.Unix(), from.Unix())
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
