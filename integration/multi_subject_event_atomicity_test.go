//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/event"
)

func multiSubjectEventFixture(t *testing.T) (*sql.DB, *event.Service) {
	t.Helper()
	db, _ := openMigratedSQLite(t)
	insertUsers(t, db, "u", "other")
	if _, err := db.Exec(`
	    INSERT INTO
	        subjects (subject_id, user_id, subject_name)
	    VALUES
	        (1, 'u', 'Coding'),
	        (2, 'u', 'Research'),
	        (3, 'u', 'Projects'),
	        (4, 'other', 'Foreign');

	    INSERT INTO
	        subjects (subject_id, user_id, subject_name, deleted_at)
	    VALUES
	        (5, 'u', 'Deleted', unixepoch());
	`); err != nil {
		t.Fatal(err)
	}
	return db, event.NewService(data.NewSQLiteEventStore(db))
}

func TestMultiSubjectEventWorkflow_SQLite(t *testing.T) {
	for _, tc := range []struct {
		name string
		ids  []int64
	}{{"unassigned", nil}, {"one Subject", []int64{1}}, {"several Subjects", []int64{3, 1, 2}}} {
		t.Run("creates historical Event with "+tc.name, func(t *testing.T) {
			db, service := multiSubjectEventFixture(t)
			item, err := service.CreatePast(t.Context(), "u", "Activity", eventTime(100), eventTime(200), tc.ids)
			if err != nil {
				t.Fatal(err)
			}
			want := slices.Clone(tc.ids)
			slices.Sort(want)
			read, err := service.Get(t.Context(), "u", item.EventID)
			if err != nil || !slices.Equal(read.SubjectIDs, want) {
				t.Fatalf("Subjects got %+v, %v; want %v", read, err, want)
			}
			var count int
			if err := db.QueryRow(`
			    SELECT
			        count(*)
			    FROM events
			`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("Event count got %d, want 1", count)
			}
			views, err := service.ListWithSubjects(t.Context(), "u", eventTime(50), eventTime(250))
			if err != nil || len(views) != 1 || len(views[0].Subjects) != len(want) {
				t.Fatalf("timeline views got %+v, %v; want one Event with %d Subjects", views, err, len(want))
			}
		})
	}
	for _, tc := range []struct {
		name string
		ids  []int64
	}{{"duplicate", []int64{1, 1}}, {"missing", []int64{1, 999}}, {"foreign", []int64{1, 4}}, {"deleted", []int64{1, 5}}} {
		t.Run("rejects "+tc.name+" set without closing predecessor or replacing links", func(t *testing.T) {
			db, service := multiSubjectEventFixture(t)
			before, err := service.Create(t.Context(), "u", "Keep", eventTime(100), []int64{1, 2})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.Create(t.Context(), "u", "Reject", eventTime(200), tc.ids); err == nil {
				t.Fatal("invalid successor accepted")
			}
			if _, err := service.Update(t.Context(), "u", before.EventID, before.Version, event.UpdateInput{Activity: "Reject", StartedAt: before.StartedAt, SubjectIDs: tc.ids}); err == nil {
				t.Fatal("invalid replacement accepted")
			}
			after, err := service.Get(t.Context(), "u", before.EventID)
			if err != nil || !reflect.DeepEqual(after, before) {
				t.Fatalf("Event changed: got %+v, %v; want %+v", after, err, before)
			}
			var count int
			if err := db.QueryRow(`
			    SELECT
			        count(*)
			    FROM events
			`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("Event count got %d, want 1", count)
			}
		})
	}
	t.Run("association failure rolls back predecessor closure and partial inserts", func(t *testing.T) {
		db, service := multiSubjectEventFixture(t)
		before, err := service.Create(t.Context(), "u", "Keep", eventTime(100), []int64{1})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`
		    CREATE TRIGGER reject_second_subject BEFORE INSERT ON event_subjects WHEN NEW.subject_id = 2 BEGIN
		    SELECT
		        RAISE (ABORT, 'synthetic association failure');

		    END
		`); err != nil {
			t.Fatal(err)
		}
		if _, err := service.Create(t.Context(), "u", "Reject", eventTime(200), []int64{1, 2}); err == nil {
			t.Fatal("injected failure not returned")
		}
		after, err := service.Get(t.Context(), "u", before.EventID)
		if err != nil || !reflect.DeepEqual(after, before) {
			t.Fatalf("predecessor got %+v,%v; want unchanged %+v", after, err, before)
		}
		var events, links int
		if err := db.QueryRow(`
		    SELECT
		        (
		            SELECT
		                count(*)
		            FROM events
		        ),
		        (
		            SELECT
		                count(*)
		            FROM event_subjects
		        )
		`).Scan(&events, &links); err != nil {
			t.Fatal(err)
		}
		if events != 1 || links != 1 {
			t.Fatalf("persisted Events/links got %d/%d, want 1/1", events, links)
		}
	})
	t.Run("stale and canceled replacements preserve every relationship", func(t *testing.T) {
		_, service := multiSubjectEventFixture(t)
		before, err := service.Create(t.Context(), "u", "Keep", eventTime(100), []int64{1, 2})
		if err != nil {
			t.Fatal(err)
		}
		input := event.UpdateInput{Activity: "Keep", StartedAt: before.StartedAt, SubjectIDs: []int64{3}}
		if _, err := service.Update(t.Context(), "u", before.EventID, before.Version+1, input); !errors.Is(err, data.ErrEventVersionConflict) {
			t.Fatalf("stale error got %v, want version conflict", err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := service.Update(ctx, "u", before.EventID, before.Version, input); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled error got %v, want cancellation", err)
		}
		after, err := service.Get(t.Context(), "u", before.EventID)
		if err != nil || !reflect.DeepEqual(after, before) {
			t.Fatalf("Event got %+v, %v; want unchanged %+v", after, err, before)
		}
	})
	t.Run("deleting Subjects preserves remaining sets and full overlapping time totals", func(t *testing.T) {
		db, service := multiSubjectEventFixture(t)
		before, err := service.CreatePast(t.Context(), "u", "Keep", eventTime(100), eventTime(190), []int64{1, 2, 3})
		if err != nil {
			t.Fatal(err)
		}
		totals, err := data.NewSQLiteMetricsStore(db).SubjectTimeInRange(t.Context(), "u", eventTime(0), eventTime(300), eventTime(300))
		if err != nil || len(totals) != 3 {
			t.Fatalf("totals got %+v,%v; want three", totals, err)
		}
		for _, total := range totals {
			if total.TimeSpentSeconds != 90 {
				t.Fatalf("Subject %d got %d seconds, want 90", total.SubjectID, total.TimeSpentSeconds)
			}
		}
		subjects := data.NewSQLiteSubjectStore(db)
		for index, id := range []int64{2, 1, 3} {
			if err := subjects.DeleteSubject(t.Context(), "u", id); err != nil {
				t.Fatal(err)
			}
			after, err := service.Get(t.Context(), "u", before.EventID)
			if err != nil {
				t.Fatal(err)
			}
			if len(after.SubjectIDs) != 2-index || after.Version != before.Version+int64(index)+1 || !after.StartedAt.Equal(before.StartedAt) || !after.EndedAt.Equal(*before.EndedAt) || *after.ActivityType != *before.ActivityType {
				t.Fatalf("after deletion got %+v, want unchanged interval and %d links", after, 2-index)
			}
		}
		var retained int
		if err := db.QueryRow(`
		    SELECT
		        count(*)
		    FROM event_subjects
		    WHERE event_id = ?
		        AND deleted_at IS NOT NULL
		`, before.EventID).Scan(&retained); err != nil {
			t.Fatal(err)
		}
		if retained != 3 {
			t.Fatalf("retained retired pairs got %d, want 3", retained)
		}
	})
}

