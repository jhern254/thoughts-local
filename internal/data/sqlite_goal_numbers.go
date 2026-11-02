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
	_, err := tx.ExecContext(ctx, `INSERT INTO subject_goal_numbers(subject_id,goal_id,user_id,goal_number)
 SELECT ?,?,?,coalesce(max(goal_number),0)+1 FROM subject_goal_numbers WHERE subject_id=?
 ON CONFLICT(subject_id,goal_id) DO NOTHING`, subjectID, goalID, userID, subjectID)
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
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM goals WHERE user_id=? AND goal_id=? AND `+visibleGoals+`)`, userID, goalID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrRecordNotFound
	}
	rows, err := tx.QueryContext(ctx, `SELECT s.subject_id,s.user_id,s.subject_name,s.created_at,s.updated_at,n.goal_number
 FROM subject_goals sg JOIN subjects s ON s.subject_id=sg.subject_id AND s.user_id=sg.user_id
 LEFT JOIN subject_goal_numbers n ON n.subject_id=sg.subject_id AND n.goal_id=sg.goal_id AND n.user_id=sg.user_id
 WHERE sg.user_id=? AND sg.goal_id=? AND s.deleted_at IS NULL ORDER BY s.subject_id`, userID, goalID)
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
