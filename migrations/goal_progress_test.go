package migrations

import "testing"

const goalProgressFixture = `
INSERT INTO users (user_id) VALUES ('owner'), ('other');
INSERT INTO subjects (subject_id,user_id,subject_name) VALUES (1,'owner','Work'),(2,'other','Other');
INSERT INTO goals (goal_id,user_id,goal_name,target_seconds) VALUES (1,'owner','Goal',60),(2,'other','Other',60);
INSERT INTO events (event_id,user_id,started_at,ended_at) VALUES (1,'owner',0,10),(2,'other',0,10);
INSERT INTO goal_progress (progress_id,event_id,goal_id,user_id,recorded_subject_id,subject_id)
VALUES (1,1,1,'owner',1,1);`

func TestGoalProgressOwnershipWorkflow_SQLite(t *testing.T) {
	for _, tc := range []struct{ name, statement string }{
		{"rejects a foreign Event", "UPDATE goal_progress SET event_id = 2 WHERE progress_id = 1"},
		{"rejects a foreign Goal", "UPDATE goal_progress SET goal_id = 2 WHERE progress_id = 1"},
		{"rejects a foreign Subject", "UPDATE goal_progress SET subject_id = 2 WHERE progress_id = 1"},
		{"rejects transferring an Event", "UPDATE events SET user_id = 'other' WHERE event_id = 1"},
		{"rejects transferring a Goal", "UPDATE goals SET user_id = 'other' WHERE goal_id = 1"},
		{"rejects transferring a Subject", "UPDATE subjects SET user_id = 'other' WHERE subject_id = 1"},
		{"rejects duplicate live contribution", "INSERT INTO goal_progress (event_id,goal_id,user_id,recorded_subject_id,subject_id) VALUES (1,1,'owner',1,1)"},
		{"requires an Event", "UPDATE goal_progress SET event_id = NULL WHERE progress_id = 1"},
		{"rejects a missing Event", "UPDATE goal_progress SET event_id = 99 WHERE progress_id = 1"},
		{"rejects a missing Goal", "UPDATE goal_progress SET goal_id = 99 WHERE progress_id = 1"},
		{"rejects a missing Subject", "UPDATE goal_progress SET subject_id = 99 WHERE progress_id = 1"},
		{"rejects changing contribution owner", "UPDATE goal_progress SET user_id = 'other' WHERE progress_id = 1"},
		{"requires recorded identity", "UPDATE goal_progress SET recorded_subject_id = NULL WHERE progress_id = 1"},
		{"requires integer recorded identity", "UPDATE goal_progress SET recorded_subject_id = 1.5 WHERE progress_id = 1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openMigratedDatabase(t)
			if _, err := db.Exec(goalProgressFixture); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.statement); err == nil {
				t.Fatal("invalid contribution mutation: got nil, want constraint error")
			}
			var count int
			if err := db.QueryRow(`
    SELECT COUNT(*) FROM goal_progress
    WHERE progress_id = 1 AND event_id = 1 AND goal_id = 1
        AND user_id = 'owner' AND subject_id = 1 AND recorded_subject_id = 1`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("preserved contribution: got %d, want 1", count)
			}
		})
	}
}

func TestGoalProgressLifecycleWorkflow_SQLite(t *testing.T) {
	for _, tc := range []struct {
		name, statement   string
		rows              int
		subject, recorded int64
		owner             string
		event, goal       int64
	}{
		{"detaches hard deleted Subjects", "DELETE FROM subjects WHERE subject_id = 1", 1, 0, 1, "owner", 1, 1},
		{"cascades Event deletion", "DELETE FROM events WHERE event_id = 1", 0, 0, 0, "", 0, 0},
		{"cascades Goal deletion", "DELETE FROM goals WHERE goal_id = 1", 0, 0, 0, "", 0, 0},
		{"cascades owner deletion", "DELETE FROM users WHERE user_id = 'owner'", 0, 0, 0, "", 0, 0},
		{"retains soft deleted endpoints", "UPDATE subjects SET deleted_at = updated_at; UPDATE goals SET deleted_at = updated_at; UPDATE events SET deleted_at = updated_at", 1, 1, 1, "owner", 1, 1},
		{"renames an owner", "UPDATE users SET user_id = 'renamed' WHERE user_id = 'owner'", 1, 1, 1, "renamed", 1, 1},
		{"renames an Event key", "UPDATE events SET event_id = 3 WHERE event_id = 1", 1, 1, 1, "owner", 3, 1},
		{"renames a Goal key", "UPDATE goals SET goal_id = 3 WHERE goal_id = 1", 1, 1, 1, "owner", 1, 3},
		{"renames a Subject key without changing recorded identity", "UPDATE subjects SET subject_id = 3 WHERE subject_id = 1", 1, 3, 1, "owner", 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openMigratedDatabase(t)
			if _, err := db.Exec(goalProgressFixture); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.statement); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := db.QueryRow("SELECT COUNT(*) FROM goal_progress").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != tc.rows {
				t.Fatalf("contribution rows: got %d, want %d", count, tc.rows)
			}
			if count > 0 {
				var subject, recorded, event, goal int64
				var owner string
				if err := db.QueryRow("SELECT COALESCE(subject_id,0),recorded_subject_id,user_id,event_id,goal_id FROM goal_progress WHERE progress_id = 1").Scan(&subject, &recorded, &owner, &event, &goal); err != nil {
					t.Fatal(err)
				}
				if subject != tc.subject || recorded != tc.recorded || owner != tc.owner || event != tc.event || goal != tc.goal {
					t.Fatalf("contribution: got %d/%d/%s/%d/%d, want %+v", subject, recorded, owner, event, goal, tc)
				}
			}
			if err := db.QueryRow("SELECT COUNT(*) FROM pragma_foreign_key_check").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("foreign-key violations: got %d, want 0", count)
			}
		})
	}
	t.Run("recreates the complete migrated schema", func(t *testing.T) {
		db := openMigratedDatabase(t)
		if _, err := db.Exec(goalProgressFixture); err != nil {
			t.Fatal(err)
		}
		applyMigrationFiles(t, db, "*.down.sql", true)
		applyMigrationFiles(t, db, "*.up.sql", false)
		if _, err := db.Exec(goalProgressFixture); err != nil {
			t.Fatal(err)
		}
	})
}
