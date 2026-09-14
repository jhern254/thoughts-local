//go:build integration

package integration_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/goal"
)

func TestGoalServiceWorkflow_SQLite(t *testing.T) {
	t.Run("runtime creates retrieves and lists goals by activation state", func(t *testing.T) {
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
		s, owner := runtime.Goals(), runtime.LocalUser().UserID
		active, err := s.Create(t.Context(), owner, goal.CreateInput{Name: "  Reading  ", TargetSeconds: 600})
		if err != nil {
			t.Fatal(err)
		}
		if active.GoalName != "Reading" || active.TargetSeconds != 600 || !active.IsActive || active.Cadence != "weekly" || active.TZ != "UTC" || active.WeekStart != "mon" || active.Version != 1 || active.StartDate != nil || active.EndDate != nil || active.DefaultCadence != nil {
			t.Fatalf("created goal: got %+v, want normalized name and schema defaults", active)
		}
		if active.CreatedAt.IsZero() || active.CreatedAt.Location() != time.UTC || !active.CreatedAt.Equal(active.UpdatedAt) || active.CreatedAt.Nanosecond() != 0 {
			t.Fatalf("timestamps: got %v/%v, want equal nonzero UTC seconds", active.CreatedAt, active.UpdatedAt)
		}
		isActive := false
		inactive, err := s.Create(t.Context(), owner, goal.CreateInput{Name: "Writing", TargetSeconds: 60, IsActive: &isActive,
			StartDate: "2024-02-29", EndDate: "2099-12-31", Cadence: "daily", TZ: "America/Los_Angeles", WeekStart: "sun", DefaultCadence: "monthly"})
		if err != nil {
			t.Fatal(err)
		}
		got, err := s.Get(t.Context(), owner, inactive.GoalID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, inactive) || got.StartDate == nil || *got.StartDate != "2024-02-29" || got.EndDate == nil || *got.EndDate != "2099-12-31" || got.Cadence != "daily" || got.TZ != "America/Los_Angeles" || got.WeekStart != "sun" || got.DefaultCadence == nil || *got.DefaultCadence != "monthly" {
			t.Fatalf("retrieved goal: got %+v, want persisted explicit settings %+v", got, inactive)
		}
		all, err := s.List(t.Context(), owner)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(all, []data.Goal{*active, *inactive}) {
			t.Fatalf("all goals: got %+v, want active then inactive records", all)
		}
		actives, err := s.ListActive(t.Context(), owner)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(actives, []data.Goal{*active}) {
			t.Fatalf("active goals: got %+v, want %+v", actives, active)
		}
		inactives, err := s.ListInactive(t.Context(), owner)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(inactives, []data.Goal{*inactive}) {
			t.Fatalf("inactive goals: got %+v, want %+v", inactives, inactive)
		}
	})
	t.Run("preserves duplicate and not found contracts with owner isolation", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "owner", "other")
		s := goal.NewService(data.NewSQLiteGoalStore(db))
		input := goal.CreateInput{Name: "Goal", TargetSeconds: 1}
		item, err := s.Create(t.Context(), "owner", input)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Create(t.Context(), "owner", input); !errors.Is(err, data.ErrDuplicateRecord) {
			t.Fatalf("duplicate create: got %v, want ErrDuplicateRecord", err)
		}
		if _, err := s.Get(t.Context(), "other", item.GoalID); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("other owner get: got %v, want ErrRecordNotFound", err)
		}
		if _, err := s.Create(t.Context(), "missing", input); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("missing owner create: got %v, want ErrRecordNotFound", err)
		}
		items, err := s.List(t.Context(), "other")
		if err != nil {
			t.Fatal(err)
		}
		if items == nil || len(items) != 0 {
			t.Fatalf("other owner list: got %#v, want non-nil empty slice", items)
		}
		if _, err := s.Create(t.Context(), "other", input); err != nil {
			t.Fatalf("other owner same name: got %v, want nil", err)
		}
	})
	t.Run("rejects an impossible calendar date without inserting a goal", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "owner")
		s := goal.NewService(data.NewSQLiteGoalStore(db))
		_, err := s.Create(t.Context(), "owner", goal.CreateInput{Name: "PRIVATE-INPUT", TargetSeconds: 1, StartDate: "2026-19-39"})
		var validation *goal.ValidationError
		if !errors.As(err, &validation) {
			t.Fatalf("invalid date: got %v, want ValidationError", err)
		}
		if got, want := validation.PublicFields()["goal_start_date"], "must be a valid calendar date in YYYY-MM-DD format"; got != want {
			t.Fatalf("date guidance: got %q, want %q", got, want)
		}
		if strings.Contains(validation.Error(), "PRIVATE-INPUT") {
			t.Fatal("validation error: got private input, want fixed message")
		}
		var count int
		if err := db.QueryRow("SELECT count(*) FROM goals").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if got, want := count, 0; got != want {
			t.Fatalf("inserted goals: got %d, want %d", got, want)
		}
	})
}
