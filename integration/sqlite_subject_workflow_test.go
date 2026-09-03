//go:build integration

package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/thought"
	_ "modernc.org/sqlite"
)

func TestSubjectWorkflow_SQLite(t *testing.T) {
	t.Run("creates and retrieves subject", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "user-1")
		service := subject.NewService(data.NewSQLiteSubjectStore(db))
		ctx := context.Background()

		created, err := service.Create(ctx, "user-1", "  learn Go  ")
		if err != nil {
			t.Fatal(err)
		}
		if created.SubjectID == 0 || created.UserID != "user-1" || created.SubjectName != "  learn Go  " {
			t.Fatalf("got created subject %#v", created)
		}
		if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
			t.Fatal("expected created subject timestamps")
		}
		got, err := service.Get(ctx, "user-1", created.SubjectID)
		if err != nil {
			t.Fatal(err)
		}
		assertSubjectEqual(t, got, created)
	})

	t.Run("enforces per-user uniqueness", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "user-1", "user-2")
		service := subject.NewService(data.NewSQLiteSubjectStore(db))
		ctx := context.Background()

		created, err := service.Create(ctx, "user-1", "coding")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.Create(ctx, "user-1", "coding"); !errors.Is(err, data.ErrDuplicateRecord) {
			t.Fatalf("got duplicate error %v, want %v", err, data.ErrDuplicateRecord)
		}

		other, err := service.Create(ctx, "user-2", "coding")
		if err != nil {
			t.Fatal(err)
		}
		if other.SubjectID == created.SubjectID || other.UserID != "user-2" {
			t.Fatalf("got other users subject %#v", other)
		}
	})

	t.Run("lists subjects for user", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "user-1", "user-2", "user-3")
		service := subject.NewService(data.NewSQLiteSubjectStore(db))
		ctx := context.Background()

		first, err := service.Create(ctx, "user-1", "coding")
		if err != nil {
			t.Fatal(err)
		}
		second, err := service.Create(ctx, "user-1", "writing")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.Create(ctx, "user-2", "private"); err != nil {
			t.Fatal(err)
		}

		listed, err := service.List(ctx, "user-1")
		if err != nil {
			t.Fatal(err)
		}
		if len(listed) != 2 || listed[0].SubjectID != first.SubjectID || listed[1].SubjectID != second.SubjectID {
			t.Fatalf("got subjects %#v", listed)
		}

		empty, err := service.List(ctx, "user-3")
		if err != nil {
			t.Fatal(err)
		}
		if empty == nil || len(empty) != 0 {
			t.Fatalf("got empty subjects %#v", empty)
		}
	})

	t.Run("updates subject", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "user-1")
		service := subject.NewService(data.NewSQLiteSubjectStore(db))
		ctx := context.Background()

		created, err := service.Create(ctx, "user-1", "coding")
		if err != nil {
			t.Fatal(err)
		}
		second, err := service.Create(ctx, "user-1", "writing")
		if err != nil {
			t.Fatal(err)
		}

		updated, err := service.Update(ctx, "user-1", created.SubjectID, "  Go programming  ")
		if err != nil {
			t.Fatal(err)
		}
		if updated.SubjectName != "  Go programming  " || !updated.CreatedAt.Equal(created.CreatedAt) || updated.UpdatedAt.Before(created.UpdatedAt) {
			t.Fatalf("got updated subject %#v, created %#v", updated, created)
		}
		if _, err := service.Update(ctx, "user-1", created.SubjectID, updated.SubjectName); err != nil {
			t.Fatalf("update to existing name: %v", err)
		}
		if _, err := service.Update(ctx, "user-1", created.SubjectID, second.SubjectName); !errors.Is(err, data.ErrDuplicateRecord) {
			t.Fatalf("got duplicate update error %v, want %v", err, data.ErrDuplicateRecord)
		}
	})

	t.Run("prevents cross-user access and mutation", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "user-1", "user-2")
		service := subject.NewService(data.NewSQLiteSubjectStore(db))
		ctx := context.Background()

		created, err := service.Create(ctx, "user-1", "coding")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.Get(ctx, "user-2", created.SubjectID); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got cross-user get error %v, want %v", err, data.ErrRecordNotFound)
		}
		if _, err := service.Update(ctx, "user-2", created.SubjectID, "not mine"); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got cross-user update error %v, want %v", err, data.ErrRecordNotFound)
		}
		if err := service.Delete(ctx, "user-2", created.SubjectID); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got cross-user delete error %v, want %v", err, data.ErrRecordNotFound)
		}
	})

	t.Run("reports missing records", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "user-1")
		service := subject.NewService(data.NewSQLiteSubjectStore(db))
		ctx := context.Background()

		if _, err := service.Get(ctx, "user-1", 99); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got missing get error %v, want %v", err, data.ErrRecordNotFound)
		}
		if _, err := service.Update(ctx, "user-1", 99, "missing"); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got missing update error %v, want %v", err, data.ErrRecordNotFound)
		}
		if err := service.Delete(ctx, "user-1", 99); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got missing delete error %v, want %v", err, data.ErrRecordNotFound)
		}
	})

	t.Run("enforces user foreign key", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		service := subject.NewService(data.NewSQLiteSubjectStore(db))

		if _, err := service.Create(context.Background(), "missing-user", "coding"); err == nil {
			t.Fatal("expected foreign key error for missing user")
		}
	})

	t.Run("deletes subject and unlinks linked thoughts", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "user-1")
		service := subject.NewService(data.NewSQLiteSubjectStore(db))
		ctx := context.Background()

		created, err := service.Create(ctx, "user-1", "coding")
		if err != nil {
			t.Fatal(err)
		}
		thoughtService := thought.NewService(data.NewSQLiteThoughtStore(db))
		linkedThought, err := thoughtService.Create(ctx, "user-1", "keep me", &created.SubjectID, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if err := service.Delete(ctx, "user-1", created.SubjectID); err != nil {
			t.Fatal(err)
		}
		if err := service.Delete(ctx, "user-1", created.SubjectID); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got repeated delete error %v, want %v", err, data.ErrRecordNotFound)
		}
		unlinkedThought, err := thoughtService.Get(ctx, "user-1", linkedThought.ThoughtID)
		if err != nil {
			t.Fatal(err)
		}
		if unlinkedThought.SubjectID != nil {
			t.Fatalf("got linked subject ID %#v after deletion", unlinkedThought.SubjectID)
		}
	})

	t.Run("persists changes after reopening database", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "user-1")
		service := subject.NewService(data.NewSQLiteSubjectStore(db))
		ctx := context.Background()

		deleted, err := service.Create(ctx, "user-1", "temporary")
		if err != nil {
			t.Fatal(err)
		}
		persisted, err := service.Create(ctx, "user-1", "writing")
		if err != nil {
			t.Fatal(err)
		}
		thoughtService := thought.NewService(data.NewSQLiteThoughtStore(db))
		linkedThought, err := thoughtService.Create(ctx, "user-1", "keep me", &deleted.SubjectID, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if err := service.Delete(ctx, "user-1", deleted.SubjectID); err != nil {
			t.Fatal(err)
		}

		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		reopened := openSQLite(t, dsn)
		reopenedService := subject.NewService(data.NewSQLiteSubjectStore(reopened))
		if _, err := reopenedService.Get(ctx, "user-1", deleted.SubjectID); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got deleted subject error %v, want %v", err, data.ErrRecordNotFound)
		}
		listed, err := reopenedService.List(ctx, "user-1")
		if err != nil {
			t.Fatal(err)
		}
		if len(listed) != 1 {
			t.Fatalf("got persisted subjects %#v", listed)
		}
		assertSubjectEqual(t, &listed[0], persisted)
		persistedThought, err := thought.NewService(data.NewSQLiteThoughtStore(reopened)).Get(ctx, "user-1", linkedThought.ThoughtID)
		if err != nil {
			t.Fatal(err)
		}
		if persistedThought.SubjectID != nil {
			t.Fatalf("got persisted subject ID %#v after deletion", persistedThought.SubjectID)
		}
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
