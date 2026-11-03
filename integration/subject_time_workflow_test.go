//go:build integration

package integration_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

func TestSubjectTimeWorkflow_SQLite(t *testing.T) {
	t.Run("clips intervals groups assigned time and ranks totals", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u", "other")
		_, err := db.Exec(`
		    INSERT INTO
		        subjects (subject_id, user_id, subject_name)
		    VALUES
		        (1, 'u', 'Coding'),
		        (2, 'u', 'Reading'),
		        (3, 'u', 'Empty'),
		        (4, 'other', 'Private');

		    INSERT INTO
		        events (user_id, started_at, ended_at)
		    VALUES
		        ('u', 50, 150);

		    INSERT INTO
		        event_subjects (event_id, subject_id, user_id)
		    VALUES
		        (last_insert_rowid(), 1, 'u');

		    INSERT INTO
		        events (user_id, started_at, ended_at)
		    VALUES
		        ('u', 150, 250);

		    INSERT INTO
		        event_subjects (event_id, subject_id, user_id)
		    VALUES
		        (last_insert_rowid(), 1, 'u');

		    INSERT INTO
		        events (user_id, started_at, ended_at)
		    VALUES
		        ('u', 200, NULL);

		    INSERT INTO
		        event_subjects (event_id, subject_id, user_id)
		    VALUES
		        (last_insert_rowid(), 2, 'u');

		    INSERT INTO
		        events (user_id, started_at, ended_at)
		    VALUES
		        ('u', 180, 180);

		    INSERT INTO
		        event_subjects (event_id, subject_id, user_id)
		    VALUES
		        (last_insert_rowid(), 3, 'u');

		    INSERT INTO
		        events (user_id, started_at, ended_at)
		    VALUES
		        ('u', 110, 160);

		    INSERT INTO
		        events (user_id, started_at, ended_at)
		    VALUES
		        ('other', 100, 300);

		    INSERT INTO
		        event_subjects (event_id, subject_id, user_id)
		    VALUES
		        (last_insert_rowid(), 4, 'other');
		`)
		if err != nil {
			t.Fatal(err)
		}
		rows, err := data.NewSQLiteMetricsStore(db).SubjectTimeInRange(t.Context(), "u", eventTime(100), eventTime(300), eventTime(280))
		if err != nil {
			t.Fatal(err)
		}
		want := []data.SubjectTimeView{{SubjectID: 1, SubjectName: "Coding", TimeSpentSeconds: 150}, {SubjectID: 2, SubjectName: "Reading", TimeSpentSeconds: 80}}
		if !reflect.DeepEqual(rows, want) {
			t.Fatalf("subject time: got %+v, want %+v", rows, want)
		}
	})
	t.Run("excludes deleted records and orders equal totals by current name", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		_, err := db.Exec(`
		    INSERT INTO
		        subjects (subject_id, user_id, subject_name)
		    VALUES
		        (1, 'u', 'Zebra'),
		        (2, 'u', 'Alpha'),
		        (3, 'u', 'Deleted');

		    INSERT INTO
		        events (user_id, started_at, ended_at)
		    VALUES
		        ('u', 100, 150);

		    INSERT INTO
		        event_subjects (event_id, subject_id, user_id)
		    VALUES
		        (last_insert_rowid(), 1, 'u');

		    INSERT INTO
		        events (user_id, started_at, ended_at)
		    VALUES
		        ('u', 150, 200);

		    INSERT INTO
		        event_subjects (event_id, subject_id, user_id)
		    VALUES
		        (last_insert_rowid(), 2, 'u');

		    INSERT INTO
		        events (user_id, started_at, ended_at)
		    VALUES
		        ('u', 200, 250);

		    INSERT INTO
		        event_subjects (event_id, subject_id, user_id)
		    VALUES
		        (last_insert_rowid(), 3, 'u');

		    UPDATE subjects
		    SET deleted_at = unixepoch(),
		        updated_at = unixepoch()
		    WHERE subject_id = 3;

		    INSERT INTO
		        events (user_id, started_at, ended_at, deleted_at, updated_at)
		    VALUES
		        ('u', 250, 300, unixepoch(), unixepoch());

		    INSERT INTO
		        event_subjects (event_id, subject_id, user_id)
		    VALUES
		        (last_insert_rowid(), 1, 'u');
		`)
		if err != nil {
			t.Fatal(err)
		}
		store := data.NewSQLiteMetricsStore(db)
		rows, err := store.SubjectTimeInRange(t.Context(), "u", eventTime(100), eventTime(300), eventTime(300))
		if err != nil {
			t.Fatal(err)
		}
		want := []data.SubjectTimeView{{SubjectID: 2, SubjectName: "Alpha", TimeSpentSeconds: 50}, {SubjectID: 1, SubjectName: "Zebra", TimeSpentSeconds: 50}}
		if !reflect.DeepEqual(rows, want) {
			t.Fatalf("ranked subjects: got %+v, want %+v", rows, want)
		}
		if _, err := db.Exec(`
		    UPDATE users
		    SET deleted_at = unixepoch(),
		        updated_at = unixepoch()
		    WHERE user_id = 'u'
		`); err != nil {
			t.Fatal(err)
		}
		rows, err = store.SubjectTimeInRange(t.Context(), "u", eventTime(100), eventTime(300), eventTime(300))
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 0 {
			t.Fatalf("deleted owner's subjects: got %+v, want empty", rows)
		}
	})
	for _, date := range []struct {
		date  string
		hours int64
	}{{"2026-03-08", 23}, {"2026-11-01", 25}} {
		t.Run("uses calendar boundaries on "+date.date, func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			insertUsers(t, db, "u")
			zone, err := time.LoadLocation("America/Los_Angeles")
			if err != nil {
				t.Fatal(err)
			}
			from, err := time.ParseInLocation("2006-01-02", date.date, zone)
			if err != nil {
				t.Fatal(err)
			}
			until := from.AddDate(0, 0, 1)
			if _, err := db.Exec(`
			    INSERT INTO
			        subjects (subject_id, user_id, subject_name)
			    VALUES
			        (1, 'u', 'Day');

			    INSERT INTO
			        events (user_id, started_at, ended_at)
			    VALUES
			        ('u', ?1, ?2);

			    INSERT INTO
			        event_subjects (event_id, subject_id, user_id)
			    VALUES
			        (last_insert_rowid(), 1, 'u')
			`, from.Add(-time.Hour).Unix(), until.Add(time.Hour).Unix()); err != nil {
				t.Fatal(err)
			}
			rows, err := data.NewSQLiteMetricsStore(db).SubjectTimeInRange(t.Context(), "u", from, until, until)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || rows[0].TimeSpentSeconds != date.hours*3600 {
				t.Fatalf("day total: got %+v, want %d seconds", rows, date.hours*3600)
			}
		})
	}
}
