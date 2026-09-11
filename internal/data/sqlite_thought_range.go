package data

import (
	"context"
	"fmt"
	"time"
)

// ListThoughtsInRange returns active thoughts observed in [from, until), across
// all subjects, ordered chronologically. Equal bounds produce an empty result.
// Bounds are explicit instants at the database's whole-second precision.
func (s *SQLiteThoughtStore) ListThoughtsInRange(ctx context.Context, userID string, from, until time.Time) ([]Thought, error) {
	rows, err := s.db.QueryContext(ctx, thoughtSelect+`
		WHERE user_id = ? AND observed_at >= ? AND observed_at < ? AND deleted_at IS NULL
		  AND EXISTS (SELECT 1 FROM users WHERE users.user_id = thoughts.user_id AND users.deleted_at IS NULL)
		ORDER BY observed_at, thought_id`, userID, from.Unix(), until.Unix())
	if err != nil {
		return nil, fmt.Errorf("list thoughts in range: %w", TranslateSQLiteError(err))
	}
	return readThoughtRows(rows)
}
