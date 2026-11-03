//go:build integration

package integration_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/event"
)

func TestEventSubjectWorkflow_SQLite(t *testing.T) {
	t.Run("subject deletion unlinks active and deleted events and thoughts together", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		_, err := db.Exec(`
		    INSERT INTO
		        subjects (subject_id, user_id, subject_name)
		    VALUES
		        (1, 'u', 'coding');

		    INSERT INTO
		        events (event_id, user_id, started_at, ended_at, created_at, updated_at, deleted_at)
		    VALUES
		        (1, 'u', 1, 2, 1, 2, NULL);

		    INSERT INTO
		        event_subjects (event_id, subject_id, user_id)
		    VALUES
		        (1, 1, 'u');

		    INSERT INTO
		        events (event_id, user_id, started_at, ended_at, created_at, updated_at, deleted_at)
		    VALUES
		        (2, 'u', 3, 4, 1, 4102444800, 2);

		    INSERT INTO
		        event_subjects (event_id, subject_id, user_id)
		    VALUES
		        (2, 1, 'u');

		    INSERT INTO
		        thoughts (user_id, subject_id, event_id, thought)
		    VALUES
		        ('u', 1, 1, 'synthetic');

		    INSERT INTO
		        goals (goal_id, user_id, goal_name, target_seconds)
		    VALUES
		        (1, 'u', 'goal', 60);

		    INSERT INTO
		        goal_progress (goal_id, user_id, time_spent_sec, event_id)
		    VALUES
		        (1, 'u', 60, 1);
		`)
		if err != nil {
			t.Fatal(err)
		}
		if err := data.NewSQLiteSubjectStore(db).DeleteSubject(t.Context(), "u", 1); err != nil {
			t.Fatal(err)
		}
		if _, err := data.NewSQLiteEventStore(db).EndEvent(t.Context(), "u", 1, 1, eventTime(3), eventTime(4)); !errors.Is(err, data.ErrEventVersionConflict) {
			t.Fatalf("got stale event error %v, want version conflict", err)
		}
		for _, check := range []struct {
			query string
			want  int64
		}{
			{`
			    SELECT
			        count(*)
			    FROM events
			    WHERE version = 2
			        AND NOT EXISTS (
			            SELECT
			                1
			            FROM event_subjects es
			            WHERE es.event_id = events.event_id
			                AND es.deleted_at IS NULL
			        )
			`, 2},
			{`
			    SELECT
			        updated_at
			    FROM events
			    WHERE event_id = 2
			`, 4102444800},
			{`
			    SELECT
			        count(*)
			    FROM thoughts
			    WHERE subject_id IS NULL
			        AND event_id = 1
			        AND version = 2
			`, 1},
			{`
			    SELECT
			        sum(time_spent_sec)
			    FROM goal_progress
			    WHERE event_id = 1
			`, 60},
		} {
			var got int64
			if err := db.QueryRow(check.query).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != check.want {
				t.Fatalf("%s: got %d, want %d", check.query, got, check.want)
			}
		}
	})

	t.Run("round trips reassigns clears and preserves subject on end", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		if _, err := db.Exec(`
		    INSERT INTO
		        subjects (subject_id, user_id, subject_name)
		    VALUES
		        (1, 'u', 'coding'),
		        (2, 'u', 'reading')
		`); err != nil {
			t.Fatal(err)
		}
		service := event.NewService(data.NewSQLiteEventStore(db))
		first, second := int64(1), int64(2)
		item, err := service.Create(t.Context(), "u", "activity", eventTime(100), []int64{first})
		if err != nil {
			t.Fatal(err)
		}
		if len(item.SubjectIDs) != 1 || item.SubjectIDs[0] != first {
			t.Fatalf("got subject %v, want 1", item.SubjectIDs)
		}
		ended, err := service.End(t.Context(), "u", item.EventID, item.Version, eventTime(200))
		if err != nil {
			t.Fatal(err)
		}
		if len(ended.SubjectIDs) != 1 || ended.SubjectIDs[0] != first {
			t.Fatalf("got ended subject %v, want 1", ended.SubjectIDs)
		}
		updated, err := service.Update(t.Context(), "u", ended.EventID, ended.Version, event.UpdateInput{
			Activity:   "activity",
			StartedAt:  ended.StartedAt,
			EndedAt:    ended.EndedAt,
			SubjectIDs: []int64{second},
		})
		if err != nil {
			t.Fatal(err)
		}
		got, err := service.Get(t.Context(), "u", updated.EventID)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.SubjectIDs) != 1 || got.SubjectIDs[0] != second {
			t.Fatalf("got retrieved subject %v, want 2", got.SubjectIDs)
		}
		items, err := service.List(t.Context(), "u", eventTime(50), eventTime(300))
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 1 || len(items[0].SubjectIDs) != 1 || items[0].SubjectIDs[0] != second {
			t.Fatalf("got listed events %+v, want one with subject 2", items)
		}
		cleared, err := service.Update(t.Context(), "u", got.EventID, got.Version, event.UpdateInput{
			Activity:   "activity",
			StartedAt:  got.StartedAt,
			EndedAt:    got.EndedAt,
			SubjectIDs: nil,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(cleared.SubjectIDs) != 0 {
			t.Fatalf("got subject %v, want nil", cleared.SubjectIDs)
		}
		past, err := service.CreatePast(t.Context(), "u", "past", eventTime(10), eventTime(20), []int64{first})
		if err != nil {
			t.Fatal(err)
		}
		if len(past.SubjectIDs) != 1 || past.SubjectIDs[0] != first {
			t.Fatalf("got past subject %v, want 1", past.SubjectIDs)
		}
	})
	for _, id := range []int64{2, 3, 99} {
		t.Run(fmt.Sprintf("unavailable subject %d leaves predecessor unchanged", id), func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			insertUsers(t, db, "u", "other")
			if _, err := db.Exec(`
			    INSERT INTO
			        subjects (subject_id, user_id, subject_name, deleted_at)
			    VALUES
			        (1, 'u', 'keep', NULL),
			        (2, 'other', 'private', NULL),
			        (3, 'u', 'deleted', unixepoch('now'))
			`); err != nil {
				t.Fatal(err)
			}
			service := event.NewService(data.NewSQLiteEventStore(db))
			subjectID := int64(1)
			before, err := service.Create(t.Context(), "u", "keep", eventTime(100), []int64{subjectID})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.Create(t.Context(), "u", "reject", eventTime(200), []int64{id}); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("got create error %v, want not found", err)
			}
			if _, err := service.CreatePast(t.Context(), "u", "reject", eventTime(10), eventTime(20), []int64{id}); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("got past error %v, want not found", err)
			}
			if _, err := service.Update(t.Context(), "u", before.EventID, before.Version, event.UpdateInput{
				Activity:   "reject",
				StartedAt:  before.StartedAt,
				EndedAt:    nil,
				SubjectIDs: []int64{id},
			}); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("got update error %v, want not found", err)
			}
			got, err := service.Get(t.Context(), "u", before.EventID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, before) {
				t.Fatalf("got predecessor %+v, want unchanged ongoing event %+v", got, before)
			}
		})
	}
	t.Run("database enforces ownership and hard deletion clears only the reference", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u", "other")
		if _, err := db.Exec(`
		    INSERT INTO
		        subjects (subject_id, user_id, subject_name)
		    VALUES
		        (1, 'u', 'coding')
		`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`
		    INSERT INTO
		        events (user_id)
		    VALUES
		        ('other');

		    INSERT INTO
		        event_subjects (event_id, subject_id, user_id)
		    VALUES
		        (last_insert_rowid(), 1, 'other')
		`); err == nil {
			t.Fatal("got nil ownership error, want constraint")
		}
		if _, err := db.Exec(`
		    INSERT INTO
		        events (user_id)
		    VALUES
		        ('u');

		    INSERT INTO
		        event_subjects (event_id, subject_id, user_id)
		    VALUES
		        (last_insert_rowid(), 1, 'u');

		    DELETE FROM subjects
		    WHERE subject_id = 1
		`); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := db.QueryRow(`
		    SELECT
		        count(*)
		    FROM events
		    WHERE user_id = 'u'
		        AND NOT EXISTS (
		            SELECT
		                1
		            FROM event_subjects es
		            WHERE es.event_id = events.event_id
		                AND es.deleted_at IS NULL
		        )
		`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("got retained events %d, want 1", count)
		}
	})
	t.Run("event unlink failure rolls back subject and thought unlinking", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		if _, err := db.Exec(`
		    INSERT INTO
		        subjects (subject_id, user_id, subject_name)
		    VALUES
		        (1, 'u', 'coding');

		    INSERT INTO
		        events (user_id)
		    VALUES
		        ('u');

		    INSERT INTO
		        event_subjects (event_id, subject_id, user_id)
		    VALUES
		        (last_insert_rowid(), 1, 'u');

		    INSERT INTO
		        thoughts (user_id, subject_id, thought)
		    VALUES
		        ('u', 1, 'keep');

		    CREATE TRIGGER reject_event_unlink BEFORE
		    UPDATE OF deleted_at ON event_subjects BEGIN
		    SELECT
		        RAISE (ABORT, 'synthetic failure');

		    END
		`); err != nil {
			t.Fatal(err)
		}
		err := data.NewSQLiteSubjectStore(db).DeleteSubject(t.Context(), "u", 1)
		var cause interface{ Code() int }
		if !errors.As(err, &cause) {
			t.Fatalf("got error %v, want wrapped SQLite cause", err)
		}
		var count int
		if err := db.QueryRow(`
		    SELECT
		        count(*)
		    FROM subjects s
		    JOIN event_subjects es ON es.subject_id = s.subject_id
		        AND es.deleted_at IS NULL
		    JOIN events e ON e.event_id = es.event_id
		        AND e.user_id = es.user_id
		    JOIN thoughts t ON t.subject_id = s.subject_id
		    WHERE s.deleted_at IS NULL
		        AND e.version = 1
		        AND t.version = 1
		`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("got unchanged links %d, want 1", count)
		}
	})
}

