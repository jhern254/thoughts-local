package migrations

import (
	"os"
	"testing"
)

func TestMigrations_EventSubjects(t *testing.T) {
	t.Run("independent down and up preserve Event and Subject endpoints", func(t *testing.T) {
		db := openMigratedDatabase(t)
		insertUsers(t, db)
		if _, err := db.Exec(`
		    INSERT INTO subjects (subject_id, user_id, subject_name) VALUES (1, 'user-1', 'Coding');
		    INSERT INTO events (event_id, user_id, started_at, ended_at) VALUES (1, 'user-1', 100, 200);
		    INSERT INTO event_subjects (event_id, subject_id, user_id) VALUES (1, 1, 'user-1');
		`); err != nil {
			t.Fatal(err)
		}
		down, err := os.ReadFile("000015_create_event_subjects.down.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(down)); err != nil {
			t.Fatal(err)
		}
		var events, subjects, relationships int
		if err := db.QueryRow(`
      SELECT
          (SELECT count(*) FROM events WHERE event_id = 1),
          (SELECT count(*) FROM subjects WHERE subject_id = 1),
          (SELECT count(*) FROM sqlite_schema WHERE name = 'event_subjects')
  `).Scan(&events, &subjects, &relationships); err != nil {
			t.Fatal(err)
		}
		if events != 1 || subjects != 1 || relationships != 0 {
			t.Fatalf("after down got Events/Subjects/relationship tables %d/%d/%d, want 1/1/0", events, subjects, relationships)
		}
		up, err := os.ReadFile("000015_create_event_subjects.up.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(up)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO event_subjects (event_id, subject_id, user_id) VALUES (1, 1, 'user-1')`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(up)); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(`SELECT count(*) FROM event_subjects`).Scan(&relationships); err != nil {
			t.Fatal(err)
		}
		if relationships != 1 {
			t.Fatalf("repeat setup relationships got %d, want 1", relationships)
		}
		rows, err := db.Query("PRAGMA foreign_key_check")
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		if rows.Next() {
			t.Fatal("foreign_key_check returned a violation")
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
	})
}
