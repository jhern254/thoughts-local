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
	update                 func(context.Context, *data.Goal) (*data.Goal, error)
	delete                 func(context.Context, string, int64, int64) error
	create                 func(context.Context, *data.Goal) (*data.Goal, error)
	get                    func(context.Context, string, int64) (*data.Goal, error)
	list, active, inactive func(context.Context, string) ([]data.Goal, error)
}

func TestService_Validation(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		change      func(*CreateGoalInput)
	}{
		{"blank name", "goal_name", func(i *CreateGoalInput) { i.Name = "   " }},
		{"long Unicode name", "goal_name", func(i *CreateGoalInput) { i.Name = strings.Repeat("界", 513) }},
		{"zero target", "target_seconds", func(i *CreateGoalInput) { i.TargetSeconds = 0 }},
		{"negative target", "target_seconds", func(i *CreateGoalInput) { i.TargetSeconds = -1 }},
		{"unknown cadence", "cadence", func(i *CreateGoalInput) { i.Cadence = "PRIVATE-INPUT" }},
		{"unknown default cadence", "default_cadence", func(i *CreateGoalInput) { i.DefaultCadence = "PRIVATE-INPUT" }},
		{"unknown week start", "week_start", func(i *CreateGoalInput) { i.WeekStart = "PRIVATE-INPUT" }},
		{"unknown timezone", "tz", func(i *CreateGoalInput) { i.TZ = "PRIVATE-INPUT" }},
		{"machine dependent timezone", "tz", func(i *CreateGoalInput) { i.TZ = "Local" }},
		{"overlong timezone", "tz", func(i *CreateGoalInput) { i.TZ = strings.Repeat("x", 65) }},
		{"impossible month and day", "goal_start_date", func(i *CreateGoalInput) { i.StartDate = "2026-19-39" }},
		{"non leap February", "goal_start_date", func(i *CreateGoalInput) { i.StartDate = "2026-02-29" }},
		{"impossible end day", "goal_end_date", func(i *CreateGoalInput) { i.EndDate = "2026-04-31" }},
		{"malformed start", "goal_start_date", func(i *CreateGoalInput) { i.StartDate = "PRIVATE-INPUT" }},
		{"malformed end", "goal_end_date", func(i *CreateGoalInput) { i.EndDate = "PRIVATE-INPUT" }},
		{"timestamp instead of date", "goal_start_date", func(i *CreateGoalInput) { i.StartDate = "2026-09-01T00:00:00Z" }},
		{"reversed dates", "goal_end_date", func(i *CreateGoalInput) { i.StartDate, i.EndDate = "2026-09-02", "2026-09-01" }},
	} {
		for _, operation := range []string{"create", "update"} {
			t.Run(operation+" rejects "+tc.name+" before persistence with safe feedback", func(t *testing.T) {
				input := CreateGoalInput{Name: "PRIVATE-INPUT", TargetSeconds: 60, Cadence: "weekly", TZ: "UTC", WeekStart: "mon"}
				tc.change(&input)
				s := NewService(storeStub{})
				var err error
				if operation == "create" {
					_, err = s.Create(t.Context(), "owner", input)
				} else {
					_, err = s.Update(t.Context(), "owner", 7, 1, UpdateGoalInput{Name: input.Name, TargetSeconds: input.TargetSeconds, StartDate: input.StartDate, EndDate: input.EndDate, Cadence: input.Cadence, TZ: input.TZ, WeekStart: input.WeekStart, DefaultCadence: input.DefaultCadence})
				}
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
	}

	t.Run("requires an owner before persistence", func(t *testing.T) {
		s := NewService(storeStub{})
		_, err := s.Create(t.Context(), "", CreateGoalInput{Name: "Goal", TargetSeconds: 60})
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.PublicFields()["user_id"] == "" {
			t.Fatalf("got %v, want owner validation", err)
		}
	})
	t.Run("protects public guidance from mutable validation fields", func(t *testing.T) {
		_, err := NewService(storeStub{}).Create(t.Context(), "owner", CreateGoalInput{})
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
			g, err := s.Create(t.Context(), "owner", CreateGoalInput{Name: strings.Repeat("界", 512), TargetSeconds: 1, StartDate: tc.start, EndDate: tc.end})
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
			g, err := s.Create(t.Context(), "owner", CreateGoalInput{Name: "Goal", TargetSeconds: 1, Cadence: cadence, DefaultCadence: cadence})
			if err != nil || g.Cadence != cadence || g.DefaultCadence == nil || *g.DefaultCadence != cadence {
				t.Fatalf("got %v, %v, want cadence %q", g, err, cadence)
			}
		}
		for _, day := range []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"} {
			g, err := s.Create(t.Context(), "owner", CreateGoalInput{Name: "Goal", TargetSeconds: 1, WeekStart: day})
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
		input := CreateGoalInput{Name: "  Reading  ", TargetSeconds: 600}
		original := input
		now := time.Unix(1000, 999).In(time.FixedZone("offset", -7*3600))
		want := &data.Goal{GoalID: 7}
		calls := 0
		s := NewService(storeStub{create: func(ctx context.Context, g *data.Goal) (*data.Goal, error) {
			expected := data.Goal{Priority: "normal", UserID: "owner", GoalName: "Reading", TargetSeconds: 600,
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
		input := CreateGoalInput{Name: " Goal ", TargetSeconds: 60, IsActive: &active,
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
		if _, got := s.Create(t.Context(), "owner", CreateGoalInput{Name: "Goal", TargetSeconds: 1}); got != want {
			t.Fatalf("got %v, want original %v", got, want)
		}
	})
}

func (s storeStub) UpdateGoal(ctx context.Context, g *data.Goal) (*data.Goal, error) {
	return s.update(ctx, g)
}
func (s storeStub) DeleteGoal(ctx context.Context, owner string, id, version int64) error {
	return s.delete(ctx, owner, id, version)
}

func TestService_Update(t *testing.T) {
	t.Run("replaces explicit settings without modifying input", func(t *testing.T) {
		for _, active := range []bool{false, true} {
			priority := "normal"
			input := UpdateGoalInput{Priority: &priority, Name: " Reading ", TargetSeconds: 120, StartDate: " 2024-02-29 ", EndDate: " 2030-01-01 ", IsActive: &active, Cadence: " monthly ", TZ: " America/Los_Angeles ", WeekStart: " sun ", DefaultCadence: " daily "}
			original := input
			now := time.Unix(1000, 999).In(time.FixedZone("offset", -7*3600))
			start, end, cadence := "2024-02-29", "2030-01-01", "daily"
			want := &data.Goal{GoalID: 7, Version: 4}
			calls := 0
			s := NewService(storeStub{update: func(ctx context.Context, g *data.Goal) (*data.Goal, error) {
				expected := data.Goal{Priority: "normal", GoalID: 7, UserID: "owner", GoalName: "Reading", TargetSeconds: 120, StartDate: &start, EndDate: &end, IsActive: active, Cadence: "monthly", TZ: "America/Los_Angeles", WeekStart: "sun", DefaultCadence: &cadence, Version: 3, UpdatedAt: time.Unix(1000, 0).UTC()}
				if ctx != t.Context() || !reflect.DeepEqual(*g, expected) {
					t.Fatalf("got %+v, want %+v with original context", g, expected)
				}
				return want, nil
			}})
			s.now = func() time.Time { calls++; return now }
			got, err := s.Update(t.Context(), "owner", 7, 3, input)
			if got != want || err != nil || calls != 1 || input != original || *input.IsActive != active {
				t.Fatalf("got %v/%v, %d clock calls and input %+v, want original result/nil, one clock call and %+v", got, err, calls, input, original)
			}
		}
	})
	t.Run("omitted activation preserves the current state and caller version", func(t *testing.T) {
		for _, active := range []bool{false, true} {
			current := &data.Goal{Priority: "normal", GoalID: 7, Version: 3, IsActive: active}
			snapshot := *current
			s := NewService(storeStub{
				get: func(ctx context.Context, owner string, id int64) (*data.Goal, error) {
					if ctx != t.Context() || owner != "owner" || id != 7 {
						t.Fatal("get did not receive original context and identity")
					}
					return current, nil
				},
				update: func(_ context.Context, item *data.Goal) (*data.Goal, error) {
					if item.IsActive != active || item.Version != 2 {
						t.Fatalf("got active=%t/version=%d, want %t/2", item.IsActive, item.Version, active)
					}
					return nil, data.ErrVersionConflict
				},
			})
			input := UpdateGoalInput{Name: "Goal", TargetSeconds: 1, Cadence: "daily", TZ: "UTC", WeekStart: "mon"}
			_, err := s.Update(t.Context(), "owner", 7, 2, input)
			if !errors.Is(err, data.ErrVersionConflict) || input.IsActive != nil || *current != snapshot {
				t.Fatalf("got %v and %+v, want conflict and unchanged input/current record", err, current)
			}
		}
	})
	t.Run("returns the original lookup failure without writing when optional settings are omitted", func(t *testing.T) {
		want := fmt.Errorf("PRIVATE-LOOKUP: %w", data.ErrDatabaseBusy)
		s := NewService(storeStub{get: func(context.Context, string, int64) (*data.Goal, error) { return nil, want }})
		for _, active := range []*bool{nil, new(bool)} {
			_, got := s.Update(t.Context(), "owner", 7, 2, UpdateGoalInput{Name: "Goal", TargetSeconds: 1, Cadence: "daily", TZ: "UTC", WeekStart: "mon", IsActive: active})
			if got != want {
				t.Fatalf("got %v, want original %v", got, want)
			}
		}
	})

	t.Run("clears empty optional settings", func(t *testing.T) {
		priority := "normal"
		active := false
		s := NewService(storeStub{update: func(_ context.Context, g *data.Goal) (*data.Goal, error) {
			if g.StartDate != nil || g.EndDate != nil || g.DefaultCadence != nil {
				t.Fatalf("got %+v, want nil optional settings", g)
			}
			return g, nil
		}})
		_, err := s.Update(t.Context(), "owner", 7, 1, UpdateGoalInput{Priority: &priority, Name: "Goal", IsActive: &active, TargetSeconds: 1, Cadence: "daily", TZ: "UTC", WeekStart: "mon", StartDate: " ", EndDate: " ", DefaultCadence: " "})
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("requires explicit settings instead of applying creation defaults", func(t *testing.T) {
		_, err := NewService(storeStub{}).Update(t.Context(), "owner", 7, 1, UpdateGoalInput{Name: "PRIVATE-INPUT", TargetSeconds: 1})
		var validation *ValidationError
		if !errors.As(err, &validation) {
			t.Fatalf("got %v, want ValidationError", err)
		}
		for _, field := range []string{"cadence", "tz", "week_start"} {
			if validation.PublicFields()[field] == "" {
				t.Fatalf("got %v, want guidance for %s", validation.PublicFields(), field)
			}
		}
	})
}

func TestService_Mutations(t *testing.T) {
	for _, operation := range []string{"update", "delete"} {
		t.Run(operation, func(t *testing.T) {
			call := func(ctx context.Context, s *Service, owner string, version int64) error {
				priority := "normal"
				active := true
				if operation == "delete" {
					return s.Delete(ctx, owner, 7, version)
				}
				_, err := s.Update(ctx, owner, 7, version, UpdateGoalInput{Priority: &priority, Name: "PRIVATE-INPUT", IsActive: &active, TargetSeconds: 1, Cadence: "daily", TZ: "UTC", WeekStart: "mon"})
				return err
			}
			t.Run("rejects missing owner and nonpositive versions before persistence", func(t *testing.T) {
				for _, tc := range []struct {
					owner, field string
					version      int64
				}{{"", "user_id", 1}, {"owner", "version", 0}, {"owner", "version", -1}} {
					err := call(t.Context(), NewService(storeStub{}), tc.owner, tc.version)
					var validation *ValidationError
					if !errors.As(err, &validation) || validation.PublicFields()[tc.field] == "" {
						t.Fatalf("got %v, want validation for %s", err, tc.field)
					}
					if strings.Contains(fmt.Sprint(validation.PublicFields())+err.Error(), "PRIVATE-INPUT") {
						t.Fatal("private input exposed")
					}
				}
			})
			t.Run("preserves arguments and original store outcomes", func(t *testing.T) {
				for _, cause := range []error{nil, data.ErrRecordNotFound, data.ErrVersionConflict, data.ErrDuplicateRecord, context.Canceled, data.ErrDatabaseBusy} {
					var want error
					if cause != nil {
						want = fmt.Errorf("PRIVATE-STORE: %w", cause)
					}
					called := false
					s := NewService(storeStub{
						update: func(ctx context.Context, g *data.Goal) (*data.Goal, error) {
							called = true
							if ctx != t.Context() || g.UserID != "owner" || g.GoalID != 7 || g.Version != 2 {
								t.Fatalf("got %+v, want supplied owner/id/version/context", g)
							}
							return nil, want
						},
						delete: func(ctx context.Context, owner string, id, version int64) error {
							called = true
							if ctx != t.Context() || owner != "owner" || id != 7 || version != 2 {
								t.Fatalf("got %s/%d/%d, want owner/7/2 and original context", owner, id, version)
							}
							return want
						},
					})
					if got := call(t.Context(), s, "owner", 2); got != want || !called {
						t.Fatalf("got %v, called %t, want original %v and store call", got, called, want)
					}
				}
			})
		})
	}
}

func TestService_Priority(t *testing.T) {
	t.Run("creates with a default or normalized explicit priority", func(t *testing.T) {
		for _, tc := range []struct{ input, want string }{{"", "normal"}, {"   ", "normal"}, {" low ", "low"}, {"normal", "normal"}, {" high ", "high"}} {
			s := NewService(storeStub{create: func(_ context.Context, g *data.Goal) (*data.Goal, error) { return g, nil }})
			got, err := s.Create(t.Context(), "owner", CreateGoalInput{Name: "Goal", TargetSeconds: 1, Priority: tc.input})
			if err != nil {
				t.Fatal(err)
			}
			if got.Priority != tc.want {
				t.Fatalf("priority: got %q, want %q", got.Priority, tc.want)
			}
		}
	})
	t.Run("rejects invalid explicit priorities before reading or writing", func(t *testing.T) {
		for _, priority := range []string{"", " ", "PRIVATE-PRIORITY", "HIGH"} {
			s := NewService(storeStub{})
			_, err := s.Update(t.Context(), "owner", 7, 1, UpdateGoalInput{Name: "Goal", TargetSeconds: 1, Cadence: "daily", TZ: "UTC", WeekStart: "mon", Priority: &priority})
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.PublicFields()["priority"] != "must be low, normal, or high" {
				t.Fatalf("got %v, want safe priority guidance", err)
			}
			if strings.Contains(fmt.Sprint(validation.PublicFields())+err.Error(), "PRIVATE-PRIORITY") {
				t.Fatal("private priority exposed")
			}
			if strings.Trim(priority, " ") != "" {
				_, err = s.Create(t.Context(), "owner", CreateGoalInput{Name: "Goal", TargetSeconds: 1, Priority: priority})
				if !errors.As(err, &validation) || validation.PublicFields()["priority"] == "" {
					t.Fatalf("create: got %v, want priority validation", err)
				}
			}
		}
	})
	t.Run("preserves omitted fields with one lookup and retains expected version", func(t *testing.T) {
		priority := " low "
		for _, tc := range []struct {
			name         string
			priority     *string
			active       *bool
			wantPriority string
			wantActive   bool
			reads        int
		}{
			{"both omitted", nil, nil, "high", true, 1},
			{"priority omitted", nil, new(bool), "high", false, 1},
			{"activation omitted", &priority, nil, "low", true, 1},
		} {
			t.Run(tc.name, func(t *testing.T) {
				reads := 0
				current := &data.Goal{Priority: "high", IsActive: true, Version: 9}
				snapshot := *current
				s := NewService(storeStub{get: func(ctx context.Context, owner string, id int64) (*data.Goal, error) {
					reads++
					if ctx != t.Context() || owner != "owner" || id != 7 {
						t.Fatal("lookup arguments changed")
					}
					return current, nil
				}, update: func(_ context.Context, g *data.Goal) (*data.Goal, error) {
					if g.Priority != tc.wantPriority || g.IsActive != tc.wantActive || g.Version != 2 {
						t.Fatalf("got %+v, want priority %s/active %t/version 2", g, tc.wantPriority, tc.wantActive)
					}
					return nil, data.ErrVersionConflict
				}})
				input := UpdateGoalInput{Name: "Goal", TargetSeconds: 1, Cadence: "daily", TZ: "UTC", WeekStart: "mon", Priority: tc.priority, IsActive: tc.active}
				_, err := s.Update(t.Context(), "owner", 7, 2, input)
				if !errors.Is(err, data.ErrVersionConflict) || reads != tc.reads || *current != snapshot {
					t.Fatalf("got %v/%d reads, want conflict/%d reads and unchanged record", err, reads, tc.reads)
				}
			})
		}
	})
	t.Run("updates explicit priorities without reading or mutating input", func(t *testing.T) {
		for _, priority := range []string{" low ", " normal ", " high "} {
			original := priority
			active := false
			s := NewService(storeStub{update: func(_ context.Context, g *data.Goal) (*data.Goal, error) { return g, nil }})
			got, err := s.Update(t.Context(), "owner", 7, 2, UpdateGoalInput{Name: "Goal", TargetSeconds: 1, Cadence: "daily", TZ: "UTC", WeekStart: "mon", Priority: &priority, IsActive: &active})
			if err != nil {
				t.Fatal(err)
			}
			if got.Priority != strings.Trim(original, " ") || priority != original || active {
				t.Fatalf("got %+v with input %q, want normalized priority and unchanged input", got, priority)
			}
		}
	})
}
