package data

import (
	"context"
	"database/sql"
	"slices"
)

// reconcileEventGoalContributions records which Goals earn time from a completed Event
// through its Subjects. It creates contributions on completion and adds or removes
// contributions when its Subject selection changes, within the Event transaction.
//
// Contributions through unchanged or deleted Subjects are retained. Label and
// time edits leave them untouched; summaries read the current Event duration.
func reconcileEventGoalContributions(ctx context.Context, tx *sql.Tx, current, previous *Event) error {
	if current.EndedAt == nil {
		return nil
	}
	var added, removed []int64
	if previous == nil {
		added = current.SubjectIDs
	} else {
		for _, id := range current.SubjectIDs {
			if !slices.Contains(previous.SubjectIDs, id) {
				added = append(added, id)
			}
		}
		for _, id := range previous.SubjectIDs {
			if !slices.Contains(current.SubjectIDs, id) {
				removed = append(removed, id)
			}
		}
	}
	for _, subjectID := range removed {
		// NULL live references and automatically retired Subjects are absent from
		// the previous live set, so ordinary edits cannot retract their earned contributions.
		if _, err := tx.ExecContext(ctx, `DELETE FROM goal_progress WHERE user_id = ? AND event_id = ? AND subject_id = ?`, current.UserID, current.EventID, subjectID); err != nil {
			return err
		}
	}
	for _, subjectID := range added {
		_, err := tx.ExecContext(ctx, `
    INSERT INTO goal_progress (event_id, goal_id, user_id, recorded_subject_id, subject_id)
    SELECT
        es.event_id,
        sg.goal_id,
        es.user_id,
        es.subject_id,
        es.subject_id
    FROM event_subjects es
    JOIN subjects s ON s.subject_id = es.subject_id AND s.user_id = es.user_id
    JOIN subject_goals sg ON sg.subject_id = es.subject_id AND sg.user_id = es.user_id
    JOIN goals g ON g.goal_id = sg.goal_id AND g.user_id = sg.user_id
    JOIN users u ON u.user_id = es.user_id
    WHERE es.user_id = ? AND es.event_id = ? AND es.subject_id = ?
        AND es.deleted_at IS NULL AND s.deleted_at IS NULL
        AND sg.deleted_at IS NULL AND g.deleted_at IS NULL
        AND g.goal_is_active = 1 AND u.deleted_at IS NULL`, current.UserID, current.EventID, subjectID)
		if err != nil {
			return err
		}
	}
	return nil
}
