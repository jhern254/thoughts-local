package migrations

import (
	"database/sql"
	"errors"
	"testing"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const mindsetFixture = `
INSERT INTO users (user_id) VALUES ('owner');
INSERT INTO thoughts (thought_id, user_id, thought) VALUES
    (1, 'owner', 'first thought'), (2, 'owner', 'second thought');`

func TestMindsetPeriodsWorkflow_SQLite(t *testing.T) {
	t.Run("starts an open period at the current epoch second", func(t *testing.T) {
		db := openMigratedDatabase(t)
		if _, err := db.Exec(mindsetFixture); err != nil {
			t.Fatal(err)
		}
		var started, now int64
		var ended sql.NullInt64
		err := db.QueryRow(`INSERT INTO thought_mindset_periods (thought_id) VALUES (1)
            RETURNING started_at, ended_at, unixepoch('now')`).Scan(&started, &ended, &now)
		if err != nil {
			t.Fatal(err)
		}
		if started != now {
			t.Fatalf("started_at: got %d, want %d", started, now)
		}
		if ended.Valid {
			t.Fatalf("ended_at: got %d, want NULL", ended.Int64)
		}
	})

	t.Run("reactivates a thought without replacing its earlier period", func(t *testing.T) {
		db := openMigratedDatabase(t)
		if _, err := db.Exec(mindsetFixture); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`
            INSERT INTO thought_mindset_periods (thought_id, started_at) VALUES (1, 100);
            UPDATE thought_mindset_periods SET ended_at = 200 WHERE thought_id = 1;
            INSERT INTO thought_mindset_periods (thought_id, started_at) VALUES (1, 300);`); err != nil {
			t.Fatal(err)
		}
		var count, open, firstStart, firstEnd int64
		if err := db.QueryRow(`SELECT count(*), sum(ended_at IS NULL), min(started_at), min(ended_at)
            FROM thought_mindset_periods WHERE thought_id = 1`).Scan(&count, &open, &firstStart, &firstEnd); err != nil {
			t.Fatal(err)
		}
		if count != 2 || open != 1 || firstStart != 100 || firstEnd != 200 {
			t.Fatalf("history: got count=%d open=%d first=[%d,%d], want 2, 1, [100,200]", count, open, firstStart, firstEnd)
		}
	})

	t.Run("allows multiple thoughts to be active simultaneously", func(t *testing.T) {
		db := openMigratedDatabase(t)
		if _, err := db.Exec(mindsetFixture); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO thought_mindset_periods (thought_id) VALUES (1), (2)"); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := db.QueryRow("SELECT count(*) FROM thought_mindset_periods WHERE ended_at IS NULL").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 2 {
			t.Fatalf("active thoughts: got %d, want 2", count)
		}
	})

	t.Run("allows closing and reactivating within the same second", func(t *testing.T) {
		db := openMigratedDatabase(t)
		if _, err := db.Exec(mindsetFixture); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`
            INSERT INTO thought_mindset_periods (thought_id, started_at) VALUES (1, 100);
            UPDATE thought_mindset_periods SET ended_at = 100 WHERE thought_id = 1;
            INSERT INTO thought_mindset_periods (thought_id, started_at) VALUES (1, 100);`); err != nil {
			t.Fatal(err)
		}
	})

	for _, tc := range []struct {
		name, statement string
		code            int
	}{
		{"rejects a second open period", "INSERT INTO thought_mindset_periods (thought_id) VALUES (1)", sqlite3.SQLITE_CONSTRAINT_UNIQUE},
		{"rejects reopening history while another period is open", "UPDATE thought_mindset_periods SET ended_at = NULL WHERE mindset_period_id = 1", sqlite3.SQLITE_CONSTRAINT_UNIQUE},
		{"rejects a missing parent", "INSERT INTO thought_mindset_periods (thought_id) VALUES (99)", sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY},
		{"rejects removing the parent reference", "UPDATE thought_mindset_periods SET thought_id = NULL WHERE mindset_period_id = 1", sqlite3.SQLITE_CONSTRAINT_NOTNULL},
		{"rejects changing to a missing parent", "UPDATE thought_mindset_periods SET thought_id = 99 WHERE mindset_period_id = 1", sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY},
		{"rejects a missing start", "INSERT INTO thought_mindset_periods (thought_id, started_at) VALUES (2, NULL)", sqlite3.SQLITE_CONSTRAINT_NOTNULL},
		{"rejects text start timestamps", "INSERT INTO thought_mindset_periods (thought_id, started_at) VALUES (2, 'invalid')", sqlite3.SQLITE_CONSTRAINT_CHECK},
		{"rejects fractional start timestamps", "INSERT INTO thought_mindset_periods (thought_id, started_at) VALUES (2, 100.5)", sqlite3.SQLITE_CONSTRAINT_CHECK},
		{"rejects text end timestamps", "UPDATE thought_mindset_periods SET ended_at = 'invalid' WHERE mindset_period_id = 2", sqlite3.SQLITE_CONSTRAINT_CHECK},
		{"rejects fractional end timestamps", "UPDATE thought_mindset_periods SET ended_at = 400.5 WHERE mindset_period_id = 2", sqlite3.SQLITE_CONSTRAINT_CHECK},
		{"rejects inserting reversed timestamps", "INSERT INTO thought_mindset_periods (thought_id, started_at, ended_at) VALUES (2, 200, 100)", sqlite3.SQLITE_CONSTRAINT_CHECK},
		{"rejects closing before the start", "UPDATE thought_mindset_periods SET ended_at = 100 WHERE mindset_period_id = 2", sqlite3.SQLITE_CONSTRAINT_CHECK},
		{"rejects moving the start after the end", "UPDATE thought_mindset_periods SET started_at = 400 WHERE mindset_period_id = 1", sqlite3.SQLITE_CONSTRAINT_CHECK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openMigratedDatabase(t)
			if _, err := db.Exec(mindsetFixture); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO thought_mindset_periods (mindset_period_id, thought_id, started_at, ended_at)
                VALUES (1, 1, 100, 200), (2, 1, 300, NULL)`); err != nil {
				t.Fatal(err)
			}
			_, err := db.Exec(tc.statement)
			var cause *sqlite.Error
			if !errors.As(err, &cause) {
				t.Fatalf("constraint: got %v, want SQLite error code %d", err, tc.code)
			}
			if got := cause.Code(); got != tc.code {
				t.Fatalf("constraint code: got %d, want %d", got, tc.code)
			}
		})
	}

	for _, table := range []string{"thoughts", "users"} {
		t.Run("soft deleting "+table+" retains open and closed history", func(t *testing.T) {
			db := openMigratedDatabase(t)
			if _, err := db.Exec(mindsetFixture); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO thought_mindset_periods (thought_id, started_at, ended_at)
                VALUES (1, 100, 200), (1, 300, NULL)`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("UPDATE " + table + " SET deleted_at = updated_at"); err != nil {
				t.Fatal(err)
			}
			var count, open int
			if err := db.QueryRow("SELECT count(*), sum(ended_at IS NULL) FROM thought_mindset_periods").Scan(&count, &open); err != nil {
				t.Fatal(err)
			}
			if count != 2 || open != 1 {
				t.Fatalf("retained history: got count=%d open=%d, want 2, 1", count, open)
			}
			if err := db.QueryRow(`SELECT count(*) FROM thought_mindset_periods p
                JOIN thoughts t USING (thought_id) JOIN users u USING (user_id)
                WHERE p.ended_at IS NULL AND t.deleted_at IS NULL AND u.deleted_at IS NULL`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("visible active mindsets: got %d, want 0", count)
			}
		})
	}

	t.Run("cascades parent ID changes and hard deletion", func(t *testing.T) {
		db := openMigratedDatabase(t)
		if _, err := db.Exec(mindsetFixture); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`
            INSERT INTO thought_mindset_periods (thought_id, started_at, ended_at) VALUES (1, 100, 200), (1, 300, NULL);
            UPDATE thoughts SET thought_id = 3 WHERE thought_id = 1;`); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := db.QueryRow("SELECT count(*) FROM thought_mindset_periods WHERE thought_id = 3").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 2 {
			t.Fatalf("relinked periods: got %d, want 2", count)
		}
		if _, err := db.Exec("DELETE FROM thoughts WHERE thought_id = 3"); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow("SELECT count(*) FROM thought_mindset_periods").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("orphaned periods: got %d, want 0", count)
		}
	})

	t.Run("rolls back mindset history without removing thoughts and recreates it", func(t *testing.T) {
		db := openMigratedDatabase(t)
		if _, err := db.Exec(mindsetFixture); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO thought_mindset_periods (thought_id) VALUES (1)"); err != nil {
			t.Fatal(err)
		}
		applyMigrationFiles(t, db, "000009_*.down.sql", true)
		var count int
		if err := db.QueryRow("SELECT count(*) FROM thoughts").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 2 {
			t.Fatalf("retained thoughts: got %d, want 2", count)
		}
		if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name = 'thought_mindset_periods' OR tbl_name = 'thought_mindset_periods'").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("remaining table/indexes: got %d, want 0", count)
		}
		applyMigrationFiles(t, db, "000009_*.up.sql", false)
		if _, err := db.Exec("INSERT INTO thought_mindset_periods (thought_id) VALUES (1)"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("rebuilds the entire populated schema", func(t *testing.T) {
		db := openMigratedDatabase(t)
		if _, err := db.Exec(mindsetFixture); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO thought_mindset_periods (thought_id) VALUES (1)"); err != nil {
			t.Fatal(err)
		}
		applyMigrationFiles(t, db, "*.down.sql", true)
		applyMigrationFiles(t, db, "*.up.sql", false)
		var count int
		if err := db.QueryRow("SELECT count(*) FROM thought_mindset_periods").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("rebuilt history: got %d, want 0", count)
		}
		if _, err := db.Exec(mindsetFixture); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO thought_mindset_periods (thought_id) VALUES (1)"); err != nil {
			t.Fatal(err)
		}
	})
}
