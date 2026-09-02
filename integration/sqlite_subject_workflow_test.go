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
	t.Run("manages subjects while enforcing ownership and uniqueness", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "user-1", "user-2", "user-3")
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

		second, err := service.Create(ctx, "user-1", "writing")
		if err != nil {
			t.Fatal(err)
		}
		got, err := service.Get(ctx, "user-1", created.SubjectID)
		if err != nil {
			t.Fatal(err)
		}
		assertSubjectEqual(t, got, created)

		if _, err := service.Create(ctx, "user-1", created.SubjectName); !errors.Is(err, data.ErrDuplicateRecord) {
			t.Fatalf("got duplicate error %v, want %v", err, data.ErrDuplicateRecord)
		}

		other, err := service.Create(ctx, "user-2", created.SubjectName)
		if err != nil {
			t.Fatal(err)
		}
		if other.SubjectID == created.SubjectID || other.UserID != "user-2" {
			t.Fatalf("got other users subject %#v", other)
		}

		listed, err := service.List(ctx, "user-1")
		if err != nil {
			t.Fatal(err)
		}
		if len(listed) != 2 || listed[0].SubjectID != created.SubjectID || listed[1].SubjectID != second.SubjectID {
			t.Fatalf("got subjects %#v", listed)
		}
		empty, err := service.List(ctx, "user-3")
		if err != nil {
			t.Fatal(err)
		}
		if empty == nil || len(empty) != 0 {
			t.Fatalf("got empty subjects %#v", empty)
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
		if _, err := service.Update(ctx, "user-2", created.SubjectID, "not mine"); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got cross-user update error %v, want %v", err, data.ErrRecordNotFound)
		}
		if _, err := service.Update(ctx, "user-1", 99, "missing"); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got missing update error %v, want %v", err, data.ErrRecordNotFound)
		}

		thoughtService := thought.NewService(data.NewSQLiteThoughtStore(db))
		linkedThought, err := thoughtService.Create(ctx, "user-1", "keep me", &created.SubjectID, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if err := service.Delete(ctx, "user-2", created.SubjectID); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got cross-user delete error %v, want %v", err, data.ErrRecordNotFound)
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

		if _, err := service.Get(ctx, "user-2", second.SubjectID); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got cross-user error %v, want %v", err, data.ErrRecordNotFound)
		}
		if _, err := service.Get(ctx, "user-1", 99); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got missing subject error %v, want %v", err, data.ErrRecordNotFound)
		}
		if _, err := service.Create(context.Background(), "missing-user", "coding"); err == nil {
			t.Fatal("expected foreign key error for missing user")
		}

		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		reopened := openSQLite(t, dsn)
		reopenedService := subject.NewService(data.NewSQLiteSubjectStore(reopened))
		if _, err := reopenedService.Get(ctx, "user-1", created.SubjectID); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got deleted subject error %v, want %v", err, data.ErrRecordNotFound)
		}
		persisted, err := reopenedService.List(ctx, "user-1")
		if err != nil {
			t.Fatal(err)
		}
		if len(persisted) != 1 {
			t.Fatalf("got persisted subjects %#v", persisted)
		}
		assertSubjectEqual(t, &persisted[0], second)
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
