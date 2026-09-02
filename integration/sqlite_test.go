//go:build integration

package integration_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"testing"

	_ "modernc.org/sqlite"
)

func openMigratedSQLite(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "thoughts.db")
	db := openSQLite(t, dsn)
	applyUpMigrations(t, db)
	return db, dsn
}

func openSQLite(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	return db
}

func applyUpMigrations(t *testing.T, db *sql.DB) {
	t.Helper()
	files, err := filepath.Glob("../migrations/*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no up migrations found")
	}
	sort.Strings(files)
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

func insertUsers(t *testing.T, db *sql.DB, userIDs ...string) {
	t.Helper()
	for _, userID := range userIDs {
		if _, err := db.Exec("INSERT INTO users (user_id) VALUES (?)", userID); err != nil {
			t.Fatal(err)
		}
	}
}
