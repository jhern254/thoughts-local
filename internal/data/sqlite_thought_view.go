package data

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
)

const thoughtSummarySelect = `SELECT t.thought_id, substr(t.thought, 1, 81), s.subject_name,
	t.observed_at, t.created_at
	FROM thoughts t
	LEFT JOIN subjects s ON s.subject_id = t.subject_id AND s.user_id = t.user_id AND s.deleted_at IS NULL
	WHERE t.user_id = ? AND t.deleted_at IS NULL
	AND EXISTS (SELECT 1 FROM users u WHERE u.user_id = t.user_id AND u.deleted_at IS NULL)`

func (s *SQLiteThoughtStore) BrowseThoughtsView(ctx context.Context, userID string, request ThoughtViewRequest) (ThoughtView, error) {
	query := thoughtSummarySelect
	args := []any{userID}
	comparison, order := "<", " DESC"
	if request.Direction == ThoughtsNewer {
		comparison, order = ">", " ASC"
	}
	if cursor := request.Cursor; cursor != nil {
		query += " AND (t.observed_at, t.created_at, t.thought_id) " + comparison + " (?, ?, ?)"
		args = append(args, cursor.ObservedAt.Unix(), cursor.CreatedAt.Unix(), cursor.ThoughtID)
	}
	query += " ORDER BY t.observed_at" + order + ", t.created_at" + order + ", t.thought_id" + order + " LIMIT ?"
	args = append(args, ThoughtViewBatchSize+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return ThoughtView{}, fmt.Errorf("browse thoughts: %w", TranslateSQLiteError(err))
	}
	defer rows.Close()
	view := ThoughtView{Items: make([]ThoughtSummary, 0, ThoughtViewBatchSize+1)}
	for rows.Next() {
		var item ThoughtSummary
		var subject sql.NullString
		var observed, created int64
		if err := rows.Scan(&item.ThoughtID, &item.Preview, &subject, &observed, &created); err != nil {
			return ThoughtView{}, fmt.Errorf("scan thought summary: %w", TranslateSQLiteError(err))
		}
		if subject.Valid {
			item.SubjectName = &subject.String
		}
		preview := []rune(item.Preview)
		if len(preview) > 80 {
			item.Preview = string(preview[:80]) + "…"
		}
		item.ObservedAt, item.CreatedAt = TimeFromUnixSec(observed), TimeFromUnixSec(created)
		view.Items = append(view.Items, item)
	}
	if err := rows.Err(); err != nil {
		return ThoughtView{}, fmt.Errorf("read thought summaries: %w", TranslateSQLiteError(err))
	}
	view.More = len(view.Items) > ThoughtViewBatchSize
	if view.More {
		view.Items = view.Items[:ThoughtViewBatchSize]
	}
	if request.Direction == ThoughtsNewer {
		slices.Reverse(view.Items)
	}
	return view, nil
}
