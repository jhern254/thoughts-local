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
		SELECT ?, ?, ?, ?
		WHERE EXISTS (SELECT 1 FROM users WHERE user_id = ? AND deleted_at IS NULL)`,
		subject.UserID,
		subject.SubjectName,
		UnixSec(subject.CreatedAt),
		UnixSec(subject.UpdatedAt),
		subject.UserID,
	)
	if err != nil {
		if isSQLiteUniqueConstraint(err) {
			return nil, ErrDuplicateRecord
		}
		return nil, fmt.Errorf("create subject: %w", TranslateSQLiteError(err))
	}

	created, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("get created subject count: %w", TranslateSQLiteError(err))
	}
	if created == 0 {
		return nil, ErrRecordNotFound
	}

	subjectID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get created subject ID: %w", TranslateSQLiteError(err))
	}
	return s.GetSubject(ctx, subject.UserID, subjectID)
}

func (s *SQLiteSubjectStore) GetSubject(ctx context.Context, userID string, subjectID int64) (*Subject, error) {
	subject, err := scanSubject(s.db.QueryRowContext(ctx, `
		SELECT subject_id, user_id, subject_name, created_at, updated_at
		FROM subjects
		WHERE user_id = ? AND subject_id = ? AND deleted_at IS NULL
		  AND EXISTS (SELECT 1 FROM users WHERE users.user_id = subjects.user_id AND users.deleted_at IS NULL)`,
		userID,
		subjectID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRecordNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get subject: %w", TranslateSQLiteError(err))
	}

	return subject, nil
}

func (s *SQLiteSubjectStore) ListSubjects(ctx context.Context, userID string) ([]Subject, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT subject_id, user_id, subject_name, created_at, updated_at
		FROM subjects
		WHERE user_id = ? AND deleted_at IS NULL
		  AND EXISTS (SELECT 1 FROM users WHERE users.user_id = subjects.user_id AND users.deleted_at IS NULL)
		ORDER BY subject_id`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list subjects: %w", TranslateSQLiteError(err))
	}
	defer rows.Close()

	subjects := make([]Subject, 0)
	for rows.Next() {
		subject, err := scanSubject(rows)
		if err != nil {
			return nil, fmt.Errorf("list subjects: %w", TranslateSQLiteError(err))
		}
		subjects = append(subjects, *subject)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list subjects: %w", TranslateSQLiteError(err))
	}
	return subjects, nil
}

func (s *SQLiteSubjectStore) UpdateSubject(ctx context.Context, userID string, subjectID int64, name string, updatedAt time.Time) (*Subject, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE subjects
		SET subject_name = ?, updated_at = ?
		WHERE user_id = ? AND subject_id = ? AND deleted_at IS NULL
		  AND EXISTS (SELECT 1 FROM users WHERE users.user_id = subjects.user_id AND users.deleted_at IS NULL)`,
		name,
		UnixSec(updatedAt),
		userID,
		subjectID,
	)
	if err != nil {
		if isSQLiteUniqueConstraint(err) {
			return nil, ErrDuplicateRecord
		}
		return nil, fmt.Errorf("update subject: %w", TranslateSQLiteError(err))
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("get updated subject count: %w", TranslateSQLiteError(err))
	}
	if updated == 0 {
		return nil, ErrRecordNotFound
	}
	return s.GetSubject(ctx, userID, subjectID)
}

func (s *SQLiteSubjectStore) DeleteSubject(ctx context.Context, userID string, subjectID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin subject deletion: %w", TranslateSQLiteError(err))
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		UPDATE subjects
		SET deleted_at = max(unixepoch('now'), updated_at),
		    updated_at = max(unixepoch('now'), updated_at)
		WHERE user_id = ? AND subject_id = ? AND deleted_at IS NULL
		  AND EXISTS (SELECT 1 FROM users WHERE users.user_id = subjects.user_id AND users.deleted_at IS NULL)`,
		userID,
		subjectID,
	)
	if err != nil {
		return fmt.Errorf("delete subject: %w", TranslateSQLiteError(err))
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get deleted subject count: %w", TranslateSQLiteError(err))
	}
	if deleted == 0 {
		return ErrRecordNotFound
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE thoughts
		SET subject_id = NULL, version = version + 1,
		    updated_at = max(unixepoch('now'), updated_at)
		WHERE user_id = ? AND subject_id = ?`, userID, subjectID)
	if err != nil {
		return fmt.Errorf("unlink subject thoughts: %w", TranslateSQLiteError(err))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit subject deletion: %w", TranslateSQLiteError(err))
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
