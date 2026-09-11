package data

import (
	"context"
	"database/sql"
	"fmt"
)

// SubjectThoughtCount is a read result, not persisted subject state.
type SubjectThoughtCount struct {
	SubjectID int64
	Count     int64
}

type SQLiteMetricsStore struct{ db *sql.DB }

func NewSQLiteMetricsStore(db *sql.DB) *SQLiteMetricsStore { return &SQLiteMetricsStore{db: db} }

func (s *SQLiteMetricsStore) CountUnassignedThoughts(ctx context.Context, userID string) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM thoughts t JOIN users u ON u.user_id = t.user_id
		WHERE t.user_id = ? AND t.subject_id IS NULL AND t.deleted_at IS NULL AND u.deleted_at IS NULL`, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count unassigned thoughts: %w", TranslateSQLiteError(err))
	}
	return count, nil
}

func (s *SQLiteMetricsStore) ThoughtCountsBySubject(ctx context.Context, userID string) ([]SubjectThoughtCount, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.subject_id, COUNT(t.thought_id)
		FROM subjects s JOIN users u ON u.user_id = s.user_id
		LEFT JOIN thoughts t ON t.subject_id = s.subject_id AND t.user_id = s.user_id AND t.deleted_at IS NULL
		WHERE s.user_id = ? AND s.deleted_at IS NULL AND u.deleted_at IS NULL
		GROUP BY s.subject_id ORDER BY s.subject_id`, userID)
	if err != nil {
		return nil, fmt.Errorf("count thoughts by subject: %w", TranslateSQLiteError(err))
	}
	defer rows.Close()
	counts := []SubjectThoughtCount{}
	for rows.Next() {
		var count SubjectThoughtCount
		if err := rows.Scan(&count.SubjectID, &count.Count); err != nil {
			return nil, fmt.Errorf("scan thought count: %w", TranslateSQLiteError(err))
		}
		counts = append(counts, count)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read thought counts: %w", TranslateSQLiteError(err))
	}
	return counts, nil
}
