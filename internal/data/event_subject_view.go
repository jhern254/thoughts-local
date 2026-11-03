package data

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"
)

// EventSubjectView is current display metadata, not a second assignment source.
type EventSubjectView struct {
	SubjectID   int64
	SubjectName string
}
type EventWithSubjects struct {
	Event    Event
	Subjects []EventSubjectView
}

type eventSubjectScanner struct {
	row      eventScanner
	subjects *string
}

func (s eventSubjectScanner) Scan(fields ...any) error {
	return s.row.Scan(append(fields, s.subjects)...)
}

// A single statement keeps the Event, live membership, and display names in the
// same SQLite read snapshot without one Subject query per timeline entry.
func (s *SQLiteEventStore) eventViews(ctx context.Context, userID, selection string, args ...any) ([]EventWithSubjects, error) {
	query := `SELECT ` + eventColumns + `, (
   SELECT json_group_array(json_object('SubjectID', subjects.subject_id, 'SubjectName', subjects.subject_name))
   FROM event_subjects
   INNER JOIN subjects ON subjects.subject_id = event_subjects.subject_id
       AND subjects.user_id = event_subjects.user_id
   WHERE event_subjects.event_id = events.event_id
     AND event_subjects.user_id = events.user_id
     AND event_subjects.deleted_at IS NULL
     AND subjects.deleted_at IS NULL
 )` + activeEvents + ` AND user_id = ? AND ` + selection + ` ORDER BY started_at, event_id`
	rows, err := s.db.QueryContext(ctx, query, append([]any{userID}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("read Events with Subjects: %w", TranslateSQLiteError(err))
	}
	defer rows.Close()
	views := []EventWithSubjects{}
	for rows.Next() {
		var subjects string
		item, err := scanEvent(eventSubjectScanner{rows, &subjects})
		if err != nil {
			return nil, err
		}
		view := EventWithSubjects{Event: *item}
		if err := json.Unmarshal([]byte(subjects), &view.Subjects); err != nil {
			return nil, err
		}
		slices.SortFunc(view.Subjects, func(a, b EventSubjectView) int {
			if a.SubjectID < b.SubjectID {
				return -1
			}
			if a.SubjectID > b.SubjectID {
				return 1
			}
			return 0
		})
		views = append(views, view)
	}
	return views, rows.Err()
}
func (s *SQLiteEventStore) ListEventsWithSubjects(ctx context.Context, userID string, from, until time.Time) ([]EventWithSubjects, error) {
	if until.Unix() <= from.Unix() {
		return nil, ErrInvalidEventInterval
	}
	return s.eventViews(ctx, userID, `started_at < ? AND (ended_at IS NULL OR ended_at > ? OR (ended_at = started_at AND started_at >= ?))`, until.Unix(), from.Unix(), from.Unix())
}
func (s *SQLiteEventStore) GetEventWithSubjects(ctx context.Context, userID string, id int64) (*EventWithSubjects, error) {
	views, err := s.eventViews(ctx, userID, `event_id = ?`, id)
	if err != nil {
		return nil, err
	}
	if len(views) == 0 {
		return nil, ErrRecordNotFound
	}
	return &views[0], nil
}
