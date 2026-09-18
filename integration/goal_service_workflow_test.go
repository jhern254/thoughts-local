//go:build integration

package integration_test

import (
	"context"
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
		active, err := s.Create(t.Context(), owner, goal.CreateGoalInput{Name: "  Reading  ", TargetSeconds: 600})
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
		inactive, err := s.Create(t.Context(), owner, goal.CreateGoalInput{Name: "Writing", TargetSeconds: 60, IsActive: &isActive,
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
		input := goal.CreateGoalInput{Name: "Goal", TargetSeconds: 1}
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
		_, err := s.Create(t.Context(), "owner", goal.CreateGoalInput{Name: "PRIVATE-INPUT", TargetSeconds: 1, StartDate: "2026-19-39"})
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

func TestGoalMutationWorkflow_SQLite(t *testing.T) {
	t.Run("updates settings and activation while preserving creation metadata", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "owner")
		s := goal.NewService(data.NewSQLiteGoalStore(db))
		original, err := s.Create(t.Context(), "owner", goal.CreateGoalInput{Name: "Reading", TargetSeconds: 60})
		if err != nil {
			t.Fatal(err)
		}
		activeSetting := false
		input := goal.UpdateGoalInput{IsActive: &activeSetting, Name: " Writing ", TargetSeconds: 120, StartDate: " 2024-02-29 ", EndDate: "2030-12-31", Cadence: " monthly ", TZ: "America/Los_Angeles", WeekStart: "sun", DefaultCadence: "daily"}
		updated, err := s.Update(t.Context(), "owner", original.GoalID, original.Version, input)
		if err != nil {
			t.Fatal(err)
		}
		start, end, cadence := "2024-02-29", "2030-12-31", "daily"
		want := *original
		want.GoalName, want.TargetSeconds, want.IsActive = "Writing", 120, false
		want.StartDate, want.EndDate, want.DefaultCadence = &start, &end, &cadence
		want.Cadence, want.TZ, want.WeekStart = "monthly", "America/Los_Angeles", "sun"
		want.Version++
		want.UpdatedAt = updated.UpdatedAt
		if !reflect.DeepEqual(*updated, want) {
			t.Fatalf("updated goal: got %+v, want %+v", updated, want)
		}
		if updated.UpdatedAt.Before(original.UpdatedAt) || updated.UpdatedAt.Location() != time.UTC || updated.UpdatedAt.Nanosecond() != 0 {
			t.Fatalf("updated time: got %v, want nondecreasing UTC seconds", updated.UpdatedAt)
		}
		got, err := s.Get(t.Context(), "owner", original.GoalID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, updated) {
			t.Fatalf("persisted goal: got %+v, want %+v", got, updated)
		}
		active, err := s.ListActive(t.Context(), "owner")
		if err != nil {
			t.Fatal(err)
		}
		if len(active) != 0 {
			t.Fatalf("active goals: got %+v, want empty", active)
		}
		inactive, err := s.ListInactive(t.Context(), "owner")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(inactive, []data.Goal{*updated}) {
			t.Fatalf("inactive goals: got %+v, want %+v", inactive, updated)
		}
		input.IsActive = nil
		updated, err = s.Update(t.Context(), "owner", updated.GoalID, updated.Version, input)
		if err != nil {
			t.Fatal(err)
		}
		if updated.IsActive {
			t.Fatal("omitted activation: got true, want preserved false")
		}
		input.IsActive = &activeSetting

		activeSetting = true
		input.StartDate, input.EndDate, input.DefaultCadence = " ", "", " "
		cleared, err := s.Update(t.Context(), "owner", updated.GoalID, updated.Version, input)
		if err != nil {
			t.Fatal(err)
		}
		if !cleared.IsActive || cleared.StartDate != nil || cleared.EndDate != nil || cleared.DefaultCadence != nil {
			t.Fatalf("cleared goal: got %+v, want active with nil optional settings", cleared)
		}
		input.IsActive = nil
		cleared, err = s.Update(t.Context(), "owner", cleared.GoalID, cleared.Version, input)
		if err != nil {
			t.Fatal(err)
		}
		if !cleared.IsActive {
			t.Fatal("omitted activation: got false, want preserved true")
		}

		active, err = s.ListActive(t.Context(), "owner")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(active, []data.Goal{*cleared}) {
			t.Fatalf("reactivated list: got %+v, want %+v", active, cleared)
		}
	})
	t.Run("rejects stale inaccessible and duplicate mutations without changing the goal", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "owner", "other")
		s := goal.NewService(data.NewSQLiteGoalStore(db))
		original, err := s.Create(t.Context(), "owner", goal.CreateGoalInput{Name: "Reading", TargetSeconds: 60})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Create(t.Context(), "owner", goal.CreateGoalInput{Name: "Taken", TargetSeconds: 1}); err != nil {
			t.Fatal(err)
		}
		input := goal.UpdateGoalInput{Name: "Current", TargetSeconds: 120, Cadence: "daily", TZ: "UTC", WeekStart: "mon"}
		current, err := s.Update(t.Context(), "owner", original.GoalID, original.Version, input)
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			owner   string
			version int64
			want    error
		}{
			{"owner", original.Version, data.ErrVersionConflict}, {"other", current.Version, data.ErrRecordNotFound},
		} {
			if _, err := s.Update(t.Context(), tc.owner, current.GoalID, tc.version, input); !errors.Is(err, tc.want) {
				t.Fatalf("rejected update: got %v, want %v", err, tc.want)
			}
			if err := s.Delete(t.Context(), tc.owner, current.GoalID, tc.version); !errors.Is(err, tc.want) {
				t.Fatalf("rejected delete: got %v, want %v", err, tc.want)
			}
		}
		input.Name = "Taken"
		if _, err := s.Update(t.Context(), "owner", current.GoalID, current.Version, input); !errors.Is(err, data.ErrDuplicateRecord) {
			t.Fatalf("duplicate update: got %v, want ErrDuplicateRecord", err)
		}
		input.Name, input.StartDate = "PRIVATE-INPUT", "2026-19-39"
		_, err = s.Update(t.Context(), "owner", current.GoalID, current.Version, input)
		var validation *goal.ValidationError
		if !errors.As(err, &validation) {
			t.Fatalf("invalid date update: got %v, want ValidationError", err)
		}
		got, err := s.Get(t.Context(), "owner", current.GoalID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, current) {
			t.Fatalf("goal after rejected writes: got %+v, want %+v", got, current)
		}
	})
	t.Run("soft deletes and hides a goal while retaining the record", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "owner")
		s := goal.NewService(data.NewSQLiteGoalStore(db))
		original, err := s.Create(t.Context(), "owner", goal.CreateGoalInput{Name: "Reading", TargetSeconds: 60})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Delete(t.Context(), "owner", original.GoalID, original.Version); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Get(t.Context(), "owner", original.GoalID); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("get deleted goal: got %v, want ErrRecordNotFound", err)
		}
		for _, list := range []struct {
			name string
			call func(context.Context, string) ([]data.Goal, error)
		}{{"all", s.List}, {"active", s.ListActive}, {"inactive", s.ListInactive}} {
			items, err := list.call(t.Context(), "owner")
			if err != nil {
				t.Fatal(err)
			}
			if items == nil || len(items) != 0 {
				t.Fatalf("%s goals: got %+v, want non-nil empty slice", list.name, items)
			}
		}
		if err := s.Delete(t.Context(), "owner", original.GoalID, original.Version); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("repeated delete: got %v, want ErrRecordNotFound", err)
		}
		var retained int
		err = db.QueryRowContext(t.Context(), `SELECT count(*) FROM goals WHERE goal_id=? AND goal_name=? AND goal_is_active=1 AND deleted_at IS NOT NULL AND version=?`, original.GoalID, original.GoalName, original.Version+1).Scan(&retained)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := retained, 1; got != want {
			t.Fatalf("retained goal: got %d, want %d", got, want)
		}
	})
}

