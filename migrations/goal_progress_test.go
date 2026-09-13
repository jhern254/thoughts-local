package migrations

import (
	"errors"
	"testing"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const goalProgressFixture = `
INSERT INTO users(user_id) VALUES ('owner'), ('other');
INSERT INTO goals(goal_id,user_id,goal_name,target_seconds) VALUES
    (1,'owner','Goal',60), (2,'other','Other goal',60);
INSERT INTO events(event_id,user_id,started_at,ended_at) VALUES
    (1,'owner',0,10), (2,'other',0,10), (3,'owner',10,20);
INSERT INTO thoughts(thought_id,user_id,event_id,thought) VALUES
    (1,'owner',3,'Thought'), (2,'other',2,'Other thought');`

// Thought 1 references a different same-owner event: progress provenance IDs
// are independent, and neither implies the other.
const goalProgressEntry = `INSERT INTO goal_progress
    (progress_id,goal_id,user_id,event_id,thought_id,time_spent_sec,progress_note,occurred_at,created_at)
    VALUES (1,1,'owner',1,1,60,'Retained provenance',100,200);`

func TestGoalProgressOwnershipWorkflow_SQLite(t *testing.T) {
	t.Run("rebuilds populated migrations with ownership enforcement", func(t *testing.T) {
		db := openMigratedDatabase(t)
		if _, err := db.Exec(goalProgressFixture + goalProgressEntry); err != nil {
			t.Fatal(err)
		}
		applyMigrationFiles(t, db, "*.down.sql", true)
		applyMigrationFiles(t, db, "*.up.sql", false)
		if _, err := db.Exec(goalProgressFixture + goalProgressEntry); err != nil {
			t.Fatalf("recreate valid progress: got %v, want nil", err)
		}
		_, err := db.Exec("UPDATE goal_progress SET event_id=2 WHERE progress_id=1")
		var cause *sqlite.Error
		if !errors.As(err, &cause) || cause.Code() != sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY {
			t.Fatalf("rebuilt ownership constraint: got %v, want SQLite foreign-key error", err)
		}
	})

	for _, tc := range []struct{ name, event, thought string }{
		{"allows progress without provenance", "NULL", "NULL"},
		{"allows event provenance", "1", "NULL"},
		{"allows thought provenance", "NULL", "1"},
		{"allows independent event and thought provenance", "1", "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openMigratedDatabase(t)
			if _, err := db.Exec(goalProgressFixture); err != nil {
				t.Fatal(err)
			}
			_, err := db.Exec(`INSERT INTO goal_progress(goal_id,user_id,event_id,thought_id,time_spent_sec)
                VALUES (1,'owner',` + tc.event + `,` + tc.thought + `,60)`)
			if err != nil {
				t.Fatalf("create progress: got %v, want nil", err)
			}
		})
	}

	for _, tc := range []struct{ name, statement string }{
		{"rejects another users goal", "INSERT INTO goal_progress(goal_id,user_id,time_spent_sec) VALUES (2,'owner',60)"},
		{"rejects another users event", "INSERT INTO goal_progress(goal_id,user_id,event_id,time_spent_sec) VALUES (1,'owner',2,60)"},
		{"rejects another users thought", "INSERT INTO goal_progress(goal_id,user_id,thought_id,time_spent_sec) VALUES (1,'owner',2,60)"},
		{"rejects missing goal", "INSERT INTO goal_progress(goal_id,user_id,time_spent_sec) VALUES (99,'owner',60)"},
		{"rejects missing event", "INSERT INTO goal_progress(goal_id,user_id,event_id,time_spent_sec) VALUES (1,'owner',99,60)"},
		{"rejects missing thought", "INSERT INTO goal_progress(goal_id,user_id,thought_id,time_spent_sec) VALUES (1,'owner',99,60)"},
		{"rejects changing progress owner", "UPDATE goal_progress SET user_id='other' WHERE progress_id=1"},
		{"rejects changing to another users goal", "UPDATE goal_progress SET goal_id=2 WHERE progress_id=1"},
		{"rejects changing to another users event", "UPDATE goal_progress SET event_id=2 WHERE progress_id=1"},
		{"rejects changing to another users thought", "UPDATE goal_progress SET thought_id=2 WHERE progress_id=1"},
		{"rejects transferring referenced goal", "UPDATE goals SET user_id='other' WHERE goal_id=1"},
		{"rejects transferring referenced event", "UPDATE events SET user_id='other' WHERE event_id=1"},
		{"rejects transferring referenced thought", "UPDATE thoughts SET user_id='other' WHERE thought_id=1"},
		{"rolls back an entire mixed ownership insert", "INSERT INTO goal_progress(goal_id,user_id,time_spent_sec) VALUES (1,'owner',30),(2,'owner',60)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openMigratedDatabase(t)
			if _, err := db.Exec(goalProgressFixture + goalProgressEntry); err != nil {
				t.Fatal(err)
			}
			_, err := db.Exec(tc.statement)
			var cause *sqlite.Error
			if !errors.As(err, &cause) {
				t.Fatalf("ownership constraint: got %v, want SQLite foreign-key error", err)
			}
			if got, want := cause.Code(), sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY; got != want {
				t.Fatalf("constraint code: got %d, want %d", got, want)
			}
			var preserved bool
			err = db.QueryRow(`SELECT goal_id=1 AND user_id='owner' AND event_id=1 AND thought_id=1
                AND time_spent_sec=60 AND progress_note='Retained provenance' AND occurred_at=100 AND created_at=200
                FROM goal_progress WHERE progress_id=1`).Scan(&preserved)
			if err != nil {
				t.Fatal(err)
			}
			if !preserved {
				t.Fatal("original progress preserved: got false, want true")
			}
			var count int
			if err := db.QueryRow("SELECT count(*) FROM goal_progress").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if got, want := count, 1; got != want {
				t.Fatalf("progress rows after rejection: got %d, want %d", got, want)
			}
			if err := db.QueryRow("SELECT count(*) FROM pragma_foreign_key_check").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if got, want := count, 0; got != want {
				t.Fatalf("foreign-key violations: got %d, want %d", got, want)
			}
		})
	}
}

