package data

import (
	"context"
	"fmt"
)

// CreateSubjectFromAlias removes precisely the reviewed alias and claims its
// name in the same transaction. A failed insert must not leave the alias removed.
// Related records stay with the original Subject; none are reassigned here.
func (s *SQLiteSubjectStore) CreateSubjectFromAlias(ctx context.Context, subject *Subject, subjectID, aliasID int64) (_ *Subject, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("create subject from alias: %w", subjectNameError(err))
		}
	}()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// Match identity AND exact spelling so a removed/recreated or renamed alias
	// cannot be consumed using an old confirmation. The write also serializes
	// competing namespace changes before the new canonical name is inserted.
	result, err := tx.ExecContext(ctx, `UPDATE subject_aliases
 SET deleted_at=max(unixepoch('now'),updated_at), updated_at=max(unixepoch('now'),updated_at)
 WHERE alias_id=? AND subject_id=? AND user_id=? AND alias_name=? AND deleted_at IS NULL
 AND EXISTS (SELECT 1 FROM subjects s JOIN users u ON u.user_id=s.user_id
 WHERE s.subject_id=subject_aliases.subject_id AND s.user_id=subject_aliases.user_id
 AND s.deleted_at IS NULL AND u.deleted_at IS NULL)`, aliasID, subjectID, subject.UserID, subject.SubjectName)
	if err != nil {
		return nil, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, ErrRecordNotFound
	}
	created, err := scanSubject(tx.QueryRowContext(ctx, `INSERT INTO subjects(user_id,subject_name,created_at,updated_at)
 VALUES(?,?,?,?) RETURNING subject_id,user_id,subject_name,created_at,updated_at`, subject.UserID, subject.SubjectName, UnixSec(subject.CreatedAt), UnixSec(subject.UpdatedAt)))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}
