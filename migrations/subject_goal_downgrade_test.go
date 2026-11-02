package migrations

import (
	"database/sql"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestMigrations_SubjectGoalDowngrade(t *testing.T) {
	migration, err := os.ReadFile("000015_soft_delete_subject_goals.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	t.Run("rejects retired relationships and rolls back schema and data", func(t *testing.T) {
		db := openMigratedDatabase(t)
		if _, err := db.Exec(subjectGoalFixture + `
   INSERT INTO goals(goal_id,user_id,goal_name,target_seconds) VALUES(3,'owner','Retired',60);
   INSERT INTO subject_goals(subject_id,goal_id,user_id,goal_number,deleted_at) VALUES(1,3,'owner',7,17);`); err != nil {
			t.Fatal(err)
		}
		before := subjectGoalDowngradeSchema(t, db)
		tx, err := db.BeginTx(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		_, migrationErr := tx.ExecContext(t.Context(), string(migration))
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
		if migrationErr == nil {
			t.Fatal("downgrade: got nil error, want rejection of retired relationships")
		}
		if !strings.Contains(migrationErr.Error(), "cannot_downgrade_retired_subject_goals") {
			t.Fatalf("downgrade: got %v, want named retirement guard rejection", migrationErr)
		}
		if after := subjectGoalDowngradeSchema(t, db); !reflect.DeepEqual(after, before) {
			t.Fatalf("schema after rollback: got %v, want %v", after, before)
		}
		rows, err := db.Query(`SELECT subject_id,goal_id,user_id,goal_number,deleted_at FROM subject_goals ORDER BY goal_id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		type relationship struct {
			subject, goal int64
			owner         string
			number        int64
			retired       sql.NullInt64
		}
		var got []relationship
		for rows.Next() {
			var row relationship
			if err := rows.Scan(&row.subject, &row.goal, &row.owner, &row.number, &row.retired); err != nil {
				t.Fatal(err)
			}
			got = append(got, row)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		want := []relationship{{1, 1, "owner", 1, sql.NullInt64{}}, {1, 3, "owner", 7, sql.NullInt64{Int64: 17, Valid: true}}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("relationships after rollback: got %+v, want %+v", got, want)
		}
	})
	t.Run("downgrades unretired relationships to the original schema", func(t *testing.T) {
		db := openMigratedDatabase(t)
		if _, err := db.Exec(subjectGoalFixture); err != nil {
			t.Fatal(err)
		}
		tx, err := db.BeginTx(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(t.Context(), string(migration)); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		rows, err := db.Query("SELECT name FROM pragma_table_info('subject_goals') ORDER BY cid")
		if err != nil {
			t.Fatal(err)
		}
		var columns []string
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			columns = append(columns, name)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"subject_id", "goal_id", "user_id"}; !reflect.DeepEqual(columns, want) {
			t.Fatalf("legacy columns: got %v,want %v", columns, want)
		}
		var subject, goal int64
		var owner string
		if err := db.QueryRow("SELECT subject_id,goal_id,user_id FROM subject_goals").Scan(&subject, &goal, &owner); err != nil {
			t.Fatal(err)
		}
		if subject != 1 || goal != 1 || owner != "owner" {
			t.Fatalf("retained association: got (%d,%d,%q),want (1,1,owner)", subject, goal, owner)
		}
		var indexCount int
		if err := db.QueryRow("SELECT count(*) FROM sqlite_schema WHERE name='idx_subject_goals_goal_subject'").Scan(&indexCount); err != nil {
			t.Fatal(err)
		}
		if indexCount != 1 {
			t.Fatalf("legacy lookup index: got %d,want 1", indexCount)
		}
		check, err := db.Query("PRAGMA foreign_key_check")
		if err != nil {
			t.Fatal(err)
		}
		defer check.Close()
		if check.Next() {
			t.Fatal("foreign key check: got a violation,want none")
		}
		if err := check.Err(); err != nil {
			t.Fatal(err)
		}
	})
}

func subjectGoalDowngradeSchema(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT type,name,COALESCE(sql,'') FROM sqlite_schema
 UNION ALL SELECT type,name,COALESCE(sql,'') FROM sqlite_temp_schema ORDER BY type,name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var schema []string
	for rows.Next() {
		var kind, name, definition string
		if err := rows.Scan(&kind, &name, &definition); err != nil {
			t.Fatal(err)
		}
		schema = append(schema, kind+"\n"+name+"\n"+definition)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return schema
}
