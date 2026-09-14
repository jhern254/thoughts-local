package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

type SQLiteGoalParentStore struct{ db *sql.DB }

func NewSQLiteGoalParentStore(db *sql.DB) *SQLiteGoalParentStore {
	return &SQLiteGoalParentStore{db: db}
}

// AddGoalParent links undeleted same-owner goals; constraints reject cycles.
func (s *SQLiteGoalParentStore) AddGoalParent(ctx context.Context, userID string, goalID, parentGoalID int64) error {
	result, err := s.db.ExecContext(ctx, `INSERT INTO goal_parents(goal_id,parent_goal_id,user_id)
 SELECT ?,?,? WHERE EXISTS (SELECT 1 FROM users WHERE user_id=? AND deleted_at IS NULL)
 AND EXISTS (SELECT 1 FROM goals WHERE goal_id=? AND user_id=? AND deleted_at IS NULL)
 AND EXISTS (SELECT 1 FROM goals WHERE goal_id=? AND user_id=? AND deleted_at IS NULL)`, goalID, parentGoalID, userID, userID, goalID, userID, parentGoalID, userID)
	if err != nil {
		var cause *sqlite.Error
		if errors.As(err, &cause) && cause.Code() == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY {
			err = &databaseError{cause: err, kind: ErrDuplicateRecord}
		}
		return fmt.Errorf("add goal parent: %w", TranslateSQLiteError(err))
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("add goal parent: %w", TranslateSQLiteError(err))
	}
	if count == 0 {
		return ErrRecordNotFound
	}
	return nil
}

// RemoveGoalParent removes only the edge, including links to retained deleted goals.
func (s *SQLiteGoalParentStore) RemoveGoalParent(ctx context.Context, userID string, goalID, parentGoalID int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM goal_parents WHERE user_id=? AND goal_id=? AND parent_goal_id=?
 AND EXISTS (SELECT 1 FROM users WHERE user_id=? AND deleted_at IS NULL)`, userID, goalID, parentGoalID, userID)
	if err != nil {
		return fmt.Errorf("remove goal parent: %w", TranslateSQLiteError(err))
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("remove goal parent: %w", TranslateSQLiteError(err))
	}
	if count == 0 {
		return ErrRecordNotFound
	}
	return nil
}

// ListGoalParents returns direct retained links in parent ID order.
func (s *SQLiteGoalParentStore) ListGoalParents(ctx context.Context, userID string, goalID int64) ([]GoalParent, error) {
	return s.listGoalLinks(ctx, userID, goalID, `goal_id = ? ORDER BY parent_goal_id`)
}

// ListGoalChildren returns direct retained links in child ID order.
func (s *SQLiteGoalParentStore) ListGoalChildren(ctx context.Context, userID string, parentGoalID int64) ([]GoalParent, error) {
	return s.listGoalLinks(ctx, userID, parentGoalID, `parent_goal_id = ? ORDER BY goal_id`)
}

func (s *SQLiteGoalParentStore) listGoalLinks(ctx context.Context, userID string, id int64, selection string) (_ []GoalParent, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("list goal relationships: %w", TranslateSQLiteError(err))
		}
	}()
	rows, err := s.db.QueryContext(ctx, `SELECT goal_id,parent_goal_id,user_id FROM goal_parents
 WHERE user_id=? AND EXISTS (SELECT 1 FROM users WHERE user_id=? AND deleted_at IS NULL) AND `+selection, userID, userID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]GoalParent, 0)
	for rows.Next() {
		item, err := scanGoalParent(rows)
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

func scanGoalParent(row interface{ Scan(...any) error }) (*GoalParent, error) {
	var item GoalParent
	if err := row.Scan(&item.GoalID, &item.ParentGoalID, &item.UserID); err != nil {
		return nil, err
	}
	return &item, nil
}
