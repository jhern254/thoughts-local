package data

import (
	"context"
	"database/sql"
	"math"
	"slices"
)

// Each revision is a complete snapshot, including an empty recipient set. The
// Event transaction owns publication; label changes deliberately leave gaps in
// the recorded Event versions. No operation amends an earlier snapshot.
func appendEventAttribution(ctx context.Context, tx *sql.Tx, current *Event, previous *Event) error {
	if current.EndedAt == nil {
		return nil
	}
	start, end := current.StartedAt.Unix(), current.EndedAt.Unix()
	if end < start || (start < 0 && end > math.MaxInt64+start) {
		return ErrInvalidEventInterval
	}
	var version sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(event_version) FROM event_credit_revisions WHERE user_id = ? AND event_id = ?`, current.UserID, current.EventID).Scan(&version); err != nil {
		return err
	}
	var added, removed []int64
	reason := "completion"
	if previous != nil {
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
		classification := len(added) > 0 || len(removed) > 0
		interval := !current.StartedAt.Equal(previous.StartedAt) || !current.EndedAt.Equal(*previous.EndedAt)
		if !classification && (!interval || !version.Valid) {
			return nil
		}
		reason = "interval_correction"
		if classification {
			reason = "classification_correction"
		}
	} else {
		added = current.SubjectIDs
	}
	var recordedAt int64
	if err := tx.QueryRowContext(ctx, `
    INSERT INTO event_credit_revisions
        (event_id, event_version, user_id, credited_start_at, credited_end_at, reason)
    VALUES (?, ?, ?, ?, ?, ?)
    RETURNING recorded_at`, current.EventID, current.Version, current.UserID, start, end, reason).Scan(&recordedAt); err != nil {
		return err
	}
	if version.Valid {
		// A detached support has no live Subject reference and cannot be explicitly
		// removed by the ordinary complete-live-set Event update contract.
		rows, err := tx.QueryContext(ctx, `
    SELECT goal_id, recorded_subject_id, subject_id, attributed_at
    FROM event_goal_credit_subjects
    WHERE user_id = ? AND event_id = ? AND event_version = ?
    ORDER BY support_id`, current.UserID, current.EventID, version.Int64)
		if err != nil {
			return err
		}
		type support struct {
			goalID, recordedSubjectID, attributedAt int64
			subjectID                               sql.NullInt64
		}
		var retained []support
		for rows.Next() {
			var s support
			if err := rows.Scan(&s.goalID, &s.recordedSubjectID, &s.subjectID, &s.attributedAt); err != nil {
				rows.Close()
				return err
			}
			if !s.subjectID.Valid || !slices.Contains(removed, s.subjectID.Int64) {
				retained = append(retained, s)
			}
		}
		err = rows.Err()
		closeErr := rows.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		for _, s := range retained {
			if err := insertEventGoalCreditSupport(ctx, tx, current, s.goalID, s.recordedSubjectID, s.subjectID, s.attributedAt); err != nil {
				return err
			}
		}
	}
	for _, subjectID := range added {
		rows, err := tx.QueryContext(ctx, `
    SELECT sg.goal_id
    FROM subject_goals sg
    JOIN goals g ON g.goal_id = sg.goal_id AND g.user_id = sg.user_id
    JOIN subjects s ON s.subject_id = sg.subject_id AND s.user_id = sg.user_id
    WHERE sg.user_id = ? AND sg.subject_id = ?
        AND sg.deleted_at IS NULL AND s.deleted_at IS NULL
        AND g.deleted_at IS NULL AND g.goal_is_active = 1
    ORDER BY sg.goal_id`, current.UserID, subjectID)
		if err != nil {
			return err
		}
		var goals []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			goals = append(goals, id)
		}
		err = rows.Err()
		closeErr := rows.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		for _, id := range goals {
			if err := insertEventGoalCreditSupport(ctx, tx, current, id, subjectID, sql.NullInt64{Int64: subjectID, Valid: true}, recordedAt); err != nil {
				return err
			}
		}
	}
	return nil
}

func insertEventGoalCreditSupport(ctx context.Context, tx *sql.Tx, event *Event, goalID, recordedSubjectID int64, subjectID sql.NullInt64, attributedAt int64) error {
	if _, err := tx.ExecContext(ctx, `
    INSERT INTO event_goal_credits (event_id, event_version, goal_id, user_id) VALUES (?, ?, ?, ?)
    ON CONFLICT (event_id, event_version, goal_id) DO NOTHING`, event.EventID, event.Version, goalID, event.UserID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
    INSERT INTO event_goal_credit_subjects
        (event_id, event_version, goal_id, user_id, recorded_subject_id, subject_id, attributed_at)
    VALUES (?, ?, ?, ?, ?, ?, ?)`, event.EventID, event.Version, goalID, event.UserID, recordedSubjectID, subjectID, attributedAt)
	return err
}
