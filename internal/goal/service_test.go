package goal

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

type storeStub struct {
	create                 func(context.Context, *data.Goal) (*data.Goal, error)
	get                    func(context.Context, string, int64) (*data.Goal, error)
	list, active, inactive func(context.Context, string) ([]data.Goal, error)
}

func TestService_Validation(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		change      func(*CreateInput)
	}{
		{"blank name", "goal_name", func(i *CreateInput) { i.Name = "   " }},
		{"long Unicode name", "goal_name", func(i *CreateInput) { i.Name = strings.Repeat("界", 513) }},
		{"zero target", "target_seconds", func(i *CreateInput) { i.TargetSeconds = 0 }},
		{"negative target", "target_seconds", func(i *CreateInput) { i.TargetSeconds = -1 }},
		{"unknown cadence", "cadence", func(i *CreateInput) { i.Cadence = "PRIVATE-INPUT" }},
		{"unknown default cadence", "default_cadence", func(i *CreateInput) { i.DefaultCadence = "PRIVATE-INPUT" }},
		{"unknown week start", "week_start", func(i *CreateInput) { i.WeekStart = "PRIVATE-INPUT" }},
		{"unknown timezone", "tz", func(i *CreateInput) { i.TZ = "PRIVATE-INPUT" }},
		{"machine dependent timezone", "tz", func(i *CreateInput) { i.TZ = "Local" }},
		{"overlong timezone", "tz", func(i *CreateInput) { i.TZ = strings.Repeat("x", 65) }},
		{"impossible month and day", "goal_start_date", func(i *CreateInput) { i.StartDate = "2026-19-39" }},
		{"non leap February", "goal_start_date", func(i *CreateInput) { i.StartDate = "2026-02-29" }},
		{"impossible end day", "goal_end_date", func(i *CreateInput) { i.EndDate = "2026-04-31" }},
		{"malformed start", "goal_start_date", func(i *CreateInput) { i.StartDate = "PRIVATE-INPUT" }},
		{"malformed end", "goal_end_date", func(i *CreateInput) { i.EndDate = "PRIVATE-INPUT" }},
		{"timestamp instead of date", "goal_start_date", func(i *CreateInput) { i.StartDate = "2026-09-01T00:00:00Z" }},
		{"reversed dates", "goal_end_date", func(i *CreateInput) { i.StartDate, i.EndDate = "2026-09-02", "2026-09-01" }},
	} {
		t.Run("rejects "+tc.name+" before persistence with safe feedback", func(t *testing.T) {
			input := CreateInput{Name: "PRIVATE-INPUT", TargetSeconds: 60}
			tc.change(&input)
			s := NewService(storeStub{create: func(context.Context, *data.Goal) (*data.Goal, error) {
				t.Fatal("unexpected persistence call")
				return nil, nil
			}})
			_, err := s.Create(t.Context(), "owner", input)
			var validation *ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("got %v, want ValidationError", err)
			}
			fields := validation.PublicFields()
			if fields[tc.field] == "" {
				t.Fatalf("got fields %v, want guidance for %s", fields, tc.field)
			}
			if strings.Contains(err.Error()+fmt.Sprint(fields), "PRIVATE-INPUT") {
				t.Fatal("validation feedback exposes private input")
			}
			if !maps.Equal(fields, validation.Fields) {
				t.Fatalf("got public fields %v, want %v", fields, validation.Fields)
			}
		})
	}
	t.Run("requires an owner before persistence", func(t *testing.T) {
		s := NewService(storeStub{})
		_, err := s.Create(t.Context(), "", CreateInput{Name: "Goal", TargetSeconds: 60})
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.PublicFields()["user_id"] == "" {
			t.Fatalf("got %v, want owner validation", err)
		}
	})
	t.Run("protects public guidance from mutable validation fields", func(t *testing.T) {
		_, err := NewService(storeStub{}).Create(t.Context(), "owner", CreateInput{})
		var validation *ValidationError
		if !errors.As(fmt.Errorf("PRIVATE-WRAPPER: %w", err), &validation) {
			t.Fatalf("got %v, want wrapped validation identity", err)
		}
		want := validation.PublicFields()
		validation.Fields["goal_name"] = "PRIVATE-INPUT"
		validation.Fields["PRIVATE-FIELD"] = "PRIVATE-INPUT"
		copy := validation.PublicFields()
		copy["goal_name"] = "PRIVATE-COPY"
		if got := validation.PublicFields(); !maps.Equal(got, want) {
			t.Fatalf("got public guidance %v, want %v", got, want)
		}
		if got := (&ValidationError{Fields: map[string]string{"PRIVATE-FIELD": "PRIVATE-INPUT"}}).PublicFields(); len(got) != 0 {
			t.Fatalf("got %v, want no untrusted public feedback", got)
		}
	})
	for _, tc := range []struct{ name, start, end string }{
		{"no dates", "", ""}, {"equal leap dates", "2024-02-29", "2024-02-29"},
		{"future start only", "2099-12-31", ""}, {"past end only", "", "2000-01-01"},
	} {
		t.Run("accepts "+tc.name, func(t *testing.T) {
			s := NewService(storeStub{create: func(_ context.Context, g *data.Goal) (*data.Goal, error) { return g, nil }})
			g, err := s.Create(t.Context(), "owner", CreateInput{Name: strings.Repeat("界", 512), TargetSeconds: 1, StartDate: tc.start, EndDate: tc.end})
			if err != nil {
				t.Fatal(err)
			}
			if (g.StartDate == nil) != (tc.start == "") || (g.EndDate == nil) != (tc.end == "") {
				t.Fatalf("got dates %v/%v, want nullability for %q/%q", g.StartDate, g.EndDate, tc.start, tc.end)
			}
		})
	}
	t.Run("accepts every schema cadence and week start", func(t *testing.T) {
		s := NewService(storeStub{create: func(_ context.Context, g *data.Goal) (*data.Goal, error) { return g, nil }})
		for _, cadence := range []string{"daily", "weekly", "monthly", "mtd", "quarterly", "yearly"} {
			g, err := s.Create(t.Context(), "owner", CreateInput{Name: "Goal", TargetSeconds: 1, Cadence: cadence, DefaultCadence: cadence})
			if err != nil || g.Cadence != cadence || g.DefaultCadence == nil || *g.DefaultCadence != cadence {
				t.Fatalf("got %v, %v, want cadence %q", g, err, cadence)
			}
		}
		for _, day := range []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"} {
			g, err := s.Create(t.Context(), "owner", CreateInput{Name: "Goal", TargetSeconds: 1, WeekStart: day})
			if err != nil || g.WeekStart != day {
				t.Fatalf("got %v, %v, want week start %q", g, err, day)
			}
		}
	})
}

