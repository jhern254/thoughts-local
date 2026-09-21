package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

type SQLiteSubjectGoalStore struct{ db *sql.DB }

func NewSQLiteSubjectGoalStore(db *sql.DB) *SQLiteSubjectGoalStore {
	return &SQLiteSubjectGoalStore{db: db}
}

// AddSubjectGoal links undeleted same-owner endpoints, including inactive goals.
func (s *SQLiteSubjectGoalStore) AddSubjectGoal(ctx context.Context, userID string, subjectID, goalID int64) error {
	result, err := s.db.ExecContext(ctx, `INSERT INTO subject_goals(subject_id,goal_id,user_id)
 SELECT ?,?,? WHERE EXISTS (SELECT 1 FROM users WHERE user_id=? AND deleted_at IS NULL)
 AND EXISTS (SELECT 1 FROM subjects WHERE subject_id=? AND user_id=? AND deleted_at IS NULL)
 AND EXISTS (SELECT 1 FROM goals WHERE goal_id=? AND user_id=? AND deleted_at IS NULL)`, subjectID, goalID, userID, userID, subjectID, userID, goalID, userID)
	if err != nil {
		var cause *sqlite.Error
		if errors.As(err, &cause) && cause.Code() == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY {
			err = &databaseError{cause: err, kind: ErrDuplicateRecord}
		}
		return fmt.Errorf("add subject goal: %w", TranslateSQLiteError(err))
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("add subject goal: %w", TranslateSQLiteError(err))
	}
	if count == 0 {
		return ErrRecordNotFound
	}
	return nil
}

// RemoveSubjectGoal removes only the link, including links to retained deleted endpoints.
func (s *SQLiteSubjectGoalStore) RemoveSubjectGoal(ctx context.Context, userID string, subjectID, goalID int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM subject_goals WHERE user_id=? AND subject_id=? AND goal_id=?
 AND EXISTS (SELECT 1 FROM users WHERE user_id=? AND deleted_at IS NULL)`, userID, subjectID, goalID, userID)
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
func (s *SQLiteSubjectGoalStore) ListActiveGoalsForSubject(ctx context.Context, userID string, subjectID int64) (_ []Goal, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("list active goals for subject: %w", TranslateSQLiteError(err))
		}
	}()
	rows, err := s.db.QueryContext(ctx, `SELECT `+goalColumns+` FROM goals
 WHERE user_id=? AND goal_is_active=1 AND `+visibleGoals+`
 AND EXISTS (
     SELECT 1 FROM subject_goals sg
     JOIN subjects s ON s.subject_id=sg.subject_id AND s.user_id=sg.user_id
     WHERE sg.subject_id=? AND sg.user_id=goals.user_id AND sg.goal_id=goals.goal_id
       AND s.deleted_at IS NULL
 ) ORDER BY CASE priority WHEN 'high' THEN 0 WHEN 'normal' THEN 1 ELSE 2 END, goal_id`, userID, subjectID)
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
	rows, err := s.db.QueryContext(ctx, `SELECT subject_id,goal_id,user_id FROM subject_goals
 WHERE user_id=? AND EXISTS (SELECT 1 FROM users WHERE user_id=? AND deleted_at IS NULL) AND `+selection, userID, userID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]SubjectGoal, 0)
	for rows.Next() {
		var item SubjectGoal
		if err := rows.Scan(&item.SubjectID, &item.GoalID, &item.UserID); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
