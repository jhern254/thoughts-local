//go:build integration

package integration_test

import "testing"

func TestMultiSubjectEventSchema_SQLite(t *testing.T) {
	t.Run("retains one same-owner relationship for each Event and Subject pair", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u", "other")
		_, err := db.Exec(`
		    INSERT INTO subjects (subject_id, user_id, subject_name)
		    VALUES (1, 'u', 'Coding'),
		        (2, 'u', 'Research'),
		        (3, 'other', 'Foreign');
		    INSERT INTO events (event_id, user_id, started_at, ended_at) VALUES (1, 'u', 1, 2);
		    INSERT INTO event_subjects (event_id, subject_id, user_id)
		    VALUES (1, 1, 'u'),
		        (1, 2, 'u');
		`)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO event_subjects (event_id, subject_id, user_id) VALUES (1, 1, 'u')`); err == nil {
			t.Fatal("duplicate pair accepted")
		}
		if _, err := db.Exec(`INSERT INTO event_subjects (event_id, subject_id, user_id) VALUES (1, 3, 'u')`); err == nil {
			t.Fatal("foreign-owned Subject accepted")
		}
		if _, err := db.Exec(`UPDATE event_subjects SET deleted_at=unixepoch('now') WHERE event_id=1 AND subject_id=1`); err != nil {
			t.Fatal(err)
		}
		var retained, live int
		if err := db.QueryRow(`SELECT count(*),count(*) FILTER (WHERE deleted_at IS NULL) FROM event_subjects WHERE event_id=1`).Scan(&retained, &live); err != nil {
			t.Fatal(err)
		}
		if retained != 2 || live != 1 {
			t.Fatalf("relationships got retained=%d live=%d, want 2/1", retained, live)
		}
		if _, err := db.Exec(`UPDATE event_subjects SET deleted_at=NULL WHERE event_id=1 AND subject_id=1`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`DELETE FROM events WHERE event_id=1`); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(`SELECT count(*) FROM event_subjects`).Scan(&retained); err != nil {
			t.Fatal(err)
		}
		if retained != 0 {
			t.Fatalf("hard-delete retained %d relationships, want zero", retained)
		}
	})
	t.Run("Events schema has no single-Subject column", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM pragma_table_info('events') WHERE name='subject_id'`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("single-Subject columns got %d, want zero", count)
		}
	})
}