func TestService_Reads(t *testing.T) {
	t.Run("get preserves arguments results and errors", func(t *testing.T) {
		item := &data.Goal{GoalID: 7}
		for _, wantErr := range []error{nil, fmt.Errorf("PRIVATE: %w", data.ErrRecordNotFound)} {
			s := NewService(storeStub{get: func(ctx context.Context, u string, id int64) (*data.Goal, error) {
				if ctx != t.Context() || u != "owner" || id != 7 {
					t.Fatalf("got %v/%s/%d, want supplied context/owner/7", ctx, u, id)
				}
				return item, wantErr
			}})
			got, err := s.Get(t.Context(), "owner", 7)
			if got != item || err != wantErr {
				t.Fatalf("got %v/%v, want original %v/%v", got, err, item, wantErr)
			}
		}
	})
	for _, kind := range []string{"all", "active", "inactive"} {
		t.Run(kind+" list delegates selection and preserves results and errors", func(t *testing.T) {
			for _, result := range []struct {
				items []data.Goal
				err   error
			}{
				{[]data.Goal{{GoalID: 7}}, nil}, {[]data.Goal{}, nil}, {nil, fmt.Errorf("PRIVATE: %w", data.ErrDatabaseBusy)},
			} {
				called := false
				list := func(ctx context.Context, u string) ([]data.Goal, error) {
					called = true
					if ctx != t.Context() || u != "owner" {
						t.Fatalf("got %v/%s, want supplied context/owner", ctx, u)
					}
					return result.items, result.err
				}
				stub := storeStub{}
				switch kind {
				case "all":
					stub.list = list
				case "active":
					stub.active = list
				case "inactive":
					stub.inactive = list
				}
				s := NewService(stub)
				call := s.List
				if kind == "active" {
					call = s.ListActive
				}
				if kind == "inactive" {
					call = s.ListInactive
				}
				got, err := call(t.Context(), "owner")
				if !called || !reflect.DeepEqual(got, result.items) || err != result.err {
					t.Fatalf("got %v/%v, want %v/%v from correct store method", got, err, result.items, result.err)
				}
			}
		})
	}
}

