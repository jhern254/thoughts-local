package data_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
	_ "modernc.org/sqlite"
)

func TestSQLiteStore_CreateSubject(t *testing.T) {
	t.Run("creates and returns persisted subject", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "test-user")
		store := data.NewSQLiteStore(db)
		now := time.Date(2026, time.September, 2, 12, 30, 0, 0, time.UTC)

		created, err := store.CreateSubject(context.Background(), &data.Subject{
			UserID:      "test-user",
			SubjectName: "  learn Go  ",
			CreatedAt:   now,
			UpdatedAt:   now,
		})

		if err != nil {
			t.Fatal(err)
		}
		if created.SubjectID == 0 {
			t.Fatal("got subject ID 0")
		}
		if created.UserID != "test-user" || created.SubjectName != "  learn Go  " {
			t.Fatalf("got subject %#v", created)
		}
		if !created.CreatedAt.Equal(now) || !created.UpdatedAt.Equal(now) {
			t.Fatalf("got timestamps %v and %v, want %v", created.CreatedAt, created.UpdatedAt, now)
		}
	})

	t.Run("returns duplicate record for the same users subject name", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "test-user")
		store := data.NewSQLiteStore(db)
		item := &data.Subject{UserID: "test-user", SubjectName: "coding"}
		if _, err := store.CreateSubject(context.Background(), item); err != nil {
			t.Fatal(err)
		}

		_, err := store.CreateSubject(context.Background(), item)

		if !errors.Is(err, data.ErrDuplicateRecord) {
			t.Fatalf("got %v, want %v", err, data.ErrDuplicateRecord)
		}
	})

	t.Run("allows the same subject name for different users", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "user-1", "user-2")
		store := data.NewSQLiteStore(db)
		if _, err := store.CreateSubject(context.Background(), &data.Subject{UserID: "user-1", SubjectName: "coding"}); err != nil {
			t.Fatal(err)
		}

		created, err := store.CreateSubject(context.Background(), &data.Subject{UserID: "user-2", SubjectName: "coding"})

		if err != nil {
			t.Fatal(err)
		}
		if created.UserID != "user-2" {
			t.Fatalf("got subject %#v", created)
		}
	})

	t.Run("rejects a subject without an existing user", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		store := data.NewSQLiteStore(db)

		_, err := store.CreateSubject(context.Background(), &data.Subject{UserID: "missing-user", SubjectName: "coding"})

		if err == nil {
			t.Fatal("expected foreign key error")
		}
	})
}

func TestSQLiteStore_GetSubject(t *testing.T) {
	t.Run("returns subject for its owner", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "test-user")
		store := data.NewSQLiteStore(db)
		created, err := store.CreateSubject(context.Background(), &data.Subject{UserID: "test-user", SubjectName: "coding"})
		if err != nil {
			t.Fatal(err)
		}

		got, err := store.GetSubject(context.Background(), "test-user", created.SubjectID)

		if err != nil {
			t.Fatal(err)
		}
		if got.SubjectID != created.SubjectID || got.UserID != created.UserID || got.SubjectName != created.SubjectName {
			t.Fatalf("got subject %#v, want %#v", got, created)
		}
	})

	t.Run("returns not found for a missing subject", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "test-user")

		_, err := data.NewSQLiteStore(db).GetSubject(context.Background(), "test-user", 99)

		if !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got %v, want %v", err, data.ErrRecordNotFound)
		}
	})

	t.Run("does not return another users subject", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "user-1", "user-2")
		store := data.NewSQLiteStore(db)
		created, err := store.CreateSubject(context.Background(), &data.Subject{UserID: "user-1", SubjectName: "coding"})
		if err != nil {
			t.Fatal(err)
		}

		_, err = store.GetSubject(context.Background(), "user-2", created.SubjectID)

		if !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got %v, want %v", err, data.ErrRecordNotFound)
		}
	})
}

func TestSubjectWorkflow_SQLitePersistence(t *testing.T) {
	t.Run("creates and retrieves a subject after reopening the database", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "test-user")
		service := subject.NewService(data.NewSQLiteStore(db))

		created, err := service.Create(context.Background(), "test-user", "coding")
		if err != nil {
			t.Fatal(err)
		}
		got, err := service.Get(context.Background(), "test-user", created.SubjectID)
		if err != nil {
			t.Fatal(err)
		}
		if got.SubjectID != created.SubjectID || got.UserID != created.UserID || got.SubjectName != created.SubjectName {
			t.Fatalf("got subject %#v, want %#v", got, created)
		}
		if !got.CreatedAt.Equal(created.CreatedAt) || !got.UpdatedAt.Equal(created.UpdatedAt) {
			t.Fatalf("got timestamps %v and %v, want %v and %v", got.CreatedAt, got.UpdatedAt, created.CreatedAt, created.UpdatedAt)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}

		reopened := openSQLite(t, dsn)
		persisted, err := subject.NewService(data.NewSQLiteStore(reopened)).Get(context.Background(), "test-user", created.SubjectID)

		if err != nil {
			t.Fatal(err)
		}
		if persisted.SubjectID != created.SubjectID || persisted.UserID != "test-user" || persisted.SubjectName != "coding" {
			t.Fatalf("got persisted subject %#v, want %#v", persisted, created)
		}
		if !persisted.CreatedAt.Equal(created.CreatedAt) || !persisted.UpdatedAt.Equal(created.UpdatedAt) {
			t.Fatalf("got persisted timestamps %v and %v, want %v and %v", persisted.CreatedAt, persisted.UpdatedAt, created.CreatedAt, created.UpdatedAt)
		}
	})
}

func openMigratedSQLite(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "thoughts.db")
	db := openSQLite(t, dsn)

	files, err := filepath.Glob("../../migrations/*.up.sql")
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
