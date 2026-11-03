//go:build integration

package integration_test

import "testing"

func TestGoalEventLedgerWorkflow_SQLite(t *testing.T) {
	t.Run("ledger starts empty without backfilling completed Events", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM event_credit_revisions`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("revisions: got %d, want zero", count)
		}
	})
}

func TestGoalEventLedgerMaintenance_SQLite(t *testing.T) {
	for _, tc := range []struct {
		name, sql string
		rejected  bool
	}{
		{"rejects duplicate recipients", "INSERT INTO goal_event_credits (event_id,event_version,goal_id,user_id) VALUES (1,1,1,'u')", true},
		{"rejects transferring Event ownership", "INSERT INTO users (user_id) VALUES ('other'); UPDATE events SET user_id='other' WHERE event_id=1", true},
		{"rejects transferring Goal ownership", "INSERT INTO users (user_id) VALUES ('other'); UPDATE goals SET user_id='other' WHERE goal_id=1", true},
		{"rejects transferring Subject ownership", "INSERT INTO users (user_id) VALUES ('other'); UPDATE subjects SET user_id='other' WHERE subject_id=1", true},
		{"rejects interval overwrite", "UPDATE event_credit_revisions SET credited_end_at=300", true},
		{"rejects revision removal", "DELETE FROM event_credit_revisions", true},
		{"rejects recipient removal", "DELETE FROM goal_event_credits", true},
		{"rejects support removal", "DELETE FROM goal_event_credit_subjects", true},
		{"rejects support rewrite", "UPDATE goal_event_credit_subjects SET recorded_subject_id=2", true},
		{"renames Event key", "UPDATE events SET event_id=10 WHERE event_id=1", false},
		{"renames Goal key", "UPDATE goals SET goal_id=10 WHERE goal_id=1", false},
		{"renames Subject key", "UPDATE subjects SET subject_id=10 WHERE subject_id=1", false},
		{"renames User key", "UPDATE users SET user_id='renamed' WHERE user_id='u'", false},
		{"deletes Event", "DELETE FROM events WHERE event_id=1", false},
		{"deletes Goal", "DELETE FROM goals WHERE goal_id=1", false},
		{"deletes Subject", "DELETE FROM subjects WHERE subject_id=1", false},
		{"deletes User", "DELETE FROM users WHERE user_id='u'", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			_, err := db.Exec(`
    INSERT INTO users (user_id) VALUES ('u');
    INSERT INTO subjects (subject_id,user_id,subject_name) VALUES (1,'u','Coding');
    INSERT INTO goals (goal_id,user_id,goal_name,target_seconds) VALUES (1,'u','Practice',60);
    INSERT INTO events (event_id,user_id,started_at,ended_at) VALUES (1,'u',100,200);
    INSERT INTO event_subjects (event_id,subject_id,user_id) VALUES (1,1,'u');
    INSERT INTO subject_goals (subject_id,goal_id,user_id,goal_number) VALUES (1,1,'u',1);
    INSERT INTO event_credit_revisions (event_id,event_version,user_id,credited_start_at,credited_end_at,reason) VALUES (1,1,'u',100,200,'completion');
    INSERT INTO goal_event_credits (event_id,event_version,goal_id,user_id) VALUES (1,1,1,'u');
    INSERT INTO goal_event_credit_subjects (event_id,event_version,goal_id,user_id,recorded_subject_id,subject_id,attributed_at) VALUES (1,1,1,'u',1,1,100);
   `)
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.Exec(tc.sql)
			if (err != nil) != tc.rejected {
				t.Fatalf("maintenance error: got %v, want rejected=%t", err, tc.rejected)
			}
			if tc.name == "deletes Subject" {
				var n int
				if err := db.QueryRow(`SELECT COUNT(*) FROM goal_event_credit_subjects WHERE subject_id IS NULL AND recorded_subject_id=1`).Scan(&n); err != nil {
					t.Fatal(err)
				}
				if n != 1 {
					t.Fatalf("detached historical supports: got %d, want 1", n)
				}
			}
			rows, err := db.Query(`PRAGMA foreign_key_check`)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			if rows.Next() {
				t.Fatal("foreign key violation after maintenance")
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
