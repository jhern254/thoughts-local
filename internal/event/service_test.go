package event

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

type storeStub struct {
	end    func(context.Context, string, int64, int64, time.Time, time.Time) (*data.Event, error)
	update func(context.Context, *data.Event) (*data.Event, error)
	start  func(context.Context, *data.Event) (*data.Event, error)
	past   func(context.Context, *data.Event) (*data.Event, error)
	get    func(context.Context, string, int64) (*data.Event, error)
	list   func(context.Context, string, time.Time, time.Time) ([]data.Event, error)
}

func (s storeStub) StartEvent(ctx context.Context, e *data.Event) (*data.Event, error) {
	return s.start(ctx, e)
}
func (s storeStub) AddPastEvent(ctx context.Context, e *data.Event) (*data.Event, error) {
	return s.past(ctx, e)
}
func (s storeStub) GetEvent(ctx context.Context, u string, id int64) (*data.Event, error) {
	return s.get(ctx, u, id)
}
func (s storeStub) ListEvents(ctx context.Context, u string, from, until time.Time) ([]data.Event, error) {
	return s.list(ctx, u, from, until)
}

func TestService_Create(t *testing.T) {
	t.Run("preserves the original store failure", func(t *testing.T) {
		want := errors.Join(data.ErrDatabaseReadOnly, errors.New("PRIVATE_MARKER"))
		service := NewService(storeStub{start: func(context.Context, *data.Event) (*data.Event, error) {
			return nil, want
		}}, nil)
		if _, err := service.Create(t.Context(), "u", "", time.Time{}); err != want {
			t.Fatalf("got %v, want original store failure", err)
		}
	})
	t.Run("defaults once and dispatches an ongoing event with normalized metadata", func(t *testing.T) {
		ctx := t.Context()
		now := time.Unix(1000, 900).In(time.FixedZone("offset", -7*3600))
		want := &data.Event{EventID: 7}
		service := NewService(storeStub{start: func(got context.Context, item *data.Event) (*data.Event, error) {
			if got != ctx || item.UserID != "u" || item.EventID != 0 || item.Version != 1 || item.EndedAt != nil || item.ActivityType == nil || *item.ActivityType != "activity" {
				t.Fatalf("got event %+v, want scoped ongoing activity", item)
			}
			for _, stamp := range []time.Time{item.StartedAt, item.CreatedAt, item.UpdatedAt} {
				if stamp.Unix() != 1000 || stamp.Nanosecond() != 0 || stamp.Location() != time.UTC {
					t.Fatalf("got timestamp %v, want UTC second 1000", stamp)
				}
			}
			return want, nil
		}}, nil)
		calls := 0
		service.now = func() time.Time { calls++; return now }
		got, err := service.Create(ctx, "u", " activity ", time.Time{})
		if err != nil || got != want || calls != 1 {
			t.Fatalf("got %v, %v, %d clock calls, want original result, nil, 1", got, err, calls)
		}
	})
	t.Run("preserves explicit starts and applies the existing optional label contract", func(t *testing.T) {
		for _, label := range []string{"", "   ", "\t", strings.Repeat("界", 4096), "a\x00" + strings.Repeat("b", 4096)} {
			service := NewService(storeStub{start: func(_ context.Context, item *data.Event) (*data.Event, error) {
				if item.StartedAt.Unix() != 100 || item.StartedAt.Nanosecond() != 0 || item.StartedAt.Location() != time.UTC {
					t.Fatalf("got start %v, want UTC second 100", item.StartedAt)
				}
				want := strings.Trim(label, " ")
				if want == "" {
					if item.ActivityType != nil {
						t.Fatal("got label, want nil")
					}
				} else if item.ActivityType == nil || *item.ActivityType != want {
					t.Fatal("got rewritten label, want complete normalized label")
				}
				return item, nil
			}}, nil)
			service.now = func() time.Time { return time.Unix(1000, 0) }
			if _, err := service.Create(t.Context(), "u", label, time.Unix(100, 999).In(time.FixedZone("offset", 3600))); err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Run("rejects invalid input before persistence with safe copied guidance", func(t *testing.T) {
		for _, tc := range []struct {
			user, label, field string
			start              time.Time
		}{
			{"", "label", "user_id", time.Time{}},
			{"u", "PRIVATE_MARKER" + strings.Repeat("界", 4096), "activity_type", time.Time{}},
			{"u", "label", "started_at", time.Unix(1001, 0)},
			{"u", "\x00private", "activity_type", time.Time{}},
		} {
			service := NewService(storeStub{}, nil)
			service.now = func() time.Time { return time.Unix(1000, 0) }
			_, err := service.Create(t.Context(), tc.user, tc.label, tc.start)
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.PublicFields()[tc.field] == "" {
				t.Fatalf("got error %v, want validation for %s", err, tc.field)
			}
			if strings.Contains(err.Error(), "PRIVATE_MARKER") {
				t.Fatal("error exposed private input")
			}
			for _, msg := range validation.PublicFields() {
				if strings.Contains(msg, "PRIVATE_MARKER") {
					t.Fatal("guidance exposed private input")
				}
			}
			validation.Fields[tc.field] = "PRIVATE_MARKER"
			copy := validation.PublicFields()
			copy[tc.field] = "PRIVATE_MARKER"
			if validation.PublicFields()[tc.field] == "PRIVATE_MARKER" {
				t.Fatal("public guidance shares mutable state")
			}
		}
	})
}

func TestService_CreatePast(t *testing.T) {
	t.Run("requires explicit valid times before persistence", func(t *testing.T) {
		for _, bounds := range [][2]time.Time{{{}, time.Unix(100, 0)}, {time.Unix(100, 0), {}}, {time.Unix(200, 0), time.Unix(100, 0)}, {time.Unix(100, 0), time.Unix(1001, 0)}} {
			service := NewService(storeStub{}, nil)
			service.now = func() time.Time { return time.Unix(1000, 0) }
			_, err := service.CreatePast(t.Context(), "u", "", bounds[0], bounds[1])
			var validation *ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("got error %v, want validation", err)
			}
		}
	})
	t.Run("allows equal timestamps after normalization and preserves original errors", func(t *testing.T) {
		ctx := t.Context()
		want := errors.Join(data.ErrEventOverlap, errors.New("private cause"))
		result := &data.Event{EventID: 7}
		service := NewService(storeStub{past: func(got context.Context, item *data.Event) (*data.Event, error) {
			if got != ctx || item.UserID != "u" || item.EndedAt == nil || !item.StartedAt.Equal(*item.EndedAt) || item.StartedAt.Unix() != 1000 {
				t.Fatalf("got event %+v, want equal scoped interval at second 1000", item)
			}
			return result, want
		}}, nil)
		service.now = func() time.Time { return time.Unix(1000, 0) }
		got, err := service.CreatePast(ctx, "u", "", time.Unix(1000, 900), time.Unix(1000, 100))
		if got != result || err != want {
			t.Fatalf("got %v, %v, want original result/error", got, err)
		}
	})
}

func TestService_Get(t *testing.T) {
	t.Run("get forwards scope without inventing ID restrictions", func(t *testing.T) {
		ctx := t.Context()
		want := data.ErrRecordNotFound
		service := NewService(storeStub{get: func(got context.Context, u string, id int64) (*data.Event, error) {
			if got != ctx || u != "u" || id != 0 {
				t.Fatal("get lost scope or ID")
			}
			return nil, want
		}}, nil)
		if _, err := service.Get(ctx, "u", 0); err != want {
			t.Fatalf("got %v, want original not-found error", err)
		}
	})
}

func TestService_List(t *testing.T) {
	t.Run("list preserves results errors and future range bounds", func(t *testing.T) {
		ctx := t.Context()
		want := errors.Join(data.ErrDatabaseBusy, context.Canceled)
		from := time.Date(2100, 1, 1, 0, 0, 0, 900, time.FixedZone("offset", 3600))
		until := from.Add(time.Hour)
		service := NewService(storeStub{list: func(got context.Context, u string, a, b time.Time) ([]data.Event, error) {
			if got != ctx || u != "u" || a.Unix() != from.Unix() || b.Unix() != until.Unix() || a.Location() != time.UTC || a.Nanosecond() != 0 {
				t.Fatal("list lost scope or normalized bounds")
			}
			return []data.Event{{EventID: 7}}, want
		}}, nil)
		got, err := service.List(ctx, "u", from, until)
		if len(got) != 1 || got[0].EventID != 7 || err != want {
			t.Fatalf("got %v, %v, want original rows/error", got, err)
		}
	})
	t.Run("rejects absent reversed and subsecond empty ranges before persistence", func(t *testing.T) {
		for _, bounds := range [][2]time.Time{{{}, time.Unix(200, 0)}, {time.Unix(100, 0), {}}, {time.Unix(200, 0), time.Unix(100, 0)}, {time.Unix(100, 0), time.Unix(100, 999)}} {
			_, err := NewService(storeStub{}, nil).List(t.Context(), "u", bounds[0], bounds[1])
			var validation *ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("got %v, want validation", err)
			}
		}
	})
}