func TestGoalProgressHistoryWorkflow_SQLite(t *testing.T) {
	for _, tc := range []struct {
		name, statement, owner string
		goal, event, thought   int64
	}{
		{"renames the user across all references", "UPDATE users SET user_id='renamed' WHERE user_id='owner'", "renamed", 1, 1, 1},
		{"renames a referenced goal ID", "UPDATE goals SET goal_id=4 WHERE goal_id=1", "owner", 4, 1, 1},
		{"renames a referenced event ID", "UPDATE events SET event_id=4 WHERE event_id=1", "owner", 1, 4, 1},
		{"renames a referenced thought ID", "UPDATE thoughts SET thought_id=4 WHERE thought_id=1", "owner", 1, 1, 4},
		{"hard deleting an event clears only its provenance", "DELETE FROM events WHERE event_id=1", "owner", 1, 0, 1},
		{"hard deleting a thought clears only its provenance", "DELETE FROM thoughts WHERE thought_id=1", "owner", 1, 1, 0},
		{"clears both optional references without losing progress", "DELETE FROM events WHERE event_id=1; DELETE FROM thoughts WHERE thought_id=1", "owner", 1, 0, 0},
		{"soft deleting the goal retains history", "UPDATE goals SET deleted_at=updated_at WHERE goal_id=1", "owner", 1, 1, 1},
		{"soft deleting the event retains provenance", "UPDATE events SET deleted_at=updated_at WHERE event_id=1", "owner", 1, 1, 1},
		{"soft deleting the thought retains provenance", "UPDATE thoughts SET deleted_at=updated_at WHERE thought_id=1", "owner", 1, 1, 1},
		{"soft deleting the user retains history", "UPDATE users SET deleted_at=updated_at WHERE user_id='owner'", "owner", 1, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openMigratedDatabase(t)
			if _, err := db.Exec(goalProgressFixture + goalProgressEntry); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.statement); err != nil {
				t.Fatalf("change referenced record: got %v, want nil", err)
			}
			var owner string
			var goal, event, thought int64
			var preserved bool
			err := db.QueryRow(`SELECT user_id,goal_id,coalesce(event_id,0),coalesce(thought_id,0),
                time_spent_sec=60 AND progress_note='Retained provenance' AND occurred_at=100 AND created_at=200
                FROM goal_progress WHERE progress_id=1`).Scan(&owner, &goal, &event, &thought, &preserved)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := owner, tc.owner; got != want {
				t.Fatalf("progress owner: got %q, want %q", got, want)
			}
			if got, want := goal, tc.goal; got != want {
				t.Fatalf("progress goal: got %d, want %d", got, want)
			}
			if got, want := event, tc.event; got != want {
				t.Fatalf("progress event (0 means NULL): got %d, want %d", got, want)
			}
			if got, want := thought, tc.thought; got != want {
				t.Fatalf("progress thought (0 means NULL): got %d, want %d", got, want)
			}
			if !preserved {
				t.Fatal("progress contribution and metadata preserved: got false, want true")
			}
			var violations int
			if err := db.QueryRow("SELECT count(*) FROM pragma_foreign_key_check").Scan(&violations); err != nil {
				t.Fatal(err)
			}
			if got, want := violations, 0; got != want {
				t.Fatalf("foreign-key violations: got %d, want %d", got, want)
			}
		})
	}
	for _, tc := range []struct{ name, statement string }{
		{"hard deleting a goal removes its progress", "DELETE FROM goals WHERE goal_id=1"},
		{"hard deleting a user removes their progress", "DELETE FROM users WHERE user_id='owner'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openMigratedDatabase(t)
			if _, err := db.Exec(goalProgressFixture + goalProgressEntry); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.statement); err != nil {
				t.Fatalf("hard delete: got %v, want nil", err)
			}
			var count int
			if err := db.QueryRow("SELECT count(*) FROM goal_progress").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if got, want := count, 0; got != want {
				t.Fatalf("remaining progress rows: got %d, want %d", got, want)
			}
		})
	}
}
