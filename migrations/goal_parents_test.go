package migrations

import (
	"database/sql"
	"errors"
	"testing"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const goalParentFixture = `
INSERT INTO users (user_id) VALUES ('owner'), ('other');
INSERT INTO goals (goal_id, user_id, goal_name, target_seconds) VALUES
    (1, 'owner', 'Wellbeing', 600), (2, 'owner', 'Fitness', 600),
    (3, 'owner', 'Outdoors', 600), (4, 'owner', 'Running', 600),
    (5, 'owner', 'Another parent', 600), (6, 'other', 'Private goal', 600);`

const goalParentDiamond = `INSERT INTO goal_parents (goal_id, parent_goal_id, user_id)
    VALUES (2, 1, 'owner'), (3, 1, 'owner'), (4, 2, 'owner'), (4, 3, 'owner');`

func TestGoalParentsWorkflow_SQLite(t *testing.T) {
	t.Run("allows multiple parents and levels", func(t *testing.T) {
		db := openMigratedDatabase(t)
		if _, err := db.Exec(goalParentFixture + goalParentDiamond); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := db.QueryRow("SELECT count(*) FROM goal_parents").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 4 {
			t.Fatalf("relationships: got %d, want 4", count)
		}
	})

	for _, tc := range []struct {
		name, statement string
		code            int
	}{
		{"rejects duplicate links", "INSERT INTO goal_parents VALUES (2, 1, 'owner')", sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY},
		{"rejects self links", "INSERT INTO goal_parents VALUES (5, 5, 'owner')", sqlite3.SQLITE_CONSTRAINT_CHECK},
		{"rejects missing children", "INSERT INTO goal_parents VALUES (99, 1, 'owner')", sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY},
		{"rejects missing parents", "INSERT INTO goal_parents VALUES (5, 99, 'owner')", sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY},
		{"rejects another users parent", "INSERT INTO goal_parents VALUES (5, 6, 'owner')", sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY},
		{"rejects another users child", "INSERT INTO goal_parents VALUES (6, 5, 'owner')", sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY},
		{"rejects missing ownership", "INSERT INTO goal_parents VALUES (5, 1, NULL)", sqlite3.SQLITE_CONSTRAINT_NOTNULL},
		{"rejects changing parent ownership", "UPDATE goal_parents SET parent_goal_id = 6 WHERE goal_id = 2", sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY},
		{"rejects changing relationship ownership", "UPDATE goal_parents SET user_id = 'other' WHERE goal_id = 2", sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY},
		{"rejects moving a linked goal to another owner", "UPDATE goals SET user_id = 'other' WHERE goal_id = 2", sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY},
		{"rejects two goal cycles", "INSERT INTO goal_parents VALUES (1, 2, 'owner')", sqlite3.SQLITE_CONSTRAINT_TRIGGER},
		{"rejects longer cycles", "INSERT INTO goal_parents VALUES (1, 4, 'owner')", sqlite3.SQLITE_CONSTRAINT_TRIGGER},
		{"rejects cycles when changing the parent", "UPDATE goal_parents SET parent_goal_id = 4 WHERE goal_id = 2", sqlite3.SQLITE_CONSTRAINT_TRIGGER},
		{"rejects cycles when changing the child", "UPDATE goal_parents SET goal_id = 1 WHERE goal_id = 4 AND parent_goal_id = 2", sqlite3.SQLITE_CONSTRAINT_TRIGGER},
		{"rolls back all rows when an insert introduces a cycle", "INSERT INTO goal_parents VALUES (5, 1, 'owner'), (1, 4, 'owner')", sqlite3.SQLITE_CONSTRAINT_TRIGGER},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openMigratedDatabase(t)
			if _, err := db.Exec(goalParentFixture + goalParentDiamond); err != nil {
				t.Fatal(err)
			}
			_, err := db.Exec(tc.statement)
			var cause *sqlite.Error
			if !errors.As(err, &cause) {
				t.Fatalf("constraint: got %v, want SQLite code %d", err, tc.code)
			}
			if got := cause.Code(); got != tc.code {
				t.Fatalf("constraint code: got %d, want %d", got, tc.code)
			}
			var count int
			if err := db.QueryRow(`SELECT count(*) FROM goal_parents WHERE user_id = 'owner'
                AND (goal_id, parent_goal_id) IN ((2,1),(3,1),(4,2),(4,3))`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 4 {
				t.Fatalf("original links: got %d, want 4", count)
			}
			if err := db.QueryRow("SELECT count(*) FROM goal_parents").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 4 {
				t.Fatalf("links after rejected statement: got %d, want 4", count)
			}
		})
	}

	for _, tc := range []struct {
		name, statement, query string
		want                   int
	}{
		{"allows valid reparenting", "UPDATE goal_parents SET parent_goal_id = 5 WHERE goal_id = 2", "SELECT count(*) FROM goal_parents WHERE goal_id = 2 AND parent_goal_id = 5", 1},
		{"cascades goal ID changes through both endpoints", "UPDATE goals SET goal_id = 7 WHERE goal_id = 2", "SELECT count(*) FROM goal_parents WHERE goal_id = 7 OR parent_goal_id = 7", 2},
		{"cascades user ID changes through all relationships", "UPDATE users SET user_id = 'renamed' WHERE user_id = 'owner'", "SELECT count(*) FROM goal_parents WHERE user_id = 'renamed'", 4},
		{"removes only incident links on hard deletion", "DELETE FROM goals WHERE goal_id = 2", "SELECT count(*) FROM goal_parents", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openMigratedDatabase(t)
			if _, err := db.Exec(goalParentFixture + goalParentDiamond); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.statement); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := db.QueryRow(tc.query).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != tc.want {
				t.Fatalf("affected links: got %d, want %d", count, tc.want)
			}
			if err := db.QueryRow("SELECT count(*) FROM goals WHERE goal_id IN (1,3,4,5,6)").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 5 {
				t.Fatalf("unaffected goals: got %d, want 5", count)
			}
			if err := db.QueryRow("SELECT count(*) FROM pragma_foreign_key_check").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("foreign key violations: got %d, want 0", count)
			}
		})
	}

	t.Run("rejects cycles through soft deleted and inactive goals", func(t *testing.T) {
		db := openMigratedDatabase(t)
		if _, err := db.Exec(goalParentFixture + goalParentDiamond); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("UPDATE goals SET deleted_at = updated_at, goal_is_active = 0 WHERE goal_id IN (2,3)"); err != nil {
			t.Fatal(err)
		}
		_, err := db.Exec("INSERT INTO goal_parents VALUES (1, 4, 'owner')")
		var cause *sqlite.Error
		if !errors.As(err, &cause) || cause.Code() != sqlite3.SQLITE_CONSTRAINT_TRIGGER {
			t.Fatalf("cycle: got %v, want trigger constraint", err)
		}
	})

	for _, full := range []bool{false, true} {
		name := "rolls back relationships while preserving goals and progress"
		if full {
			name = "rebuilds the full populated schema"
		}
		t.Run(name, func(t *testing.T) {
			db := openMigratedDatabase(t)
			if _, err := db.Exec(goalParentFixture + goalParentDiamond + "INSERT INTO goal_progress (goal_id,user_id,time_spent_sec) VALUES (4,'owner',60)"); err != nil {
				t.Fatal(err)
			}
			pattern := "000010_*.down.sql"
			if full {
				pattern = "*.down.sql"
			}
			applyMigrationFiles(t, db, pattern, true)
			var count int
			if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name = 'goal_parents' OR tbl_name = 'goal_parents'").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("relationship schema objects: got %d, want 0", count)
			}
			if full {
				applyMigrationFiles(t, db, "*.up.sql", false)
				if _, err := db.Exec(goalParentFixture); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := db.QueryRow("SELECT count(*) FROM goals").Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != 6 {
					t.Fatalf("retained goals: got %d, want 6", count)
				}
				if err := db.QueryRow("SELECT count(*) FROM goal_progress").Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != 1 {
					t.Fatalf("retained progress: got %d, want 1", count)
				}
				applyMigrationFiles(t, db, "000010_*.up.sql", false)
			}
			if _, err := db.Exec(goalParentDiamond); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// This query documents the future rollup contract, not a production service.
// Deduplicate goal IDs, not time values; separate equal-duration entries count.
func goalRollup(t *testing.T, db *sql.DB, userID string, goalID, from, until int64) int64 {
	t.Helper()
	var seconds int64
	err := db.QueryRow(`WITH RECURSIVE descendants(goal_id) AS (
        SELECT g.goal_id FROM goals g JOIN users u USING (user_id)
        WHERE g.goal_id = ? AND g.user_id = ? AND g.deleted_at IS NULL AND u.deleted_at IS NULL
        UNION
        SELECT p.goal_id FROM goal_parents p JOIN descendants d ON p.parent_goal_id = d.goal_id
        WHERE p.user_id = ?
    )
    SELECT coalesce(sum(p.time_spent_sec), 0) FROM goal_progress p JOIN descendants d USING (goal_id)
    WHERE p.user_id = ? AND p.occurred_at >= ? AND p.occurred_at < ?`, goalID, userID, userID, userID, from, until).Scan(&seconds)
	if err != nil {
		t.Fatal(err)
	}
	return seconds
}

func TestGoalRollupWorkflow_SQLite(t *testing.T) {
	for _, tc := range []struct {
		name, change string
		root, want   int64
	}{
		{"counts shared descendants once and includes direct progress", "", 1, 150},
		{"gives full credit to each parent", "", 2, 120},
		{"gives full credit to the other parent", "", 3, 120},
		{"retains progress through a soft deleted intermediate goal", "UPDATE goals SET deleted_at = updated_at WHERE goal_id IN (2,3)", 1, 150},
		{"retains a soft deleted descendants progress", "UPDATE goals SET deleted_at = updated_at WHERE goal_id = 4", 1, 150},
		{"retains inactive descendants progress", "UPDATE goals SET goal_is_active = 0 WHERE goal_id IN (2,3,4)", 1, 150},
		{"removing one path does not remove shared progress", "DELETE FROM goal_parents WHERE goal_id = 4 AND parent_goal_id = 2", 1, 150},
		{"removing all old paths changes past attribution", "DELETE FROM goal_parents WHERE goal_id = 4; INSERT INTO goal_parents VALUES (4,5,'owner')", 1, 30},
		{"adding a new parent includes existing progress", "INSERT INTO goal_parents VALUES (4,5,'owner')", 5, 120},
		{"keeps descendants after hard deleting an intermediate goal", "DELETE FROM goals WHERE goal_id = 2", 1, 150},
		{"preserves existing hard deletion of a goals own progress", "DELETE FROM goals WHERE goal_id = 4", 1, 30},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openMigratedDatabase(t)
			if _, err := db.Exec(goalParentFixture + goalParentDiamond + `
                INSERT INTO goal_progress (goal_id,user_id,occurred_at,time_spent_sec) VALUES
                    (1,'owner',100,30), (4,'owner',100,60), (4,'owner',150,60),
                    (4,'owner',99,900), (4,'owner',200,900), (6,'other',100,900);`); err != nil {
				t.Fatal(err)
			}
			if tc.change != "" {
				if _, err := db.Exec(tc.change); err != nil {
					t.Fatal(err)
				}
			}
			if got := goalRollup(t, db, "owner", tc.root, 100, 200); got != tc.want {
				t.Fatalf("rolled up seconds: got %d, want %d", got, tc.want)
			}
			if got := goalRollup(t, db, "other", tc.root, 100, 200); got != 0 {
				t.Fatalf("other users rollup: got %d, want 0", got)
			}
		})
	}
}
