package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// SQLiteThoughtStore persists thoughts in SQLite.
type SQLiteThoughtStore struct {
	db *sql.DB
}

// NewSQLiteThoughtStore returns a thought store backed by db.
func NewSQLiteThoughtStore(db *sql.DB) *SQLiteThoughtStore {
	return &SQLiteThoughtStore{db: db}
}

func (s *SQLiteThoughtStore) CreateThought(ctx context.Context, thought *Thought) (*Thought, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO thoughts (
			user_id,
			subject_id,
			event_id,
			thought,
			version,
			observed_at,
			created_at,
			updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		thought.UserID,
		thought.SubjectID,
		thought.EventID,
		thought.Thought,
		thought.Version,
		UnixSec(thought.ObservedAt),
		UnixSec(thought.CreatedAt),
		UnixSec(thought.UpdatedAt),
	)
	if err != nil {
		if thought.SubjectID != nil && isSQLiteForeignKeyConstraint(err) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("create thought: %w", err)
	}

	thoughtID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get created thought ID: %w", err)
	}
	return s.GetThought(ctx, thought.UserID, thoughtID)
}

func (s *SQLiteThoughtStore) GetThought(ctx context.Context, userID string, thoughtID int64) (*Thought, error) {
	var thought Thought
	var subjectID, eventID sql.NullInt64
	var observedAt, createdAt, updatedAt int64
	err := s.db.QueryRowContext(ctx, `
		SELECT
			thought_id,
			user_id,
			subject_id,
			event_id,
			thought,
			version,
			observed_at,
			created_at,
			updated_at
		FROM thoughts
		WHERE user_id = ? AND thought_id = ?`,
		userID,
		thoughtID,
	).Scan(
		&thought.ThoughtID,
		&thought.UserID,
		&subjectID,
		&eventID,
		&thought.Thought,
		&thought.Version,
		&observedAt,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRecordNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get thought: %w", err)
	}

	thought.SubjectID = int64Pointer(subjectID)
	thought.EventID = int64Pointer(eventID)
	thought.ObservedAt = TimeFromUnixSec(observedAt)
	thought.CreatedAt = TimeFromUnixSec(createdAt)
	thought.UpdatedAt = TimeFromUnixSec(updatedAt)
	return &thought, nil
}

func int64Pointer(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}

func isSQLiteForeignKeyConstraint(err error) bool {
	var sqliteErr *sqlite.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY
}

var _ ThoughtStore = (*SQLiteThoughtStore)(nil)
