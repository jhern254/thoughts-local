package data

import (
	"context"
	"database/sql"
	"fmt"
)

// NumberedGoalSubject is a live membership with its stable Subject-local reference.
type NumberedGoalSubject struct {
	Subject    Subject
	GoalNumber int64
}

// reserveGoalNumber runs under the membership writer's transaction. Unlinking
// never releases a reservation, so a later link recovers the same reference.
func reserveGoalNumber(ctx context.Context, tx *sql.Tx, userID string, subjectID, goalID int64) error {
	_, err := tx.ExecContext(ctx, `
        INSERT INTO subject_goal_numbers (
            subject_id,
            goal_id,
            user_id,
            goal_number
        )
        SELECT ?, ?, ?, COALESCE(MAX(goal_number), 0) + 1
        FROM subject_goal_numbers
        WHERE subject_id = ?
        ON CONFLICT (subject_id, goal_id) DO NOTHING`, subjectID, goalID, userID, subjectID)
	return err
}

// ListLiveNumberedSubjectsForGoal preserves the live-membership visibility contract.
// Missing reservations are errors rather than being mistaken for Unassigned.
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
	// Keep memberships with missing reservations visible so scanning their NULL
	// number fails, rather than silently treating corrupt membership as Misc.
	rows, err := tx.QueryContext(ctx, `
        SELECT
            subject.subject_id,
            subject.user_id,
            subject.subject_name,
            subject.created_at,
            subject.updated_at,
            reservation.goal_number
        FROM subject_goals AS membership
        JOIN subjects AS subject
          ON subject.subject_id = membership.subject_id
         AND subject.user_id = membership.user_id
        LEFT JOIN subject_goal_numbers AS reservation
          ON reservation.subject_id = membership.subject_id
         AND reservation.goal_id = membership.goal_id
         AND reservation.user_id = membership.user_id
        WHERE membership.user_id = ?
          AND membership.goal_id = ?
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
