package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// NumberedGoalSubject is a live membership with its stable Subject-local reference.
type NumberedGoalSubject struct {
	Subject    Subject
	GoalNumber int64
}

// activateGoalSubject requires validated live endpoints and the transaction's
// write lock. Reactivation preserves the original number, even at the integer
// ceiling; new numbers include every retained relationship in their maximum.
func activateGoalSubject(ctx context.Context, tx *sql.Tx, userID string, subjectID, goalID int64) error {
	result, err := tx.ExecContext(ctx, `
        UPDATE subject_goals
        SET deleted_at = NULL
        WHERE user_id = ?
          AND subject_id = ?
          AND goal_id = ?
          AND deleted_at IS NOT NULL`, userID, subjectID, goalID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 1 {
		return nil
	}

	// An already active pair reuses its candidate number so SQLite reports the
	// duplicate key even when no larger integer number can be allocated.
	_, err = tx.ExecContext(ctx, `
        INSERT INTO subject_goals (subject_id, goal_id, user_id, goal_number)
        SELECT ?, ?, ?, COALESCE(
            (SELECT goal_number
             FROM subject_goals
             WHERE subject_id = ? AND goal_id = ?),
            (SELECT COALESCE(MAX(goal_number), 0) + 1
             FROM subject_goals
             WHERE subject_id = ?)
        )`, subjectID, goalID, userID, subjectID, goalID, subjectID)
	var cause *sqlite.Error
	if errors.As(err, &cause) && cause.Code() == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY {
		return &databaseError{cause: err, kind: ErrDuplicateRecord}
	}
	return err
}

// ListLiveNumberedSubjectsForGoal preserves the live-membership visibility contract.
func (s *SQLiteSubjectGoalStore) ListLiveNumberedSubjectsForGoal(ctx context.Context, userID string, goalID int64) (_ []NumberedGoalSubject, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("list numbered goal subjects: %w", TranslateSQLiteError(err))
		}
	}()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var exists bool
	if err = tx.QueryRowContext(ctx, `
        SELECT EXISTS (
            SELECT 1
            FROM goals
            WHERE user_id = ?
              AND goal_id = ?
              AND `+visibleGoals+`
        )`, userID, goalID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrRecordNotFound
	}
	rows, err := tx.QueryContext(ctx, `
        SELECT
            subject.subject_id,
            subject.user_id,
            subject.subject_name,
            subject.created_at,
            subject.updated_at,
            membership.goal_number
        FROM subject_goals AS membership
        JOIN subjects AS subject
          ON subject.subject_id = membership.subject_id
         AND subject.user_id = membership.user_id
        WHERE membership.user_id = ?
          AND membership.goal_id = ?
          AND membership.deleted_at IS NULL
          AND subject.deleted_at IS NULL
        ORDER BY subject.subject_id`, userID, goalID)
	if err != nil {
		return nil, err
	}
	refs := make([]NumberedGoalSubject, 0)
	for rows.Next() {
		var ref NumberedGoalSubject
		var created, updated int64
		if err = rows.Scan(&ref.Subject.SubjectID, &ref.Subject.UserID, &ref.Subject.SubjectName, &created, &updated, &ref.GoalNumber); err != nil {
			rows.Close()
			return nil, err
		}
		ref.Subject.CreatedAt = TimeFromUnixSec(created)
		ref.Subject.UpdatedAt = TimeFromUnixSec(updated)
		refs = append(refs, ref)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return refs, nil
}
