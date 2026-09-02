package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// SQLiteStore persists application data in SQLite.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore returns a store backed by db.
func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

func (s *SQLiteStore) CreateSubject(ctx context.Context, subject *Subject) (*Subject, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO subjects (user_id, subject_name, created_at, updated_at)
		VALUES (?, ?, ?, ?)`,
		subject.UserID,
		subject.SubjectName,
		UnixSec(subject.CreatedAt),
		UnixSec(subject.UpdatedAt),
	)
	if err != nil {
		if isSQLiteUniqueConstraint(err) {
			return nil, ErrDuplicateRecord
		}
		return nil, fmt.Errorf("create subject: %w", err)
	}

	subjectID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get created subject ID: %w", err)
	}
	return s.GetSubject(ctx, subject.UserID, subjectID)
}

func (s *SQLiteStore) GetSubject(ctx context.Context, userID string, subjectID int64) (*Subject, error) {
	var subject Subject
	var createdAt, updatedAt int64
	err := s.db.QueryRowContext(ctx, `
		SELECT subject_id, user_id, subject_name, created_at, updated_at
		FROM subjects
		WHERE user_id = ? AND subject_id = ?`,
		userID,
		subjectID,
	).Scan(
		&subject.SubjectID,
		&subject.UserID,
		&subject.SubjectName,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRecordNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get subject: %w", err)
	}

	subject.CreatedAt = TimeFromUnixSec(createdAt)
	subject.UpdatedAt = TimeFromUnixSec(updatedAt)
	return &subject, nil
}

func isSQLiteUniqueConstraint(err error) bool {
	var sqliteErr *sqlite.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}

var _ SubjectStore = (*SQLiteStore)(nil)
