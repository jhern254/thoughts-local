package migrations

import (
	"database/sql"
	"os"
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
				t.Fatal(err)
			}
			var deleted sql.NullInt64
			if err := db.QueryRow("SELECT deleted_at FROM " + table).Scan(&deleted); err != nil {
				t.Fatal(err)
			}
			if deleted.Valid {
				t.Fatal("new row is deleted")
			}
			for _, value := range []string{"created_at-1", "updated_at+1", "'invalid'", "created_at+0.5"} {
				if _, err := db.Exec("UPDATE " + table + " SET deleted_at=" + value); err == nil {
					t.Fatalf("accepted deletion time %s", value)
				}
			}
			if _, err := db.Exec("UPDATE " + table + " SET deleted_at=updated_at"); err != nil {
				t.Fatal(err)
			}
			var subjectID, eventID, thoughtID, seconds int64
			if err := db.QueryRow(`SELECT t.subject_id,p.event_id,p.thought_id,p.time_spent_sec FROM thoughts t JOIN goal_progress p ON p.thought_id=t.thought_id JOIN thought_tags tt ON tt.thought_id=t.thought_id`).Scan(&subjectID, &eventID, &thoughtID, &seconds); err != nil {
				t.Fatal(err)
			}
			if subjectID != 1 || eventID != 1 || thoughtID != 1 || seconds != 60 {
				t.Fatal("historical relationships changed")
			}
			var count int
			if err := db.QueryRow("SELECT count(*) FROM pragma_foreign_key_check").Scan(&count); err != nil || count != 0 {
				t.Fatalf("foreign keys: count=%d err=%v", count, err)
			}
		})
		t.Run(table+" blocks rollback after deletion", func(t *testing.T) {
			db := openMigratedDatabase(t)
			if _, err := db.Exec(deletionFixture); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("UPDATE " + table + " SET deleted_at=updated_at"); err != nil {
				t.Fatal(err)
			}
			migration, err := os.ReadFile("000009_soft_deletion.down.sql")
			if err != nil {
				t.Fatal(err)
			}
			tx, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			if _, err = tx.Exec(string(migration)); err == nil {
				t.Fatal("rollback accepted deleted data")
			}
			if err := tx.Rollback(); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := db.QueryRow("SELECT count(*) FROM " + table + " WHERE deleted_at IS NOT NULL").Scan(&count); err != nil || count != 1 {
				t.Fatalf("lost deletion marker: %d, %v", count, err)
			}
		})
	}
	for _, tc := range []struct{ table, column string }{{"subjects", "subject_name"}, {"tags", "tag_name"}, {"goals", "goal_name"}} {
		t.Run(tc.table+" allows reuse only after deletion", func(t *testing.T) {
			db := openMigratedDatabase(t)
			if _, err := db.Exec(deletionFixture); err != nil {
				t.Fatal(err)
			}
			columns, values := "user_id,"+tc.column, "'u','name'"
			if tc.table == "goals" {
				columns += ",target_seconds"
				values += ",60"
			}
			insert := "INSERT INTO " + tc.table + " (" + columns + ") VALUES (" + values + ")"
			if _, err := db.Exec(insert); err == nil {
				t.Fatal("accepted duplicate active name")
			}
			if _, err := db.Exec("UPDATE " + tc.table + " SET deleted_at=updated_at"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(insert); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(insert); err == nil {
				t.Fatal("accepted second active duplicate")
			}
			if _, err := db.Exec("UPDATE " + tc.table + " SET deleted_at=NULL"); err == nil {
				t.Fatal("restored conflicting name")
			}
		})
	}
	t.Run("preserves existing data during upgrade and unused rollback", func(t *testing.T) {
		db := openMigratedDatabase(t)
		applyMigrationFiles(t, db, "000009*.down.sql", false)
		if _, err := db.Exec(deletionFixture); err != nil {
			t.Fatal(err)
		}
		applyMigrationFiles(t, db, "000009*.up.sql", false)
		var count int
		if err := db.QueryRow("SELECT count(*) FROM thoughts WHERE subject_id=1 AND event_id=1 AND deleted_at IS NULL").Scan(&count); err != nil || count != 1 {
			t.Fatalf("upgrade changed data: %d %v", count, err)
		}
		applyMigrationFiles(t, db, "000009*.down.sql", false)
		if _, err := db.Exec("INSERT INTO subjects(user_id,subject_name) VALUES ('u','name')"); err == nil {
			t.Fatal("rollback lost uniqueness")
		}
		applyMigrationFiles(t, db, "000009*.up.sql", false)
	})
}
