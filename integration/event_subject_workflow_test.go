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
		_, err := db.Exec(`INSERT INTO subjects(subject_id,user_id,subject_name) VALUES (1,'u','coding');
   INSERT INTO events(event_id,user_id,subject_id,started_at,ended_at,created_at,updated_at,deleted_at) VALUES
    (1,'u',1,1,2,1,2,NULL),(2,'u',1,3,4,1,4102444800,2);
   INSERT INTO thoughts(user_id,subject_id,event_id,thought) VALUES ('u',1,1,'synthetic');
   INSERT INTO goals(goal_id,user_id,goal_name,target_seconds) VALUES (1,'u','goal',60);
   INSERT INTO goal_progress(goal_id,user_id,time_spent_sec,event_id) VALUES (1,'u',60,1);`)
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
			{`SELECT count(*) FROM events WHERE subject_id IS NULL AND version=2`, 2},
			{`SELECT updated_at FROM events WHERE event_id=2`, 4102444800},
			{`SELECT count(*) FROM thoughts WHERE subject_id IS NULL AND event_id=1 AND version=2`, 1},
			{`SELECT sum(time_spent_sec) FROM goal_progress WHERE event_id=1`, 60},
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
		if _, err := db.Exec(`INSERT INTO subjects(subject_id,user_id,subject_name) VALUES (1,'u','coding'),(2,'u','reading')`); err != nil {
			t.Fatal(err)
		}
		service := event.NewService(data.NewSQLiteEventStore(db))
		first, second := int64(1), int64(2)
		item, err := service.Create(t.Context(), "u", "activity", eventTime(100), &first)
		if err != nil {
			t.Fatal(err)
		}
		if item.SubjectID == nil || *item.SubjectID != first {
			t.Fatalf("got subject %v, want 1", item.SubjectID)
		}
		ended, err := service.End(t.Context(), "u", item.EventID, item.Version, eventTime(200))
		if err != nil {
			t.Fatal(err)
		}
		if ended.SubjectID == nil || *ended.SubjectID != first {
			t.Fatalf("got ended subject %v, want 1", ended.SubjectID)
		}
		updated, err := service.Update(t.Context(), "u", ended.EventID, ended.Version, event.UpdateInput{
			Activity:  "activity",
			StartedAt: ended.StartedAt,
			EndedAt:   ended.EndedAt,
			SubjectID: &second,
		})
		if err != nil {
			t.Fatal(err)
		}
		got, err := service.Get(t.Context(), "u", updated.EventID)
		if err != nil {
			t.Fatal(err)
		}
		if got.SubjectID == nil || *got.SubjectID != second {
			t.Fatalf("got retrieved subject %v, want 2", got.SubjectID)
		}
		items, err := service.List(t.Context(), "u", eventTime(50), eventTime(300))
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 1 || items[0].SubjectID == nil || *items[0].SubjectID != second {
			t.Fatalf("got listed events %+v, want one with subject 2", items)
		}
		cleared, err := service.Update(t.Context(), "u", got.EventID, got.Version, event.UpdateInput{
			Activity:  "activity",
			StartedAt: got.StartedAt,
			EndedAt:   got.EndedAt,
			SubjectID: nil,
		})
		if err != nil {
			t.Fatal(err)
		}
		if cleared.SubjectID != nil {
			t.Fatalf("got subject %v, want nil", cleared.SubjectID)
		}
		past, err := service.CreatePast(t.Context(), "u", "past", eventTime(10), eventTime(20), &first)
		if err != nil {
			t.Fatal(err)
		}
		if past.SubjectID == nil || *past.SubjectID != first {
			t.Fatalf("got past subject %v, want 1", past.SubjectID)
		}
	})
	for _, id := range []int64{2, 3, 99} {
		t.Run(fmt.Sprintf("unavailable subject %d leaves predecessor unchanged", id), func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			insertUsers(t, db, "u", "other")
			if _, err := db.Exec(`INSERT INTO subjects(subject_id,user_id,subject_name,deleted_at) VALUES (1,'u','keep',NULL),(2,'other','private',NULL),(3,'u','deleted',unixepoch('now'))`); err != nil {
				t.Fatal(err)
			}
			service := event.NewService(data.NewSQLiteEventStore(db))
			subjectID := int64(1)
			before, err := service.Create(t.Context(), "u", "keep", eventTime(100), &subjectID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.Create(t.Context(), "u", "reject", eventTime(200), &id); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("got create error %v, want not found", err)
			}
			if _, err := service.CreatePast(t.Context(), "u", "reject", eventTime(10), eventTime(20), &id); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("got past error %v, want not found", err)
			}
			if _, err := service.Update(t.Context(), "u", before.EventID, before.Version, event.UpdateInput{
				Activity:  "reject",
				StartedAt: before.StartedAt,
				EndedAt:   nil,
				SubjectID: &id,
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
		if _, err := db.Exec(`INSERT INTO subjects(subject_id,user_id,subject_name) VALUES (1,'u','coding')`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO events(user_id,subject_id) VALUES ('other',1)`); err == nil {
			t.Fatal("got nil ownership error, want constraint")
		}
		if _, err := db.Exec(`INSERT INTO events(user_id,subject_id) VALUES ('u',1); DELETE FROM subjects WHERE subject_id=1`); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM events WHERE user_id='u' AND subject_id IS NULL`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("got retained events %d, want 1", count)
		}
	})
	t.Run("event unlink failure rolls back subject and thought unlinking", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		if _, err := db.Exec(`INSERT INTO subjects(subject_id,user_id,subject_name) VALUES (1,'u','coding');
   INSERT INTO events(user_id,subject_id) VALUES ('u',1);
   INSERT INTO thoughts(user_id,subject_id,thought) VALUES ('u',1,'keep');
   CREATE TRIGGER reject_event_unlink BEFORE UPDATE OF subject_id ON events BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); err != nil {
			t.Fatal(err)
		}
		err := data.NewSQLiteSubjectStore(db).DeleteSubject(t.Context(), "u", 1)
		var cause interface{ Code() int }
		if !errors.As(err, &cause) {
			t.Fatalf("got error %v, want wrapped SQLite cause", err)
		}
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM subjects s JOIN events e ON e.subject_id=s.subject_id JOIN thoughts t ON t.subject_id=s.subject_id WHERE s.deleted_at IS NULL AND e.version=1 AND t.version=1`).Scan(&count); err != nil {
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
			if _, err := db.Exec(`INSERT INTO subjects(subject_id,user_id,subject_name) VALUES (1,'u','coding'),(2,'u','reading')`); err != nil {
				t.Fatal(err)
			}
			service := event.NewService(data.NewSQLiteEventStore(db))
			first, second := int64(1), int64(2)
			current, err := service.CreatePast(t.Context(), "u", "original", eventTime(100), eventTime(200), &first)
			if err != nil {
				t.Fatal(err)
			}
			end := eventTime(250)
			input := event.UpdateInput{
				Activity:  "corrected",
				StartedAt: eventTime(50),
				EndedAt:   &end,
				SubjectID: current.SubjectID,
			}
			switch scenario {
			case "another subject reassigns event":
				input.SubjectID = &second
			case "nil subject explicitly clears assignment":
				input.SubjectID = nil
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
			if input.SubjectID == nil {
				if got.SubjectID != nil {
					t.Fatalf("got subject ID %d, want nil", *got.SubjectID)
				}
			} else {
				if got.SubjectID == nil {
					t.Fatalf("got nil subject ID, want %d", *input.SubjectID)
				}
				if got, want := *got.SubjectID, *input.SubjectID; got != want {
					t.Fatalf("got subject ID %d, want %d", got, want)
				}
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