func TestGoalPriorityServiceWorkflow_SQLite(t *testing.T) {
	t.Run("retains omitted priority and activation and persists explicit changes", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "owner")
		service := goal.NewService(data.NewSQLiteGoalStore(db))
		original, err := service.Create(t.Context(), "owner", goal.CreateGoalInput{Name: "Goal", TargetSeconds: 60})
		if err != nil {
			t.Fatal(err)
		}
		if original.Priority != "normal" {
			t.Fatalf("default priority: got %q, want normal", original.Priority)
		}
		priority, active := " high ", false
		input := goal.UpdateGoalInput{Name: "Goal", TargetSeconds: 60, Cadence: "weekly", TZ: "UTC", WeekStart: "mon", Priority: &priority, IsActive: &active}
		updated, err := service.Update(t.Context(), "owner", original.GoalID, original.Version, input)
		if err != nil {
			t.Fatal(err)
		}
		input.Priority, input.IsActive = nil, nil
		input.Name = "Renamed"
		kept, err := service.Update(t.Context(), "owner", updated.GoalID, updated.Version, input)
		if err != nil {
			t.Fatal(err)
		}
		if kept.Priority != "high" || kept.IsActive {
			t.Fatalf("preserved settings: got priority %q/active %t, want high/false", kept.Priority, kept.IsActive)
		}
		if _, err := service.Update(t.Context(), "owner", kept.GoalID, updated.Version, input); !errors.Is(err, data.ErrVersionConflict) {
			t.Fatalf("stale omitted update: got %v, want conflict", err)
		}
		got, err := service.Get(t.Context(), "owner", kept.GoalID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, kept) {
			t.Fatalf("persisted goal: got %+v, want %+v", got, kept)
		}
	})
}