func TestEventUpdateSubjectWorkflow_SQLite(t *testing.T) {
	for _, scenario := range []string{
		"correction preserves subject when current assignment is supplied",
		"another subject reassigns event",
		"nil subject explicitly clears assignment",
	} {
		t.Run(scenario, func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			insertUsers(t, db, "u")
			if _, err := db.Exec(`
			    INSERT INTO
			        subjects (subject_id, user_id, subject_name)
			    VALUES
			        (1, 'u', 'coding'),
			        (2, 'u', 'reading')
			`); err != nil {
				t.Fatal(err)
			}
			service := event.NewService(data.NewSQLiteEventStore(db))
			first, second := int64(1), int64(2)
			current, err := service.CreatePast(t.Context(), "u", "original", eventTime(100), eventTime(200), []int64{first})
			if err != nil {
				t.Fatal(err)
			}
			end := eventTime(250)
			input := event.UpdateInput{
				Activity:   "corrected",
				StartedAt:  eventTime(50),
				EndedAt:    &end,
				SubjectIDs: current.SubjectIDs,
			}
			switch scenario {
			case "another subject reassigns event":
				input.SubjectIDs = []int64{second}
			case "nil subject explicitly clears assignment":
				input.SubjectIDs = nil
			}
			updated, err := service.Update(t.Context(), "u", current.EventID, current.Version, input)
			if err != nil {
				t.Fatal(err)
			}
			got, err := service.Get(t.Context(), "u", current.EventID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, updated) {
				t.Fatalf("got persisted event %+v, want returned event %+v", got, updated)
			}
			if !reflect.DeepEqual(got.SubjectIDs, append([]int64{}, input.SubjectIDs...)) {
				t.Fatalf("Subject IDs got %v, want %v", got.SubjectIDs, input.SubjectIDs)
			}

			if got.ActivityType == nil || *got.ActivityType != input.Activity || !got.StartedAt.Equal(input.StartedAt) || got.EndedAt == nil || !got.EndedAt.Equal(end) {
				t.Fatalf("got editable state %+v, want %+v", got, input)
			}
			if got.Version != current.Version+1 || !got.CreatedAt.Equal(current.CreatedAt) || got.UpdatedAt.Before(current.UpdatedAt) {
				t.Fatalf("got metadata %+v, want advanced version and retained creation with nondecreasing update time", got)
			}
		})
	}
}
