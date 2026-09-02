//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
	_ "modernc.org/sqlite"
)

func TestSubjectWorkflow_SQLite(t *testing.T) {
	t.Run("persists subjects while enforcing ownership and uniqueness", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "user-1", "user-2")
		service := subject.NewService(data.NewSQLiteSubjectStore(db))

		created, err := service.Create(context.Background(), "user-1", "  learn Go  ")
		if err != nil {
			t.Fatal(err)
		}
		if created.SubjectID == 0 || created.UserID != "user-1" || created.SubjectName != "  learn Go  " {
			t.Fatalf("got created subject %#v", created)
		}
		if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
			t.Fatal("expected created subject timestamps")
		}

		got, err := service.Get(context.Background(), "user-1", created.SubjectID)
		if err != nil {
			t.Fatal(err)
		}
		assertSubjectEqual(t, got, created)

		if _, err := service.Create(context.Background(), "user-1", created.SubjectName); !errors.Is(err, data.ErrDuplicateRecord) {
			t.Fatalf("got duplicate error %v, want %v", err, data.ErrDuplicateRecord)
		}

		other, err := service.Create(context.Background(), "user-2", created.SubjectName)
		if err != nil {
			t.Fatal(err)
		}
		if other.SubjectID == created.SubjectID || other.UserID != "user-2" {
			t.Fatalf("got other users subject %#v", other)
		}

		if _, err := service.Get(context.Background(), "user-2", created.SubjectID); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got cross-user error %v, want %v", err, data.ErrRecordNotFound)
		}
		if _, err := service.Get(context.Background(), "user-1", 99); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got missing subject error %v, want %v", err, data.ErrRecordNotFound)
		}
		if _, err := service.Create(context.Background(), "missing-user", "coding"); err == nil {
			t.Fatal("expected foreign key error for missing user")
		}

		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		reopened := openSQLite(t, dsn)
		persisted, err := subject.NewService(data.NewSQLiteSubjectStore(reopened)).Get(context.Background(), "user-1", created.SubjectID)
		if err != nil {
			t.Fatal(err)
		}
		assertSubjectEqual(t, persisted, created)
	})
}

func assertSubjectEqual(t *testing.T, got, want *data.Subject) {
	t.Helper()
	if got.SubjectID != want.SubjectID || got.UserID != want.UserID || got.SubjectName != want.SubjectName {
		t.Fatalf("got subject %#v, want %#v", got, want)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) || !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Fatalf("got timestamps %v and %v, want %v and %v", got.CreatedAt, got.UpdatedAt, want.CreatedAt, want.UpdatedAt)
	}
}

func openMigratedSQLite(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "thoughts.db")
	db := openSQLite(t, dsn)

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

func insertUsers(t *testing.T, db *sql.DB, userIDs ...string) {
	t.Helper()
	for _, userID := range userIDs {
		if _, err := db.Exec("INSERT INTO users (user_id) VALUES (?)", userID); err != nil {
			t.Fatal(err)
		}
	}
}
