package data

import (
	"context"
	"database/sql"
)

// Validate the complete replacement before retiring any current relationships.
// The caller owns the Event write transaction, including its version check.
func validateEventSubjects(ctx context.Context, tx *sql.Tx, userID string, subjectIDs []int64) error {
	seen := make(map[int64]struct{}, len(subjectIDs))
	for _, subjectID := range subjectIDs {
		if _, duplicate := seen[subjectID]; duplicate {
			return ErrDuplicateRecord
		}
		seen[subjectID] = struct{}{}
		var available bool
		err := tx.QueryRowContext(ctx, `
		    SELECT EXISTS (
		        SELECT 1
		        FROM subjects
		        JOIN users ON users.user_id = subjects.user_id
		        WHERE subjects.subject_id = ?
		            AND subjects.user_id = ?
		            AND subjects.deleted_at IS NULL
		            AND users.deleted_at IS NULL
		    )
		`, subjectID, userID).Scan(&available)
		if err != nil {
			return err
		}
		if !available {
			return ErrRecordNotFound
		}
	}
	return ctx.Err()
}

// replaceEventSubjects preserves retained pair identity. It does not advance the
// Event version: the surrounding Event mutation validates the selected Subjects
// before any mutation and advances the Event version exactly once.
func replaceEventSubjects(ctx context.Context, tx *sql.Tx, userID string, eventID int64, subjectIDs []int64, retiredAt int64) error {
	selected := make(map[int64]bool, len(subjectIDs))
	for _, subjectID := range subjectIDs {
		selected[subjectID] = true
	}
	rows, err := tx.QueryContext(ctx, `
	    SELECT subject_id
	    FROM event_subjects
	    WHERE user_id = ?
	        AND event_id = ?
	        AND deleted_at IS NULL
	    ORDER BY subject_id
	`, userID, eventID)
	if err != nil {
		return err
	}
	var removed []int64
	for rows.Next() {
		var subjectID int64
		if err := rows.Scan(&subjectID); err != nil {
			rows.Close()
			return err
		}
		if selected[subjectID] {
			delete(selected, subjectID)
		} else {
			removed = append(removed, subjectID)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, subjectID := range removed {
		if _, err := tx.ExecContext(ctx, `
		    UPDATE event_subjects
		    SET deleted_at = ?
		    WHERE user_id = ?
		        AND event_id = ?
		        AND subject_id = ?
		        AND deleted_at IS NULL
		`, retiredAt, userID, eventID, subjectID); err != nil {
			return err
		}
	}
	for _, subjectID := range subjectIDs {
		if !selected[subjectID] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
		    INSERT INTO event_subjects (event_id, subject_id, user_id) VALUES (?, ?, ?)
		    ON CONFLICT (event_id, subject_id)
		    DO UPDATE SET deleted_at = NULL
		    WHERE event_subjects.user_id = excluded.user_id
		        AND event_subjects.deleted_at IS NOT NULL
		`, eventID, subjectID, userID); err != nil {
			return err
		}
	}
	return nil
}
