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

const subjectAliasColumns = `alias_id, subject_id, user_id, alias_name, created_at, updated_at`

func (s *SQLiteSubjectStore) CreateSubjectAlias(ctx context.Context, alias *SubjectAlias) (*SubjectAlias, error) {
	row := s.db.QueryRowContext(ctx, `INSERT INTO subject_aliases(subject_id,user_id,alias_name,created_at,updated_at)
 SELECT ?,?,?,?,? WHERE EXISTS (
 SELECT 1 FROM subjects s JOIN users u ON u.user_id=s.user_id
 WHERE s.subject_id=? AND s.user_id=? AND s.deleted_at IS NULL AND u.deleted_at IS NULL)
 RETURNING `+subjectAliasColumns,
		alias.SubjectID, alias.UserID, alias.AliasName, UnixSec(alias.CreatedAt), UnixSec(alias.UpdatedAt), alias.SubjectID, alias.UserID)
	created, err := scanSubjectAlias(row)
	if err != nil {
		return nil, fmt.Errorf("create subject alias: %w", subjectNameError(err))
	}
	return created, nil
}

func (s *SQLiteSubjectStore) UpdateSubjectAlias(ctx context.Context, userID string, subjectID, aliasID int64, name string, updatedAt time.Time) (*SubjectAlias, error) {
	row := s.db.QueryRowContext(ctx, `UPDATE subject_aliases
 SET alias_name=?, updated_at=max(updated_at,?)
 WHERE alias_id=? AND user_id=? AND subject_id=? AND deleted_at IS NULL
 AND EXISTS (SELECT 1 FROM subjects s JOIN users u ON u.user_id=s.user_id
 WHERE s.subject_id=subject_aliases.subject_id AND s.user_id=subject_aliases.user_id
 AND s.deleted_at IS NULL AND u.deleted_at IS NULL)
 RETURNING `+subjectAliasColumns, name, UnixSec(updatedAt), aliasID, userID, subjectID)
	updated, err := scanSubjectAlias(row)
	if err != nil {
		return nil, fmt.Errorf("update subject alias: %w", subjectNameError(err))
	}
	return updated, nil
}

func (s *SQLiteSubjectStore) RemoveSubjectAlias(ctx context.Context, userID string, subjectID, aliasID int64) error {
	result, err := s.db.ExecContext(ctx, `UPDATE subject_aliases
 SET deleted_at=max(unixepoch('now'),updated_at), updated_at=max(unixepoch('now'),updated_at)
 WHERE alias_id=? AND user_id=? AND subject_id=? AND deleted_at IS NULL
 AND EXISTS (SELECT 1 FROM subjects s JOIN users u ON u.user_id=s.user_id
 WHERE s.subject_id=subject_aliases.subject_id AND s.user_id=subject_aliases.user_id
 AND s.deleted_at IS NULL AND u.deleted_at IS NULL)`, aliasID, userID, subjectID)
	if err != nil {
		return fmt.Errorf("remove subject alias: %w", TranslateSQLiteError(err))
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("removed subject alias count: %w", TranslateSQLiteError(err))
	}
	if count == 0 {
		return ErrRecordNotFound
	}
	return nil
}

func (s *SQLiteSubjectStore) ListSubjectAliases(ctx context.Context, userID string, subjectID int64) ([]SubjectAlias, error) {
	if _, err := s.GetSubject(ctx, userID, subjectID); err != nil {
		return nil, err
	}
	return s.listSubjectAliases(ctx, `SELECT a.alias_id,a.subject_id,a.user_id,a.alias_name,a.created_at,a.updated_at
 FROM subject_aliases a JOIN subjects s ON s.subject_id=a.subject_id AND s.user_id=a.user_id
 JOIN users u ON u.user_id=a.user_id
 WHERE a.user_id=? AND a.subject_id=? AND a.deleted_at IS NULL AND s.deleted_at IS NULL
 AND u.deleted_at IS NULL ORDER BY a.alias_id`, userID, subjectID)
}