func TestMultiSubjectEventNoOp_SQLite(t *testing.T) {
	t.Run("reordered identical sets preserve relationship rows and keep the full-update version contract", func(t *testing.T) {
		db, service := multiSubjectEventFixture(t)
		before, err := service.Create(t.Context(), "u", "Keep", eventTime(100), []int64{1, 2})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`
		    CREATE TRIGGER reject_relationship_rewrite BEFORE
		    UPDATE ON event_subjects BEGIN
		    SELECT
		        RAISE (ABORT, 'relationship row rewritten');

		    END
		`); err != nil {
			t.Fatal(err)
		}
		selected := []int64{2, 1}
		after, err := service.Update(t.Context(), "u", before.EventID, before.Version, event.UpdateInput{Activity: "Keep", StartedAt: before.StartedAt, SubjectIDs: selected})
		if err != nil {
			t.Fatal(err)
		}
		if after.Version != before.Version+1 || !slices.Equal(after.SubjectIDs, []int64{1, 2}) || !slices.Equal(selected, []int64{2, 1}) {
			t.Fatalf("updated Event got %+v, input %v; want one version increment, sorted output, unchanged input", after, selected)
		}
		if _, err := service.Update(t.Context(), "u", after.EventID, after.Version, event.UpdateInput{Activity: "Reject", StartedAt: after.StartedAt, SubjectIDs: nil}); err == nil {
			t.Fatal("injected retirement failure not returned")
		}
		persisted, err := service.Get(t.Context(), "u", after.EventID)
		if err != nil || !reflect.DeepEqual(persisted, after) {
			t.Fatalf("failed retirement changed Event: got %+v,%v; want %+v", persisted, err, after)
		}
	})
}
