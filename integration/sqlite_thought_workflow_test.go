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
)

func TestThoughtWorkflow_SQLite(t *testing.T) {
	t.Run("persists assigned and unassigned thoughts while enforcing ownership", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "user-1", "user-2")
		subjectService := subject.NewService(data.NewSQLiteSubjectStore(db))
		thoughtStore := data.NewSQLiteThoughtStore(db)
		thoughtService := thought.NewService(thoughtStore)
		ctx := context.Background()

		ownedSubject, err := subjectService.Create(ctx, "user-1", "coding")
		if err != nil {
			t.Fatal(err)
		}
		otherSubject, err := subjectService.Create(ctx, "user-2", "private")
		if err != nil {
			t.Fatal(err)
		}

		observedAt := time.Date(2026, time.September, 2, 12, 30, 0, 0, time.FixedZone("test", -7*60*60))
		assigned, err := thoughtService.Create(ctx, "user-1", "  learn Go  ", &ownedSubject.SubjectID, observedAt)
		if err != nil {
			t.Fatal(err)
		}
		if assigned.ThoughtID == 0 || assigned.UserID != "user-1" || assigned.Thought != "  learn Go  " || assigned.Version != 1 {
			t.Fatalf("got assigned thought %#v", assigned)
		}
		if assigned.SubjectID == nil || *assigned.SubjectID != ownedSubject.SubjectID || assigned.EventID != nil {
			t.Fatalf("got assigned relationships %#v", assigned)
		}
		if !assigned.ObservedAt.Equal(observedAt) || assigned.ObservedAt.Location() != time.UTC {
			t.Fatalf("got observed time %v, want %v in UTC", assigned.ObservedAt, observedAt)
		}
		if assigned.CreatedAt.IsZero() || assigned.UpdatedAt.IsZero() {
			t.Fatal("expected assigned thought timestamps")
		}

		unassigned, err := thoughtService.Create(ctx, "user-1", "inbox", nil, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if unassigned.SubjectID != nil || unassigned.EventID != nil || unassigned.ObservedAt.IsZero() {
			t.Fatalf("got unassigned thought %#v", unassigned)
		}

		eventResult, err := db.Exec("INSERT INTO events (user_id) VALUES (?)", "user-1")
		if err != nil {
			t.Fatal(err)
		}
		eventID, err := eventResult.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		storedWithEvent, err := thoughtStore.CreateThought(ctx, &data.Thought{
			UserID:     "user-1",
			SubjectID:  &ownedSubject.SubjectID,
			EventID:    &eventID,
			Thought:    "event detail",
			Version:    2,
			ObservedAt: observedAt,
			CreatedAt:  observedAt,
			UpdatedAt:  observedAt,
		})
		if err != nil {
			t.Fatal(err)
		}
		if storedWithEvent.EventID == nil || *storedWithEvent.EventID != eventID || storedWithEvent.Version != 2 {
			t.Fatalf("got event-linked thought %#v", storedWithEvent)
		}

		got, err := thoughtService.Get(ctx, "user-1", assigned.ThoughtID)
		if err != nil {
			t.Fatal(err)
		}
		assertThoughtEqual(t, got, assigned)

		missingSubjectID := int64(99)
		if _, err := thoughtService.Create(ctx, "user-1", "orphan", &missingSubjectID, time.Time{}); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got missing subject error %v, want %v", err, data.ErrRecordNotFound)
		}
		if _, err := thoughtService.Create(ctx, "user-1", "not mine", &otherSubject.SubjectID, time.Time{}); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got cross-user subject error %v, want %v", err, data.ErrRecordNotFound)
		}
		if _, err := thoughtService.Get(ctx, "user-2", assigned.ThoughtID); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got cross-user thought error %v, want %v", err, data.ErrRecordNotFound)
		}

		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		reopened := openSQLite(t, dsn)
		reopenedService := thought.NewService(data.NewSQLiteThoughtStore(reopened))
		persistedAssigned, err := reopenedService.Get(ctx, "user-1", assigned.ThoughtID)
		if err != nil {
			t.Fatal(err)
		}
		assertThoughtEqual(t, persistedAssigned, assigned)
		persistedUnassigned, err := reopenedService.Get(ctx, "user-1", unassigned.ThoughtID)
		if err != nil {
			t.Fatal(err)
		}
		assertThoughtEqual(t, persistedUnassigned, unassigned)
		persistedWithEvent, err := reopenedService.Get(ctx, "user-1", storedWithEvent.ThoughtID)
		if err != nil {
			t.Fatal(err)
		}
		assertThoughtEqual(t, persistedWithEvent, storedWithEvent)
	})
}

func assertThoughtEqual(t *testing.T, got, want *data.Thought) {
	t.Helper()
	if got.ThoughtID != want.ThoughtID || got.UserID != want.UserID || got.Thought != want.Thought || got.Version != want.Version {
		t.Fatalf("got thought %#v, want %#v", got, want)
	}
	assertOptionalIDEqual(t, "subject", got.SubjectID, want.SubjectID)
	assertOptionalIDEqual(t, "event", got.EventID, want.EventID)
	if !got.ObservedAt.Equal(want.ObservedAt) || !got.CreatedAt.Equal(want.CreatedAt) || !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Fatalf("got thought timestamps %#v, want %#v", got, want)
	}
}

func assertOptionalIDEqual(t *testing.T, name string, got, want *int64) {
	t.Helper()
	if got == nil && want == nil {
		return
	}
	if got == nil || want == nil || *got != *want {
		t.Fatalf("got %s ID %#v, want %#v", name, got, want)
	}
}
