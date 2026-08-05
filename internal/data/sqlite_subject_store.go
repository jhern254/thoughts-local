package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// SQLiteSubjectStore persists subjects in SQLite.
type SQLiteSubjectStore struct {
	db *sql.DB
}

func NewSQLiteSubjectStore(db *sql.DB) *SQLiteSubjectStore {
	return &SQLiteSubjectStore{db: db}
}

func (s *SQLiteSubjectStore) GetSubject(ctx context.Context, userID, name string) (*Subject, error) {
	var subject Subject
	var createdAt, updatedAt int64
	err := s.db.QueryRowContext(ctx, `
		SELECT subject_id, user_id, subject_name, created_at, updated_at
		FROM subjects
		WHERE user_id = ? AND subject_name = ?`, userID, name,
	).Scan(&subject.SubjectID, &subject.UserID, &subject.SubjectName, &createdAt, &updatedAt)
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

func (s *SQLiteSubjectStore) CaptureSubject(ctx context.Context, userID string, subject *Subject) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO subjects (user_id, subject_name, created_at, updated_at)
		VALUES (?, ?, ?, ?)`,
		userID,
		subject.SubjectName,
		UnixSec(subject.CreatedAt),
		UnixSec(subject.UpdatedAt),
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return 0, ErrDuplicateRecord
		}
		return 0, err
	}
	return result.LastInsertId()
}

func isUniqueConstraint(err error) bool {
	var sqliteErr *sqlite.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}
