//go:build integration

package integration_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/event"
)

func TestEventServiceWorkflow_SQLite(t *testing.T) {
	t.Run("runtime composes create get and list with automatic handoff", func(t *testing.T) {
		_, dsn := openMigratedSQLite(t)
		runtime, err := application.Open(t.Context(), dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := runtime.Close(); err != nil {
				t.Error(err)
			}
		})
		service, user := runtime.Events(), runtime.LocalUser().UserID
		first, err := service.Create(t.Context(), user, " first ", eventTime(100))
		if err != nil {
			t.Fatal(err)
		}
		second, err := service.Create(t.Context(), user, "", eventTime(200))
		if err != nil {
			t.Fatal(err)
		}
		got, err := service.Get(t.Context(), user, first.EventID)
		if err != nil {
			t.Fatal(err)
		}
		if got.ActivityType == nil || *got.ActivityType != "first" || got.EndedAt == nil || !got.EndedAt.Equal(second.StartedAt) || got.Version != 2 {
			t.Fatalf("got predecessor %+v, want normalized label and end at successor start with version 2", got)
		}
		items, err := service.List(t.Context(), user, eventTime(100), eventTime(300))
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 2 || items[0].EventID != first.EventID || items[1].EventID != second.EventID || items[1].EndedAt != nil || items[1].ActivityType != nil {
			t.Fatalf("got events %+v, want ordered predecessor and untitled ongoing successor", items)
		}
	})
	t.Run("historical creation preserves the ongoing event and returns overlap and not-found contracts", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u", "other")
		service := event.NewService(data.NewSQLiteEventStore(db))
		ongoing, err := service.Create(t.Context(), "u", "", eventTime(300))
		if err != nil {
			t.Fatal(err)
		}
		past, err := service.CreatePast(t.Context(), "u", "past", eventTime(100), eventTime(200))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.CreatePast(t.Context(), "u", "overlap", eventTime(150), eventTime(250)); !errors.Is(err, data.ErrEventOverlap) {
			t.Fatalf("got error %v, want ErrEventOverlap", err)
		}
		if _, err := service.Get(t.Context(), "other", past.EventID); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got error %v, want ErrRecordNotFound", err)
		}
		got, err := service.Get(t.Context(), "u", ongoing.EventID)
		if err != nil {
			t.Fatal(err)
		}
		if got.EndedAt != nil || got.Version != 1 {
			t.Fatalf("got ongoing event %+v, want unchanged ongoing version 1", got)
		}
	})
	t.Run("optional label validation matches SQLite without losing text", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		service := event.NewService(data.NewSQLiteEventStore(db))
		for _, label := range []string{"", "   ", "\t", "  " + strings.Repeat("界", 4096) + "  ", "a\x00" + strings.Repeat("b", 4096)} {
			got, err := service.CreatePast(t.Context(), "u", label, eventTime(100), eventTime(100))
			if err != nil {
				t.Fatal(err)
			}
			want := strings.Trim(label, " ")
			if want == "" {
				if got.ActivityType != nil {
					t.Fatal("got activity label, want nil")
				}
			} else if got.ActivityType == nil || *got.ActivityType != want {
				t.Fatal("got different stored label, want full normalized input")
			}
		}
		var validation *event.ValidationError
		if _, err := service.Create(t.Context(), "u", strings.Repeat("界", 4097), time.Time{}); !errors.As(err, &validation) {
			t.Fatalf("got error %v, want label validation", err)
		}
	})
}
