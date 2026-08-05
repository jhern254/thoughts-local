package data

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestApplyMigrationsCreatesSubjectSchema(t *testing.T) {
	db := openTestSQLite(t)

	if err := ApplyMigrations(context.Background(), db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := ApplyMigrations(context.Background(), db); err != nil {
		t.Fatalf("reapply migrations: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO users (user_id) VALUES ('test-user')`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO subjects (user_id, subject_name) VALUES ('test-user', 'coding')`); err != nil {
		t.Fatalf("insert subject: %v", err)
	}
}

func TestSQLiteSubjectStoreCaptureSubject(t *testing.T) {
	db := openMigratedSubjectDB(t)
	store := NewSQLiteSubjectStore(db)
	ctx := context.Background()
	now := time.Date(2026, time.August, 5, 12, 0, 0, 0, time.UTC)
	subject := &Subject{
		UserID:      "test-user",
		SubjectName: "coding",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	id, err := store.CaptureSubject(ctx, "test-user", subject)
	if err != nil {
		t.Fatalf("capture subject: %v", err)
	}
	if id == 0 {
		t.Fatal("got subject ID 0, want generated ID")
	}

	var gotName string
	var gotCreatedAt, gotUpdatedAt int64
	err = db.QueryRowContext(ctx, `SELECT subject_name, created_at, updated_at FROM subjects WHERE subject_id = ?`, id).Scan(&gotName, &gotCreatedAt, &gotUpdatedAt)
	if err != nil {
		t.Fatalf("query subject: %v", err)
	}
	if gotName != subject.SubjectName {
		t.Errorf("got name %q, want %q", gotName, subject.SubjectName)
	}
	if gotCreatedAt != UnixSec(now) || gotUpdatedAt != UnixSec(now) {
		t.Errorf("got timestamps %d, %d; want %d", gotCreatedAt, gotUpdatedAt, UnixSec(now))
	}

	got, err := store.GetSubject(ctx, "test-user", "coding")
	if err != nil {
		t.Fatalf("get subject: %v", err)
	}
	if got.SubjectID != id || got.UserID != subject.UserID || got.SubjectName != subject.SubjectName {
		t.Errorf("got subject %#v, want persisted subject", got)
	}
	if !got.CreatedAt.Equal(now) || !got.UpdatedAt.Equal(now) {
		t.Errorf("got timestamps %v, %v; want %v", got.CreatedAt, got.UpdatedAt, now)
	}
	if _, err := store.GetSubject(ctx, "test-user", "missing"); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("got missing subject error %v, want ErrRecordNotFound", err)
	}
}

func TestSQLiteSubjectStoreCaptureSubjectMapsDuplicate(t *testing.T) {
	db := openMigratedSubjectDB(t)
	store := NewSQLiteSubjectStore(db)
	subject := &Subject{UserID: "test-user", SubjectName: "coding", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}

	if _, err := store.CaptureSubject(context.Background(), "test-user", subject); err != nil {
		t.Fatalf("first capture subject: %v", err)
	}
	if _, err := store.CaptureSubject(context.Background(), "test-user", subject); !errors.Is(err, ErrDuplicateRecord) {
		t.Fatalf("got error %v, want ErrDuplicateRecord", err)
	}
}

func TestSQLiteSubjectStoreCaptureSubjectEnforcesUserForeignKey(t *testing.T) {
	db := openTestSQLite(t)
	if err := ApplyMigrations(context.Background(), db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	store := NewSQLiteSubjectStore(db)
	subject := &Subject{UserID: "missing-user", SubjectName: "coding", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}

	if _, err := store.CaptureSubject(context.Background(), "missing-user", subject); err == nil {
		t.Fatal("got nil error for missing user")
	}
}

func openMigratedSubjectDB(t *testing.T) *sql.DB {
	t.Helper()
	db := openTestSQLite(t)
	if err := ApplyMigrations(context.Background(), db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO users (user_id) VALUES ('test-user')`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return db
}

func openTestSQLite(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := EnableSQLiteFK(db); err != nil {
		db.Close()
		t.Fatalf("enable foreign keys: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
