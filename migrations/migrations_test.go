package migrations

import (
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"testing"

	_ "modernc.org/sqlite"
)

func openMigratedDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	applyMigrationFiles(t, db, "*.up.sql", false)
	return db
}

func applyMigrationFiles(t *testing.T, db *sql.DB, pattern string, reverse bool) {
	t.Helper()
	files, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	if reverse {
		for left, right := 0, len(files)-1; left < right; left, right = left+1, right-1 {
			files[left], files[right] = files[right], files[left]
		}
	}
	for _, file := range files {
		migration, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(migration)); err != nil {
			t.Fatalf("apply %s: %v", file, err)
		}
	}
}

func TestMigrations_ThoughtSubjectOwnership(t *testing.T) {
	t.Run("allows thought without subject", func(t *testing.T) {
		db := openMigratedDatabase(t)
		insertUsers(t, db)

		if _, err := db.Exec("INSERT INTO thoughts (user_id, thought) VALUES (?, ?)", "user-1", "unassigned"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("allows thought with subject owned by user", func(t *testing.T) {
		db := openMigratedDatabase(t)
		insertUsers(t, db)
		if _, err := db.Exec("INSERT INTO subjects (subject_id, user_id, subject_name) VALUES (1, 'user-1', 'coding')"); err != nil {
			t.Fatal(err)
		}

		if _, err := db.Exec("INSERT INTO thoughts (user_id, subject_id, thought) VALUES ('user-1', 1, 'learn Go')"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("rejects thought with another users subject", func(t *testing.T) {
		db := openMigratedDatabase(t)
		insertUsers(t, db)
		if _, err := db.Exec("INSERT INTO subjects (subject_id, user_id, subject_name) VALUES (1, 'user-1', 'coding')"); err != nil {
			t.Fatal(err)
		}

		if _, err := db.Exec("INSERT INTO thoughts (user_id, subject_id, thought) VALUES ('user-2', 1, 'learn Go')"); err == nil {
			t.Fatal("expected ownership constraint error")
		}
	})

	t.Run("rejects ownership mismatch on update", func(t *testing.T) {
		db := openMigratedDatabase(t)
		insertUsers(t, db)
		if _, err := db.Exec("INSERT INTO subjects (subject_id, user_id, subject_name) VALUES (1, 'user-1', 'coding')"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO thoughts (thought_id, user_id, subject_id, thought) VALUES (1, 'user-1', 1, 'learn Go')"); err != nil {
			t.Fatal(err)
		}

		if _, err := db.Exec("UPDATE thoughts SET user_id = 'user-2' WHERE thought_id = 1"); err == nil {
			t.Fatal("expected ownership constraint error")
		}
	})

	t.Run("clears subject without clearing thought owner", func(t *testing.T) {
		db := openMigratedDatabase(t)
		insertUsers(t, db)
		if _, err := db.Exec("INSERT INTO subjects (subject_id, user_id, subject_name) VALUES (1, 'user-1', 'coding')"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO thoughts (thought_id, user_id, subject_id, thought) VALUES (1, 'user-1', 1, 'learn Go')"); err != nil {
			t.Fatal(err)
		}

		if _, err := db.Exec("DELETE FROM subjects WHERE subject_id = 1"); err != nil {
			t.Fatal(err)
		}
		var userID string
		var subjectID *int64
		if err := db.QueryRow("SELECT user_id, subject_id FROM thoughts WHERE thought_id = 1").Scan(&userID, &subjectID); err != nil {
			t.Fatal(err)
		}
		if userID != "user-1" || subjectID != nil {
			t.Fatalf("got user %q and subject ID %#v", userID, subjectID)
		}
	})

	t.Run("cascades subject ownership to thought", func(t *testing.T) {
		db := openMigratedDatabase(t)
		insertUsers(t, db)
		if _, err := db.Exec("INSERT INTO subjects (subject_id, user_id, subject_name) VALUES (1, 'user-1', 'coding')"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO thoughts (thought_id, user_id, subject_id, thought) VALUES (1, 'user-1', 1, 'learn Go')"); err != nil {
			t.Fatal(err)
		}

		if _, err := db.Exec("UPDATE subjects SET user_id = 'user-2' WHERE subject_id = 1"); err != nil {
			t.Fatal(err)
		}
		var userID string
		if err := db.QueryRow("SELECT user_id FROM thoughts WHERE thought_id = 1").Scan(&userID); err != nil {
			t.Fatal(err)
		}
		if userID != "user-2" {
			t.Fatalf("got user %q, want user-2", userID)
		}
	})

	t.Run("cascades user ID through subject and thought", func(t *testing.T) {
		db := openMigratedDatabase(t)
		insertUsers(t, db)
		if _, err := db.Exec("INSERT INTO subjects (subject_id, user_id, subject_name) VALUES (1, 'user-1', 'coding')"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO thoughts (thought_id, user_id, subject_id, thought) VALUES (1, 'user-1', 1, 'learn Go')"); err != nil {
			t.Fatal(err)
		}

		if _, err := db.Exec("UPDATE users SET user_id = 'renamed-user' WHERE user_id = 'user-1'"); err != nil {
			t.Fatal(err)
		}
		var subjectUserID, thoughtUserID string
		if err := db.QueryRow("SELECT subjects.user_id, thoughts.user_id FROM subjects JOIN thoughts USING (subject_id) WHERE subject_id = 1").Scan(&subjectUserID, &thoughtUserID); err != nil {
			t.Fatal(err)
		}
		if subjectUserID != "renamed-user" || thoughtUserID != "renamed-user" {
			t.Fatalf("got subject user %q and thought user %q", subjectUserID, thoughtUserID)
		}
	})

	t.Run("passes foreign key check", func(t *testing.T) {
		db := openMigratedDatabase(t)
		var table string
		err := db.QueryRow("PRAGMA foreign_key_check").Scan(&table)
		if err != sql.ErrNoRows {
			t.Fatalf("got foreign key check result %q, error %v", table, err)
		}
	})
}

func TestMigrations_UserVersion(t *testing.T) {
	t.Run("defaults version to one", func(t *testing.T) {
		db := openMigratedDatabase(t)

		if _, err := db.Exec("INSERT INTO users (user_id) VALUES ('user-1')"); err != nil {
			t.Fatal(err)
		}
		var version int64
		if err := db.QueryRow("SELECT version FROM users WHERE user_id = 'user-1'").Scan(&version); err != nil {
			t.Fatal(err)
		}
		if version != 1 {
			t.Fatalf("got version %d, want 1", version)
		}
	})

	for _, version := range []int64{0, -1} {
		t.Run("rejects non-positive version", func(t *testing.T) {
			db := openMigratedDatabase(t)

			if _, err := db.Exec("INSERT INTO users (user_id, version) VALUES ('user-1', ?)", version); err == nil {
				t.Fatalf("expected version %d to be rejected", version)
			}
		})
	}
}

func TestMigrations_Down(t *testing.T) {
	db := openMigratedDatabase(t)

	applyMigrationFiles(t, db, "*.down.sql", true)

	var count int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("got %d application tables after down migrations", count)
	}
}

func insertUsers(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec("INSERT INTO users (user_id) VALUES ('user-1'), ('user-2')"); err != nil {
		t.Fatal(err)
	}
}
