package migrations

import (
	"database/sql"
	"testing"
)

const deletionFixture = `
INSERT INTO users(user_id, handle, email) VALUES ('u', 'local', 'local@example.com');
INSERT INTO subjects(subject_id,user_id,subject_name) VALUES (1,'u','name');
INSERT INTO events(event_id,user_id) VALUES (1,'u');
INSERT INTO thoughts(thought_id,user_id,subject_id,event_id,thought) VALUES (1,'u',1,1,'keep');
INSERT INTO tags(tag_id,user_id,tag_name) VALUES (1,'u','name');
INSERT INTO thought_tags(thought_id,tag_id) VALUES (1,1);
INSERT INTO goals(goal_id,user_id,goal_name,target_seconds) VALUES (1,'u','name',60);
INSERT INTO goal_progress(goal_id,user_id,event_id,thought_id,time_spent_sec) VALUES (1,'u',1,1,60);`

func TestSoftDeletionWorkflow_SQLite(t *testing.T) {
	for _, table := range []string{"users", "subjects", "events", "thoughts", "tags", "goals"} {
		t.Run(table+" retains records and validates deletion time", func(t *testing.T) {
			db := openMigratedDatabase(t)
			if _, err := db.Exec(deletionFixture); err != nil {
				t.Fatalf("got error %v, want nil", err)
			}
			var deleted sql.NullInt64
			if err := db.QueryRow("SELECT deleted_at FROM " + table).Scan(&deleted); err != nil {
				t.Fatalf("got error %v, want nil", err)
			}
			if deleted.Valid {
				t.Fatalf("got deletion timestamp %d, want NULL", deleted.Int64)
			}
			for _, value := range []string{"created_at-1", "updated_at+1", "'invalid'", "created_at+0.5"} {
				if _, err := db.Exec("UPDATE " + table + " SET deleted_at=" + value); err == nil {
					t.Fatalf("got deletion timestamp update error %v for %s, want constraint error", err, value)
				}
			}
			if _, err := db.Exec("UPDATE " + table + " SET deleted_at=updated_at"); err != nil {
				t.Fatalf("got error %v, want nil", err)
			}
			var subjectID, eventID, thoughtID, seconds int64
			if err := db.QueryRow(`SELECT t.subject_id,p.event_id,p.thought_id,p.time_spent_sec FROM thoughts t JOIN goal_progress p ON p.thought_id=t.thought_id JOIN thought_tags tt ON tt.thought_id=t.thought_id`).Scan(&subjectID, &eventID, &thoughtID, &seconds); err != nil {
				t.Fatalf("got error %v, want nil", err)
			}
			if got, want := subjectID, int64(1); got != want {
				t.Fatalf("got retained subject ID %d, want %d", got, want)
			}
			if got, want := eventID, int64(1); got != want {
				t.Fatalf("got retained event ID %d, want %d", got, want)
			}
			if got, want := thoughtID, int64(1); got != want {
				t.Fatalf("got retained thought ID %d, want %d", got, want)
			}
			if got, want := seconds, int64(60); got != want {
				t.Fatalf("got progress seconds %d, want %d", got, want)
			}
			var count int
			if err := db.QueryRow("SELECT count(*) FROM pragma_foreign_key_check").Scan(&count); err != nil {
				t.Fatalf("got query error %v, want nil", err)
			}
			if got, want := count, 0; got != want {
				t.Fatalf("got foreign-key violation count %d, want %d", got, want)
			}
		})

	}
	for _, tc := range []struct{ table, column string }{{"subjects", "subject_name"}, {"tags", "tag_name"}, {"goals", "goal_name"}} {
		t.Run(tc.table+" allows reuse only after deletion", func(t *testing.T) {
			db := openMigratedDatabase(t)
			if _, err := db.Exec(deletionFixture); err != nil {
				t.Fatalf("got error %v, want nil", err)
			}
			columns, values := "user_id,"+tc.column, "'u','name'"
			if tc.table == "goals" {
				columns += ",target_seconds"
				values += ",60"
			}
			insert := "INSERT INTO " + tc.table + " (" + columns + ") VALUES (" + values + ")"
			if _, err := db.Exec(insert); err == nil {
				t.Fatalf("got name uniqueness error %v, want constraint error", err)
			}
			if _, err := db.Exec("UPDATE " + tc.table + " SET deleted_at=updated_at"); err != nil {
				t.Fatalf("got error %v, want nil", err)
			}
			if _, err := db.Exec(insert); err != nil {
				t.Fatalf("got error %v, want nil", err)
			}
			if _, err := db.Exec(insert); err == nil {
				t.Fatalf("got name uniqueness error %v, want constraint error", err)
			}
			if _, err := db.Exec("UPDATE " + tc.table + " SET deleted_at=NULL"); err == nil {
				t.Fatalf("got name uniqueness error %v, want constraint error", err)
			}
		})
	}
	t.Run("rebuilds a populated database from the original migrations", func(t *testing.T) {
		db := openMigratedDatabase(t)
		if _, err := db.Exec(deletionFixture); err != nil {
			t.Fatalf("got fixture error %v, want nil", err)
		}
		if _, err := db.Exec("UPDATE subjects SET deleted_at=updated_at"); err != nil {
			t.Fatalf("got deletion error %v, want nil", err)
		}
		applyMigrationFiles(t, db, "*.down.sql", true)
		applyMigrationFiles(t, db, "*.up.sql", false)
		var count int
		if err := db.QueryRow("SELECT count(*) FROM subjects").Scan(&count); err != nil {
			t.Fatalf("got query error %v, want nil", err)
		}
		if got, want := count, 0; got != want {
			t.Fatalf("got rebuilt subject count %d, want %d", got, want)
		}
		if _, err := db.Exec(deletionFixture); err != nil {
			t.Fatalf("got rebuilt fixture error %v, want nil", err)
		}
		if err := db.QueryRow("SELECT count(*) FROM subjects WHERE deleted_at IS NULL").Scan(&count); err != nil {
			t.Fatalf("got query error %v, want nil", err)
		}
		if got, want := count, 1; got != want {
			t.Fatalf("got undeleted subject count %d, want %d", got, want)
		}
	})
}
