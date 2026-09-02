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

// SQLiteSubjectStore persists subjects in SQLite.
type SQLiteSubjectStore struct {
	db *sql.DB
}

// NewSQLiteSubjectStore returns a subject store backed by db.
func NewSQLiteSubjectStore(db *sql.DB) *SQLiteSubjectStore {
	return &SQLiteSubjectStore{db: db}
}

func (s *SQLiteSubjectStore) CreateSubject(ctx context.Context, subject *Subject) (*Subject, error) {
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

func (s *SQLiteSubjectStore) GetSubject(ctx context.Context, userID string, subjectID int64) (*Subject, error) {
	subject, err := scanSubject(s.db.QueryRowContext(ctx, `
		SELECT subject_id, user_id, subject_name, created_at, updated_at
		FROM subjects
		WHERE user_id = ? AND subject_id = ?`,
		userID,
		subjectID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRecordNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get subject: %w", err)
	}

	return subject, nil
}

func (s *SQLiteSubjectStore) ListSubjects(ctx context.Context, userID string) ([]Subject, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT subject_id, user_id, subject_name, created_at, updated_at
		FROM subjects
		WHERE user_id = ?
		ORDER BY subject_id`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list subjects: %w", err)
	}
	defer rows.Close()

	subjects := make([]Subject, 0)
	for rows.Next() {
		subject, err := scanSubject(rows)
		if err != nil {
			return nil, fmt.Errorf("list subjects: %w", err)
		}
		subjects = append(subjects, *subject)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list subjects: %w", err)
	}
	return subjects, nil
}

func (s *SQLiteSubjectStore) UpdateSubject(ctx context.Context, userID string, subjectID int64, name string, updatedAt time.Time) (*Subject, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE subjects
		SET subject_name = ?, updated_at = ?
		WHERE user_id = ? AND subject_id = ?`,
		name,
		UnixSec(updatedAt),
		userID,
		subjectID,
	)
	if err != nil {
		if isSQLiteUniqueConstraint(err) {
			return nil, ErrDuplicateRecord
		}
		return nil, fmt.Errorf("update subject: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("get updated subject count: %w", err)
	}
	if updated == 0 {
		return nil, ErrRecordNotFound
	}
	return s.GetSubject(ctx, userID, subjectID)
}

func (s *SQLiteSubjectStore) DeleteSubject(ctx context.Context, userID string, subjectID int64) error {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM subjects
		WHERE user_id = ? AND subject_id = ?`,
		userID,
		subjectID,
	)
	if err != nil {
		return fmt.Errorf("delete subject: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get deleted subject count: %w", err)
	}
	if deleted == 0 {
		return ErrRecordNotFound
	}
	return nil
}

type subjectScanner interface {
	Scan(...any) error
}

func scanSubject(scanner subjectScanner) (*Subject, error) {
	var subject Subject
	var createdAt, updatedAt int64
	if err := scanner.Scan(
		&subject.SubjectID,
		&subject.UserID,
		&subject.SubjectName,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, err
	}
	subject.CreatedAt = TimeFromUnixSec(createdAt)
	subject.UpdatedAt = TimeFromUnixSec(updatedAt)
	return &subject, nil
}

func isSQLiteUniqueConstraint(err error) bool {
	var sqliteErr *sqlite.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}

var _ SubjectStore = (*SQLiteSubjectStore)(nil)
