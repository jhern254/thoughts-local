package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// CreateGoalWithSubjects persists a prepared Goal and its complete selection in
// one transaction. SQLite remains authoritative for ownership and uniqueness.
func (s *SQLiteSubjectGoalStore) CreateGoalWithSubjects(ctx context.Context, item *Goal, subjectIDs []int64) (_ *Goal, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("create goal with subjects: %w", translateGoalError(err))
		}
	}()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	created, err := scanGoal(tx.QueryRowContext(ctx, insertGoalSQL, item.UserID, item.GoalName, item.TargetSeconds, item.Priority,
		item.StartDate, item.EndDate, item.IsActive, item.Cadence, item.TZ, item.WeekStart, item.DefaultCadence, item.CreatedAt.Unix(), item.UpdatedAt.Unix(), item.UserID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRecordNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := validateGoalSubjects(ctx, tx, item.UserID, subjectIDs); err != nil {
		return nil, err
	}
	for _, id := range subjectIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO subject_goals(subject_id,goal_id,user_id) VALUES(?,?,?)`, id, created.GoalID, item.UserID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

func validateGoalSubjects(ctx context.Context, tx *sql.Tx, userID string, subjectIDs []int64) error {
	for _, id := range subjectIDs {
		var live bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM subjects WHERE user_id=? AND subject_id=? AND deleted_at IS NULL)`, userID, id).Scan(&live); err != nil {
			return err
		}
		if !live {
			return ErrRecordNotFound
		}
	}
	return nil
}

// ReplaceGoalSubjects preserves retained links to deleted Subjects. Taking the
// write lock before reading membership prevents a concurrent link mutation from
// bypassing the caller's version check, including an otherwise identical set.
func (s *SQLiteSubjectGoalStore) ReplaceGoalSubjects(ctx context.Context, userID string, goalID, expectedVersion int64, subjectIDs []int64) (_ *Goal, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("replace goal subjects: %w", translateGoalError(err))
		}
	}()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE goals SET version=version WHERE user_id=? AND goal_id=? AND version=? AND `+visibleGoals, userID, goalID, expectedVersion)
	if err != nil {
		return nil, err
	}
	if err := checkGoalMutation(ctx, tx, result, userID, goalID); err != nil {
		return nil, err
	}
	if err := validateGoalSubjects(ctx, tx, userID, subjectIDs); err != nil {
		return nil, err
	}
	current, err := listLiveGoalSubjects(ctx, tx, userID, goalID)
	if err != nil {
		return nil, err
	}
	selected := make(map[int64]bool, len(subjectIDs))
	for _, id := range subjectIDs {
		selected[id] = true
	}
	changed := false
	for _, subject := range current {
		if selected[subject.SubjectID] {
			delete(selected, subject.SubjectID)
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM subject_goals WHERE user_id=? AND goal_id=? AND subject_id=?`, userID, goalID, subject.SubjectID); err != nil {
			return nil, err
		}
		changed = true
	}
	for _, id := range subjectIDs {
		if !selected[id] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO subject_goals(subject_id,goal_id,user_id) VALUES(?,?,?)`, id, goalID, userID); err != nil {
			return nil, err
		}
		changed = true
	}
	if changed {
		if err := advanceGoalAssociationVersion(ctx, tx, userID, goalID); err != nil {
			return nil, err
		}
	}
	item, err := scanGoal(tx.QueryRowContext(ctx, `SELECT `+goalColumns+` FROM goals WHERE user_id=? AND goal_id=?`, userID, goalID))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return item, nil
}

func advanceGoalAssociationVersion(ctx context.Context, tx *sql.Tx, userID string, goalID int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE goals SET version=version+1, updated_at=max(updated_at,unixepoch('now')) WHERE user_id=? AND goal_id=?`, userID, goalID)
	return err
}

// ListLiveSubjectsForGoal distinguishes an accessible Unassigned Goal (a nonnil
// empty slice) from an unavailable Goal (ErrRecordNotFound). Inactive Goals are visible.
func (s *SQLiteSubjectGoalStore) ListLiveSubjectsForGoal(ctx context.Context, userID string, goalID int64) (_ []Subject, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("list live subjects for goal: %w", TranslateSQLiteError(err))
		}
	}()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM goals WHERE user_id=? AND goal_id=? AND `+visibleGoals+`)`, userID, goalID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrRecordNotFound
	}
	subjects, err := listLiveGoalSubjects(ctx, tx, userID, goalID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return subjects, nil
}

func listLiveGoalSubjects(ctx context.Context, tx *sql.Tx, userID string, goalID int64) ([]Subject, error) {
	rows, err := tx.QueryContext(ctx, `SELECT s.subject_id,s.user_id,s.subject_name,s.created_at,s.updated_at
 FROM subjects s JOIN subject_goals sg ON s.subject_id=sg.subject_id AND s.user_id=sg.user_id
 WHERE sg.user_id=? AND sg.goal_id=? AND s.deleted_at IS NULL ORDER BY s.subject_id`, userID, goalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	subjects := make([]Subject, 0)
	for rows.Next() {
		subject, err := scanSubject(rows)
		if err != nil {
			return nil, err
		}
		subjects = append(subjects, *subject)
	}
	return subjects, rows.Err()
}