func (s *SQLiteSubjectStore) ListUserSubjectAliases(ctx context.Context, userID string) ([]SubjectAlias, error) {
	return s.listSubjectAliases(ctx, `SELECT a.alias_id,a.subject_id,a.user_id,a.alias_name,a.created_at,a.updated_at
 FROM subject_aliases a JOIN subjects s ON s.subject_id=a.subject_id AND s.user_id=a.user_id
 JOIN users u ON u.user_id=a.user_id
 WHERE a.user_id=? AND a.deleted_at IS NULL AND s.deleted_at IS NULL AND u.deleted_at IS NULL
 ORDER BY a.alias_name`, userID)
}

func (s *SQLiteSubjectStore) listSubjectAliases(ctx context.Context, query string, args ...any) ([]SubjectAlias, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list subject aliases: %w", TranslateSQLiteError(err))
	}
	defer rows.Close()
	aliases := make([]SubjectAlias, 0)
	for rows.Next() {
		alias, err := scanSubjectAlias(rows)
		if err != nil {
			return nil, fmt.Errorf("read subject alias: %w", TranslateSQLiteError(err))
		}
		aliases = append(aliases, *alias)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list subject aliases: %w", TranslateSQLiteError(err))
	}
	return aliases, nil
}

func (s *SQLiteSubjectStore) ResolveSubjectName(ctx context.Context, userID, name string) (*Subject, error) {
	// Each branch uses its active-name index. The migration guards ensure the
	// UNION ALL can return at most one row, even across concurrent name writes.
	row := s.db.QueryRowContext(ctx, `SELECT s.subject_id,s.user_id,s.subject_name,s.created_at,s.updated_at
 FROM subjects s JOIN users u ON u.user_id=s.user_id
 WHERE s.user_id=? AND s.subject_name=? AND s.deleted_at IS NULL AND u.deleted_at IS NULL
 UNION ALL
 SELECT s.subject_id,s.user_id,s.subject_name,s.created_at,s.updated_at
 FROM subject_aliases a JOIN subjects s ON s.subject_id=a.subject_id AND s.user_id=a.user_id
 JOIN users u ON u.user_id=a.user_id
 WHERE a.user_id=? AND a.alias_name=? AND a.deleted_at IS NULL AND s.deleted_at IS NULL
 AND u.deleted_at IS NULL`, userID, name, userID, name)
	subject, err := scanSubject(row)
	if err != nil {
		return nil, fmt.Errorf("resolve subject name: %w", subjectNameError(err))
	}
	return subject, nil
}

func scanSubjectAlias(row subjectScanner) (*SubjectAlias, error) {
	var alias SubjectAlias
	var createdAt, updatedAt int64
	if err := row.Scan(&alias.AliasID, &alias.SubjectID, &alias.UserID, &alias.AliasName, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	alias.CreatedAt = TimeFromUnixSec(createdAt)
	alias.UpdatedAt = TimeFromUnixSec(updatedAt)
	return &alias, nil
}

func subjectNameError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrRecordNotFound
	}
	var cause *sqlite.Error
	// Only the named namespace guard is a duplicate. Other trigger failures
	// (including injected write failures) must retain their original meaning.
	nameConflict := isSQLiteUniqueConstraint(err)
	if errors.As(err, &cause) && cause.Code() == sqlite3.SQLITE_CONSTRAINT_TRIGGER {
		// modernc exposes the trigger message through its formatted Error string.
		// Match the entire fixed diagnostic, never a phrase in an unrelated error.
		nameConflict = cause.Error() == fmt.Sprintf("constraint failed: subject name conflict (%d)", sqlite3.SQLITE_CONSTRAINT_TRIGGER)
	}
	if nameConflict {
		return &databaseError{cause: err, kind: ErrDuplicateRecord}
	}
	return TranslateSQLiteError(err)
}
