package migrations

import "testing"

func TestMigrations_SubjectGoalLifecycle(t *testing.T) {
	for _, tc := range []struct{ name, statement string }{
		{"requires a number", "INSERT INTO subject_goals(subject_id,goal_id,user_id) VALUES(1,3,'owner')"},
		{"rejects zero numbers", "INSERT INTO subject_goals(subject_id,goal_id,user_id,goal_number) VALUES(1,3,'owner',0)"},
		{"rejects fractional numbers", "INSERT INTO subject_goals(subject_id,goal_id,user_id,goal_number) VALUES(1,3,'owner',1.5)"},
		{"rejects overflow numbers", "INSERT INTO subject_goals(subject_id,goal_id,user_id,goal_number) VALUES(1,3,'owner',9223372036854775808)"},
		{"retired numbers cannot be reused", "UPDATE subject_goals SET deleted_at=1; INSERT INTO subject_goals(subject_id,goal_id,user_id,goal_number) VALUES(1,3,'owner',1)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openMigratedDatabase(t)
			if _, err := db.Exec(subjectGoalFixture + "INSERT INTO goals(goal_id,user_id,goal_name,target_seconds) VALUES(3,'owner','Another',60)"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.statement); err == nil {
				t.Fatal("write: got nil error, want constraint rejection")
			}
			var count int
			if err := db.QueryRow("SELECT count(*) FROM subject_goals").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("retained rows: got %d,want 1", count)
			}
		})
	}
}

func TestMigrations_RetiredSubjectGoalCascades(t *testing.T) {
	for _, endpoint := range []string{"subjects", "goals", "users"} {
		t.Run("hard deletion of "+endpoint+" removes retired relationships", func(t *testing.T) {
			db := openMigratedDatabase(t)
			if _, err := db.Exec(subjectGoalFixture + "UPDATE subject_goals SET deleted_at=unixepoch('now')"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("DELETE FROM " + endpoint + " WHERE user_id='owner'"); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := db.QueryRow("SELECT count(*) FROM subject_goals").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("remaining retired links: got %d,want 0", count)
			}
			rows, err := db.Query("PRAGMA foreign_key_check")
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			if rows.Next() {
				t.Fatal("foreign-key violation")
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