func (s storeStub) CreateGoal(ctx context.Context, g *data.Goal) (*data.Goal, error) {
	return s.create(ctx, g)
}
func (s storeStub) GetGoal(ctx context.Context, u string, id int64) (*data.Goal, error) {
	return s.get(ctx, u, id)
}
func (s storeStub) ListGoals(ctx context.Context, u string) ([]data.Goal, error) {
	return s.list(ctx, u)
}
func (s storeStub) ListActiveGoals(ctx context.Context, u string) ([]data.Goal, error) {
	return s.active(ctx, u)
}
func (s storeStub) ListInactiveGoals(ctx context.Context, u string) ([]data.Goal, error) {
	return s.inactive(ctx, u)
}

func TestService_Create(t *testing.T) {
	t.Run("defaults settings and timestamps without changing input", func(t *testing.T) {
		input := CreateInput{Name: "  Reading  ", TargetSeconds: 600}
		original := input
		now := time.Unix(1000, 999).In(time.FixedZone("offset", -7*3600))
		want := &data.Goal{GoalID: 7}
		calls := 0
		s := NewService(storeStub{create: func(ctx context.Context, g *data.Goal) (*data.Goal, error) {
			expected := data.Goal{UserID: "owner", GoalName: "Reading", TargetSeconds: 600,
				IsActive: true, Cadence: "weekly", TZ: "UTC", WeekStart: "mon", Version: 1,
				CreatedAt: time.Unix(1000, 0).UTC(), UpdatedAt: time.Unix(1000, 0).UTC()}
			if ctx != t.Context() || !reflect.DeepEqual(*g, expected) {
				t.Fatalf("got context %v and goal %+v, want supplied context and %+v", ctx, *g, expected)
			}
			return want, nil
		}})
		s.now = func() time.Time { calls++; return now }
		got, err := s.Create(t.Context(), "owner", input)
		if got != want || err != nil || calls != 1 || input != original {
			t.Fatalf("got %v, %v, %d clock calls, input %+v; want original result, nil, 1, %+v", got, err, calls, input, original)
		}
	})
	t.Run("preserves explicit settings and inactive creation", func(t *testing.T) {
		active := false
		input := CreateInput{Name: " Goal ", TargetSeconds: 60, IsActive: &active,
			StartDate: " 2024-02-29 ", EndDate: " 2030-12-31 ", Cadence: " daily ",
			TZ: " America/Los_Angeles ", WeekStart: " sun ", DefaultCadence: " monthly "}
		snapshot := input
		s := NewService(storeStub{create: func(_ context.Context, g *data.Goal) (*data.Goal, error) {
			if g.IsActive || g.GoalName != "Goal" || g.Cadence != "daily" || g.TZ != "America/Los_Angeles" || g.WeekStart != "sun" || g.StartDate == nil || *g.StartDate != "2024-02-29" || g.EndDate == nil || *g.EndDate != "2030-12-31" || g.DefaultCadence == nil || *g.DefaultCadence != "monthly" {
				t.Fatalf("got settings %+v, want normalized explicit settings", g)
			}
			return g, nil
		}})
		if _, err := s.Create(t.Context(), "owner", input); err != nil {
			t.Fatal(err)
		}
		if input != snapshot || active {
			t.Fatalf("got changed input %+v, want %+v and inactive", input, snapshot)
		}
	})
	t.Run("returns the original store failure", func(t *testing.T) {
		want := errors.Join(data.ErrDuplicateRecord, errors.New("PRIVATE-STORE"))
		s := NewService(storeStub{create: func(context.Context, *data.Goal) (*data.Goal, error) { return nil, want }})
		if _, got := s.Create(t.Context(), "owner", CreateInput{Name: "Goal", TargetSeconds: 1}); got != want {
			t.Fatalf("got %v, want original %v", got, want)
		}
	})
}
