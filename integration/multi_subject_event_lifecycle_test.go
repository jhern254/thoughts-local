//go:build integration

package integration_test

import (
	"github.com/jhern254/go-thoughts/internal/data"
	"slices"
	"testing"
	"time"
)

func TestSQLiteEventStore_MultipleSubjects(t *testing.T) {
	t.Run("creates retires reactivates and preserves Subjects when ending", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		if _, err := db.Exec(`
		    INSERT INTO users (user_id) VALUES ('u');INSERT INTO subjects (subject_id, user_id, subject_name)
		    VALUES (1, 'u', 'One'),
		        (2, 'u', 'Two');
		`); err != nil {
			t.Fatal(err)
		}
		store := data.NewSQLiteEventStore(db)
		now := time.Now().UTC().Truncate(time.Second)
		created, err := store.StartEvent(t.Context(), &data.Event{UserID: "u", SubjectIDs: []int64{1, 2}, StartedAt: now, CreatedAt: now, UpdatedAt: now})
		if err != nil {
			t.Fatal(err)
		}
		views, err := store.ListEventsWithSubjects(t.Context(), "u", now.Add(-time.Second), now.Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if len(views) != 1 || len(views[0].Subjects) != 2 {
			t.Fatalf("timeline views got %+v, want one Event with two Subjects", views)
		}
		reopened := data.NewSQLiteEventStore(openSQLite(t, dsn))
		read, err := reopened.GetEvent(t.Context(), "u", created.EventID)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(read.SubjectIDs, []int64{1, 2}) {
			t.Fatalf("Subjects got %v, want [1 2]", read.SubjectIDs)
		}
		read.SubjectIDs = []int64{2}
		updated, err := store.UpdateEvent(t.Context(), read)
		if err != nil {
			t.Fatal(err)
		}
		updated.SubjectIDs = []int64{1, 2}
		updated, err = store.UpdateEvent(t.Context(), updated)
		if err != nil {
			t.Fatal(err)
		}
		ended, err := store.EndEvent(t.Context(), "u", updated.EventID, updated.Version, now.Add(time.Second), now.Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(ended.SubjectIDs, []int64{1, 2}) {
			t.Fatalf("ended Subjects got %v, want [1 2]", ended.SubjectIDs)
		}
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM event_subjects WHERE event_id=?`, created.EventID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 2 {
			t.Fatalf("retained pairs got %d, want 2", count)
		}
	})
}
