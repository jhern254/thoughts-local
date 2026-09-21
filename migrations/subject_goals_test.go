package migrations

import (
	"os"
	"testing"
)

const subjectGoalFixture = `INSERT INTO users(user_id) VALUES ('owner'),('other');
 INSERT INTO subjects(subject_id,user_id,subject_name) VALUES (1,'owner','Coding'),(2,'other','Other');
 INSERT INTO goals(goal_id,user_id,goal_name,target_seconds) VALUES (1,'owner','Practice',60),(2,'other','Other',60);
 INSERT INTO subject_goals(subject_id,goal_id,user_id) VALUES (1,1,'owner');`

func TestMigrations_SubjectGoals(t *testing.T) {
	for _, tc := range []struct{ name, statement string }{
		{"rejects duplicate pairs", "INSERT INTO subject_goals VALUES (1,1,'owner')"},
		{"rejects foreign subjects", "INSERT INTO subject_goals VALUES (2,1,'owner')"},
		{"rejects foreign goals", "INSERT INTO subject_goals VALUES (1,2,'owner')"},
		{"rejects missing subjects", "INSERT INTO subject_goals VALUES (99,1,'owner')"},
		{"rejects missing goals", "INSERT INTO subject_goals VALUES (1,99,'owner')"},
		{"requires subject", "INSERT INTO subject_goals VALUES (NULL,1,'owner')"},
		{"requires goal", "INSERT INTO subject_goals VALUES (1,NULL,'owner')"},
		{"requires owner", "INSERT INTO subject_goals VALUES (1,1,NULL)"},
		{"rejects changing link owner", "UPDATE subject_goals SET user_id='other'"},
		{"rejects changing subject ownership alone", "UPDATE subjects SET user_id='other' WHERE subject_id=1"},
		{"rejects changing goal ownership alone", "UPDATE goals SET user_id='other' WHERE goal_id=1"},
		{"rejects changing to foreign subject", "UPDATE subject_goals SET subject_id=2"},
		{"rejects changing to foreign goal", "UPDATE subject_goals SET goal_id=2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openMigratedDatabase(t)
			if _, err := db.Exec(subjectGoalFixture); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.statement); err == nil {
				t.Fatal("write error: got nil, want constraint violation")
			}
			var count int
			if err := db.QueryRow("SELECT count(*) FROM subject_goals WHERE subject_id=1 AND goal_id=1 AND user_id='owner'").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("original link count: got %d, want 1", count)
			}
		})
	}

	for _, tc := range []struct{ name, statement string }{
		{"subject", "DELETE FROM subjects WHERE subject_id=1"},
		{"goal", "DELETE FROM goals WHERE goal_id=1"},
		{"owner", "DELETE FROM users WHERE user_id='owner'"},
	} {
		t.Run("hard deleting "+tc.name+" cascades only affected links", func(t *testing.T) {
			db := openMigratedDatabase(t)
			if _, err := db.Exec(subjectGoalFixture + "INSERT INTO subject_goals VALUES (2,2,'other');"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.statement); err != nil {
				t.Fatal(err)
			}
			var count, subject, goal int
			if err := db.QueryRow("SELECT count(*),subject_id,goal_id FROM subject_goals").Scan(&count, &subject, &goal); err != nil {
				t.Fatal(err)
			}
			if count != 1 || subject != 2 || goal != 2 {
				t.Fatalf("remaining links: got count=%d subject=%d goal=%d, want count=1 subject=2 goal=2", count, subject, goal)
			}
		})
	}

	t.Run("key updates cascade while preserving shared ownership", func(t *testing.T) {
		db := openMigratedDatabase(t)
		if _, err := db.Exec(subjectGoalFixture); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE subjects SET subject_id=10 WHERE subject_id=1;
 UPDATE goals SET goal_id=20 WHERE goal_id=1;
 UPDATE users SET user_id='renamed' WHERE user_id='owner';`); err != nil {
			t.Fatal(err)
		}
		var subject, goal int
		var owner string
		if err := db.QueryRow("SELECT subject_id,goal_id,user_id FROM subject_goals").Scan(&subject, &goal, &owner); err != nil {
			t.Fatal(err)
		}
		if subject != 10 || goal != 20 || owner != "renamed" {
			t.Fatalf("updated link: got (%d,%d,%q), want (10,20,renamed)", subject, goal, owner)
		}
	})

	t.Run("down and up preserve existing entities", func(t *testing.T) {
		db := openMigratedDatabase(t)
		if _, err := db.Exec(subjectGoalFixture); err != nil {
			t.Fatal(err)
		}
		for _, file := range []string{"000011_create_subject_goals.down.sql", "000011_create_subject_goals.up.sql"} {
			migration, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(string(migration)); err != nil {
				t.Fatal(err)
			}
		}
		var subjects, goals, links int
		if err := db.QueryRow("SELECT (SELECT count(*) FROM subjects),(SELECT count(*) FROM goals),(SELECT count(*) FROM subject_goals)").Scan(&subjects, &goals, &links); err != nil {
			t.Fatal(err)
		}
		if subjects != 2 || goals != 2 || links != 0 {
			t.Fatalf("counts after migration cycle: got subjects=%d goals=%d links=%d, want 2,2,0", subjects, goals, links)
		}
	})
}
