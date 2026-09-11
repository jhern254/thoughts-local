package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

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
		SELECT ?, ?, ?, ?, ?, ?, ?, ?
		WHERE EXISTS (SELECT 1 FROM users WHERE user_id = ? AND deleted_at IS NULL)
		  AND (? IS NULL OR EXISTS (SELECT 1 FROM subjects WHERE subject_id = ? AND user_id = ? AND deleted_at IS NULL))
		  AND (? IS NULL OR EXISTS (
			SELECT 1 FROM events e JOIN users u ON u.user_id = e.user_id
			WHERE e.event_id = ? AND e.deleted_at IS NULL AND u.deleted_at IS NULL))`,
		thought.UserID,
		thought.SubjectID,
		thought.EventID,
		thought.Thought,
		thought.Version,
		UnixSec(thought.ObservedAt),
		UnixSec(thought.CreatedAt),
		UnixSec(thought.UpdatedAt),
		thought.UserID,
		thought.SubjectID,
		thought.SubjectID,
		thought.UserID,
		thought.EventID,
		thought.EventID,
	)
	if err != nil {
		if thought.SubjectID != nil && isSQLiteForeignKeyConstraint(err) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("create thought: %w", TranslateSQLiteError(err))
	}

	created, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("get created thought count: %w", TranslateSQLiteError(err))
	}
	if created == 0 {
		return nil, ErrRecordNotFound
	}

	thoughtID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get created thought ID: %w", TranslateSQLiteError(err))
	}
	return s.GetThought(ctx, thought.UserID, thoughtID)
}

func (s *SQLiteThoughtStore) GetThought(ctx context.Context, userID string, thoughtID int64) (*Thought, error) {
	thought, err := scanThought(s.db.QueryRowContext(ctx, thoughtSelect+`
		WHERE user_id = ? AND thought_id = ? AND deleted_at IS NULL
		  AND EXISTS (SELECT 1 FROM users WHERE users.user_id = thoughts.user_id AND users.deleted_at IS NULL)`, userID, thoughtID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRecordNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get thought: %w", TranslateSQLiteError(err))
	}
	return thought, nil
}

const thoughtSelect = `SELECT
			thought_id,
			user_id,
			(SELECT subject_id FROM subjects WHERE subject_id = thoughts.subject_id AND deleted_at IS NULL),
			(SELECT e.event_id FROM events e JOIN users u ON u.user_id = e.user_id
			 WHERE e.event_id = thoughts.event_id AND e.deleted_at IS NULL AND u.deleted_at IS NULL),
			thought,
			version,
			observed_at,
			created_at,
			updated_at
		FROM thoughts `

func (s *SQLiteThoughtStore) ListThoughts(ctx context.Context, userID string, subjectID int64) ([]Thought, error) {
	rows, err := s.db.QueryContext(ctx, thoughtSelect+`
		WHERE user_id = ? AND subject_id = ? AND deleted_at IS NULL
		  AND EXISTS (
			SELECT 1 FROM subjects s JOIN users u ON u.user_id = s.user_id
			WHERE s.subject_id = thoughts.subject_id AND s.user_id = thoughts.user_id
			  AND s.deleted_at IS NULL AND u.deleted_at IS NULL)
		ORDER BY observed_at DESC, thought_id DESC`, userID, subjectID)
	if err != nil {
		return nil, fmt.Errorf("list thoughts: %w", TranslateSQLiteError(err))
	}
	return readThoughtRows(rows)
}

func (s *SQLiteThoughtStore) ListUnassignedThoughts(ctx context.Context, userID string) ([]Thought, error) {
	rows, err := s.db.QueryContext(ctx, thoughtSelect+`
		WHERE user_id = ? AND subject_id IS NULL AND deleted_at IS NULL
		  AND EXISTS (SELECT 1 FROM users WHERE users.user_id = thoughts.user_id AND users.deleted_at IS NULL)
		ORDER BY observed_at DESC, thought_id DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list unassigned thoughts: %w", TranslateSQLiteError(err))
	}
	return readThoughtRows(rows)
}

func readThoughtRows(rows *sql.Rows) ([]Thought, error) {
	defer rows.Close()
	thoughts := []Thought{}
	for rows.Next() {
		item, err := scanThought(rows)
		if err != nil {
			return nil, fmt.Errorf("scan thought: %w", TranslateSQLiteError(err))
		}
		thoughts = append(thoughts, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read thoughts: %w", TranslateSQLiteError(err))
	}
	return thoughts, nil
}

func scanThought(row interface{ Scan(...any) error }) (*Thought, error) {
	var thought Thought
	var subjectID, eventID sql.NullInt64
	var observedAt, createdAt, updatedAt int64
	err := row.Scan(
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
	if err != nil {
		return nil, err
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

// UpdateThought changes only text, preserving relationships and observed time.
func (s *SQLiteThoughtStore) UpdateThought(ctx context.Context, userID string, id int64, body string, version int64, updatedAt time.Time) (_ *Thought, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("update thought: %w", TranslateSQLiteError(err))
		}
	}()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE thoughts
		SET thought = ?, version = version + 1, updated_at = max(updated_at, ?)
		WHERE user_id = ? AND thought_id = ? AND version = ? AND deleted_at IS NULL
		  AND EXISTS (SELECT 1 FROM users WHERE users.user_id = thoughts.user_id AND users.deleted_at IS NULL)`,
		body, updatedAt.Unix(), userID, id, version)
	if err != nil {
		return nil, err
	}
	if err := checkThoughtMutation(ctx, tx, result, userID, id); err != nil {
		return nil, err
	}
	item, err := scanThought(tx.QueryRowContext(ctx, thoughtSelect+` WHERE user_id = ? AND thought_id = ?`, userID, id))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return item, nil
}

// DeleteThought retains the row and its history, rejecting stale callers.
func (s *SQLiteThoughtStore) DeleteThought(ctx context.Context, userID string, id, version int64) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("delete thought: %w", TranslateSQLiteError(err))
		}
	}()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE thoughts
		SET deleted_at = max(unixepoch('now'), updated_at),
		    updated_at = max(unixepoch('now'), updated_at), version = version + 1
		WHERE user_id = ? AND thought_id = ? AND version = ? AND deleted_at IS NULL
		  AND EXISTS (SELECT 1 FROM users WHERE users.user_id = thoughts.user_id AND users.deleted_at IS NULL)`, userID, id, version)
	if err != nil {
		return err
	}
	if err := checkThoughtMutation(ctx, tx, result, userID, id); err != nil {
		return err
	}
	return tx.Commit()
}

// Check a missed conditional write in the same transaction so a concurrent
// mutation cannot change whether the caller sees not-found or a stale version.
func checkThoughtMutation(ctx context.Context, tx *sql.Tx, result sql.Result, userID string, id int64) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 0 {
		return nil
	}
	var active bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM thoughts
		WHERE user_id = ? AND thought_id = ? AND deleted_at IS NULL
		  AND EXISTS (SELECT 1 FROM users WHERE users.user_id = thoughts.user_id AND users.deleted_at IS NULL))`, userID, id).Scan(&active)
	if err != nil {
		return err
	}
	if !active {
		return ErrRecordNotFound
	}
	return ErrVersionConflict
}

var _ ThoughtStore = (*SQLiteThoughtStore)(nil)
