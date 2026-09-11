package event

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

func (s storeStub) EndEvent(ctx context.Context, u string, id, version int64, end, updated time.Time) (*data.Event, error) {
	return s.end(ctx, u, id, version, end, updated)
}

func (s storeStub) UpdateEvent(ctx context.Context, item *data.Event) (*data.Event, error) {
	return s.update(ctx, item)
}

func TestService_End(t *testing.T) {
	for _, supplied := range []time.Time{{}, time.Unix(900, 123).In(time.FixedZone("offset", 3600))} {
		name := "defaults the end to one captured now"
		if !supplied.IsZero() {
			name = "preserves an explicit end instant"
		}
		t.Run(name, func(t *testing.T) {
			wantEnd := int64(1000)
			if !supplied.IsZero() {
				wantEnd = 900
			}
			want := &data.Event{EventID: 7}
			cause := errors.Join(data.ErrDatabaseBusy, errors.New("PRIVATE_MARKER"))
			s := NewService(storeStub{end: func(ctx context.Context, u string, id, version int64, end, updated time.Time) (*data.Event, error) {
				if ctx != t.Context() || u != "u" || id != 7 || version != 3 || end.Unix() != wantEnd || end.Nanosecond() != 0 || end.Location() != time.UTC || updated != time.Unix(1000, 0).UTC() {
					t.Fatalf("unexpected end request: %s %d %d %v %v", u, id, version, end, updated)
				}
				return want, cause
			}})
			calls := 0
			s.now = func() time.Time { calls++; return time.Unix(1000, 999) }
			got, err := s.End(t.Context(), "u", 7, 3, supplied)
			if got != want || err != cause || calls != 1 {
				t.Fatalf("got %v, %v, %d clock calls, want original result/error and one call", got, err, calls)
			}
		})
	}
	t.Run("rejects invalid requests before writing", func(t *testing.T) {
		s := NewService(storeStub{})
		s.now = func() time.Time { return time.Unix(1000, 0) }
		for _, input := range []struct {
			user    string
			version int64
			end     time.Time
		}{
			{"", 1, time.Time{}}, {"u", 0, time.Time{}}, {"u", 1, time.Unix(1001, 0)},
		} {
			var validation *ValidationError
			if _, err := s.End(t.Context(), input.user, 7, input.version, input.end); !errors.As(err, &validation) {
				t.Fatalf("got %v, want validation", err)
			}
		}
	})
}

func TestService_Update(t *testing.T) {
	t.Run("normalizes a correction without mutating caller timestamps and preserves failures", func(t *testing.T) {
		end := time.Unix(900, 123).In(time.FixedZone("offset", 3600))
		original := end
		cause := errors.Join(data.ErrEventVersionConflict, errors.New("PRIVATE_MARKER"))
		want := &data.Event{EventID: 7}
		s := NewService(storeStub{update: func(ctx context.Context, item *data.Event) (*data.Event, error) {
			if ctx != t.Context() || item.UserID != "u" || item.EventID != 7 || item.Version != 3 || item.ActivityType == nil || *item.ActivityType != "label" || item.StartedAt != time.Unix(800, 0).UTC() || item.EndedAt == nil || *item.EndedAt != time.Unix(900, 0).UTC() || item.UpdatedAt != time.Unix(1000, 0).UTC() {
				t.Fatalf("unexpected correction: %+v", item)
			}
			return want, cause
		}})
		calls := 0
		s.now = func() time.Time { calls++; return time.Unix(1000, 999) }
		got, err := s.Update(t.Context(), "u", 7, 3, " label ", time.Unix(800, 123), &end)
		if got != want || err != cause || end != original || calls != 1 {
			t.Fatalf("got %v, %v, end %v, clock calls %d; want preserved result/error/input and one call", got, err, end, calls)
		}
	})
	t.Run("retains ongoing state and permits clearing the label", func(t *testing.T) {
		s := NewService(storeStub{update: func(_ context.Context, item *data.Event) (*data.Event, error) {
			if item.EndedAt != nil || item.ActivityType != nil {
				t.Fatalf("got %+v, want untitled ongoing correction", item)
			}
			return item, nil
		}})
		if _, err := s.Update(t.Context(), "u", 7, 1, "   ", time.Unix(800, 0), nil); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("rejects invalid corrections and exposes only fixed guidance", func(t *testing.T) {
		s := NewService(storeStub{})
		s.now = func() time.Time { return time.Unix(1000, 0) }
		for _, input := range []struct {
			name, user, label string
			version           int64
			start, end        time.Time
		}{
			{"missing user", "", "", 1, time.Unix(800, 0), time.Unix(900, 0)},
			{"invalid version", "u", "", 0, time.Unix(800, 0), time.Unix(900, 0)},
			{"missing start", "u", "", 1, time.Time{}, time.Unix(900, 0)},
			{"missing end", "u", "", 1, time.Unix(800, 0), time.Time{}},
			{"future start", "u", "", 1, time.Unix(1001, 0), time.Unix(1002, 0)},
			{"future end", "u", "", 1, time.Unix(800, 0), time.Unix(1001, 0)},
			{"reversed interval", "u", "", 1, time.Unix(900, 0), time.Unix(800, 0)},
			{"long label", "u", strings.Repeat("界", 4097) + "PRIVATE_MARKER", 1, time.Unix(800, 0), time.Unix(900, 0)},
		} {
			t.Run(input.name, func(t *testing.T) {
				_, err := s.Update(t.Context(), input.user, 7, input.version, input.label, input.start, &input.end)
				var validation *ValidationError
				if !errors.As(err, &validation) {
					t.Fatalf("got %v, want validation", err)
				}
				for field := range validation.Fields {
					validation.Fields[field] = "PRIVATE_MARKER"
				}
				for _, message := range validation.PublicFields() {
					if strings.Contains(message, "PRIVATE_MARKER") {
						t.Fatal("private input entered public guidance")
					}
				}
			})
		}
	})
}
