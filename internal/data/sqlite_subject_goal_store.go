package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

type SQLiteSubjectGoalStore struct{ db *sql.DB }

func NewSQLiteSubjectGoalStore(db *sql.DB) *SQLiteSubjectGoalStore {
	return &SQLiteSubjectGoalStore{db: db}
}

// AddSubjectGoal links undeleted same-owner endpoints, including inactive goals.
// A successful link change advances the Goal version once in the same transaction.
func (s *SQLiteSubjectGoalStore) AddSubjectGoal(ctx context.Context, userID string, subjectID, goalID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin subject goal mutation: %w", TranslateSQLiteError(err))
	}
	defer tx.Rollback()

	// Acquire the write lock before validating or allocating a Subject-local number.
	result, err := tx.ExecContext(ctx, `
        UPDATE goals
        SET version = version
        WHERE user_id = ?
            AND goal_id = ?
            AND `+visibleGoals, userID, goalID)
	if err != nil {
		return fmt.Errorf("lock goal for association: %w", TranslateSQLiteError(err))
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrRecordNotFound
	}
	if err := validateGoalSubjects(ctx, tx, userID, []int64{subjectID}); err != nil {
		return fmt.Errorf("validate goal subject: %w", TranslateSQLiteError(err))
	}
	if err := activateGoalSubject(ctx, tx, userID, subjectID, goalID); err != nil {
		return fmt.Errorf("add subject goal: %w", TranslateSQLiteError(err))
	}
	if err := advanceGoalAssociationVersion(ctx, tx, userID, goalID); err != nil {
		return fmt.Errorf("update goal association version: %w", TranslateSQLiteError(err))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit subject goal mutation: %w", TranslateSQLiteError(err))
	}
	return nil
}

// RemoveSubjectGoal retires a live relationship and advances the Goal version
// once. Deleted endpoints and already retired relationships remain unchanged.
func (s *SQLiteSubjectGoalStore) RemoveSubjectGoal(ctx context.Context, userID string, subjectID, goalID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin subject goal mutation: %w", TranslateSQLiteError(err))
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
	    UPDATE subject_goals
	    SET deleted_at = unixepoch('now')
	    WHERE user_id = ?
	        AND subject_id = ?
	        AND goal_id = ?
	        AND deleted_at IS NULL
	        AND EXISTS (
	            SELECT
	                1
	            FROM users
	            WHERE user_id = ?
	                AND deleted_at IS NULL
	        )
	        AND EXISTS (
	            SELECT
	                1
	            FROM subjects
	            WHERE subject_id = ?
	                AND user_id = ?
	                AND deleted_at IS NULL
	        )
	        AND EXISTS (
	            SELECT
	                1
	            FROM goals
	            WHERE goal_id = ?
	                AND user_id = ?
	                AND deleted_at IS NULL
	        )
	`, userID, subjectID, goalID, userID, subjectID, userID, goalID, userID)
	if err != nil {
		return fmt.Errorf("remove subject goal: %w", TranslateSQLiteError(err))
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("remove subject goal: %w", TranslateSQLiteError(err))
	}
	if count == 0 {
		return ErrRecordNotFound
	}
	if err := advanceGoalAssociationVersion(ctx, tx, userID, goalID); err != nil {
		return fmt.Errorf("update goal association version: %w", TranslateSQLiteError(err))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit subject goal mutation: %w", TranslateSQLiteError(err))
	}
	return nil
}

// ListSubjectGoals returns retained links in goal ID order, including deleted endpoints.
// Inactive goals remain linked; unavailable owners and empty selections return an empty slice.
func (s *SQLiteSubjectGoalStore) ListSubjectGoals(ctx context.Context, userID string, subjectID int64) ([]SubjectGoal, error) {
	return s.listSubjectGoals(ctx, userID, subjectID, `subject_id = ? ORDER BY goal_id`)
}

// ListGoalSubjects returns retained links in subject ID order with the same visibility as ListSubjectGoals.
func (s *SQLiteSubjectGoalStore) ListGoalSubjects(ctx context.Context, userID string, goalID int64) ([]SubjectGoal, error) {
	return s.listSubjectGoals(ctx, userID, goalID, `goal_id = ? ORDER BY subject_id`)
}

// ListActiveGoalsForSubject returns current active goals through an undeleted,
// same-owner subject, ordered by priority then ID. Schedule dates are not eligibility filters.
// Retained-link listing remains available through ListSubjectGoals and ListGoalSubjects.
func (s *SQLiteSubjectGoalStore) ListActiveGoalsForSubject(ctx context.Context, userID string, subjectID int64) ([]Goal, error) {
	return s.ListActiveGoalsForSubjects(ctx, userID, []int64{subjectID})
}

func (s *SQLiteSubjectGoalStore) ListActiveGoalsForSubjects(ctx context.Context, userID string, subjectIDs []int64) (_ []Goal, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("list active goals for subject: %w", TranslateSQLiteError(err))
		}
	}()
	// One bound JSON array avoids a per-Subject read and SQLite variable limits.
	encoded, err := json.Marshal(subjectIDs)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+goalColumns+`
FROM goals
WHERE user_id = ?
    AND goal_is_active = 1
    AND `+visibleGoals+`
    AND EXISTS (
        SELECT 1
        FROM subject_goals
        JOIN subjects ON subjects.subject_id = subject_goals.subject_id
            AND subjects.user_id = subject_goals.user_id
        WHERE subject_goals.subject_id IN (SELECT value FROM json_each(?))
            AND subject_goals.user_id = goals.user_id
            AND subject_goals.goal_id = goals.goal_id
            AND subject_goals.deleted_at IS NULL
            AND subjects.deleted_at IS NULL
    )
ORDER BY CASE priority WHEN 'high' THEN 0 WHEN 'normal' THEN 1 ELSE 2 END, goal_id`, userID, string(encoded))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Goal, 0)
	for rows.Next() {
		item, err := scanGoal(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *SQLiteSubjectGoalStore) listSubjectGoals(ctx context.Context, userID string, id int64, selection string) (_ []SubjectGoal, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("list subject goal relationships: %w", TranslateSQLiteError(err))
		}
	}()
	rows, err := s.db.QueryContext(ctx, `
        SELECT
            subject_id,
            goal_id,
            user_id,
            goal_number,
            deleted_at
        FROM subject_goals
        WHERE user_id = ?
          AND EXISTS (
              SELECT 1
              FROM users
              WHERE user_id = ?
                  AND deleted_at IS NULL
          )
          AND `+selection, userID, userID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]SubjectGoal, 0)
	for rows.Next() {
		var item SubjectGoal
		var deletedAt sql.NullInt64
		if err := rows.Scan(&item.SubjectID, &item.GoalID, &item.UserID, &item.GoalNumber, &deletedAt); err != nil {
			return nil, err
		}
		if deletedAt.Valid {
			retired := TimeFromUnixSec(deletedAt.Int64)
			item.DeletedAt = &retired
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
