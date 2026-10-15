package timeline

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

type dayEvents struct {
	EventReader
	list func(context.Context, string, time.Time, time.Time) ([]data.Event, error)
}

func (s dayEvents) List(ctx context.Context, user string, from, until time.Time) ([]data.Event, error) {
	return s.list(ctx, user, from, until)
}

type dayMetrics struct {
	MetricsReader
	groups func(context.Context, string, time.Time, time.Time, time.Time) ([]data.MiscThoughtGroup, error)
}

func (s dayMetrics) MiscThoughtGroups(ctx context.Context, user string, from, until, ongoingUntil time.Time) ([]data.MiscThoughtGroup, error) {
	return s.groups(ctx, user, from, until, ongoingUntil)
}

func TestService_Day(t *testing.T) {
	t.Run("merges real and generated entries chronologically while retaining distinct identities", func(t *testing.T) {
		from, until := time.Unix(100, 0).UTC(), time.Unix(500, 0).UTC()
		events := dayEvents{list: func(ctx context.Context, user string, start, end time.Time) ([]data.Event, error) {
			if ctx != t.Context() || user != "u" || start != from || end != until {
				t.Fatal("event list lost scope")
			}
			return []data.Event{{EventID: 1, StartedAt: time.Unix(200, 0)}}, nil
		}}
		metrics := dayMetrics{groups: func(ctx context.Context, user string, start, end, cutoff time.Time) ([]data.MiscThoughtGroup, error) {
			if ctx != t.Context() || user != "u" || start != from || end != until || cutoff.Unix() != 401 {
				t.Fatal("group read lost scope or current second")
			}
			return []data.MiscThoughtGroup{{ObservedAt: time.Unix(300, 0), Count: 1}, {ObservedAt: time.Unix(100, 0), Count: 2}}, nil
		}}
		service := NewService(events, thoughtReaderStub(nil), metrics)
		service.now = func() time.Time { return time.Unix(400, 999) }
		entries, err := service.Day(t.Context(), "u", from, until)
		if err != nil || len(entries) != 3 {
			t.Fatalf("got entries %v, %v, want three entries", entries, err)
		}
		for i, want := range []int64{100, 200, 300} {
			if got := entries[i].StartedAt().Unix(); got != want {
				t.Fatalf("entry %d: got time %d, want %d", i, got, want)
			}
		}
		if entries[0].Key().IsZero() || entries[0].Key().EventID != 0 || entries[1].Key().EventID != 1 || entries[0].Key() == entries[2].Key() {
			t.Fatalf("got entry identities %+v, want distinct real and generated identities", entries)
		}
	})
	t.Run("returns original failures and stops after failed event read", func(t *testing.T) {
		cause := errors.New("read failure")
		for _, eventFails := range []bool{true, false} {
			events := dayEvents{list: func(context.Context, string, time.Time, time.Time) ([]data.Event, error) {
				if eventFails {
					return nil, cause
				}
				return nil, nil
			}}
			metrics := dayMetrics{groups: func(context.Context, string, time.Time, time.Time, time.Time) ([]data.MiscThoughtGroup, error) {
				if eventFails {
					t.Fatal("groups queried after event failure")
				}
				return nil, cause
			}}
			service := NewService(events, thoughtReaderStub(nil), metrics)
			got, err := service.Day(t.Context(), "u", time.Unix(100, 0), time.Unix(200, 0))
			if got != nil || err != cause {
				t.Fatalf("got %v, %v, want nil and original error", got, err)
			}
		}
	})
}

func TestService_OpenMiscThoughtsView(t *testing.T) {
	t.Run("normalizes a timestamp and freezes exactly one second", func(t *testing.T) {
		at := time.Unix(100, 900).In(time.FixedZone("offset", 3600))
		metrics := dayMetrics{groups: func(ctx context.Context, user string, from, until, cutoff time.Time) ([]data.MiscThoughtGroup, error) {
			if ctx != t.Context() || user != "u" || from != time.Unix(100, 0).UTC() || until != time.Unix(101, 0).UTC() || cutoff.Unix() != 201 {
				t.Fatal("Misc lookup lost scope or timestamp precision")
			}
			return []data.MiscThoughtGroup{{ObservedAt: from, Count: 1}}, nil
		}}
		service := NewService(eventReaderStub(nil), thoughtReaderStub(nil), metrics)
		service.now = func() time.Time { return time.Unix(200, 999) }
		scope, err := service.OpenMiscThoughtsView(t.Context(), "u", at)
		if err != nil || scope.Key().ObservedAt != time.Unix(100, 0).UTC() || scope.until.Sub(scope.from) != time.Second {
			t.Fatalf("got scope %+v, %v, want owned one-second scope", scope, err)
		}
	})
	t.Run("rejects missing groups and preserves read failures", func(t *testing.T) {
		cause := errors.New("group read failure")
		for _, err := range []error{nil, cause} {
			service := NewService(eventReaderStub(nil), thoughtReaderStub(nil), dayMetrics{groups: func(context.Context, string, time.Time, time.Time, time.Time) ([]data.MiscThoughtGroup, error) {
				return nil, err
			}})
			_, got := service.OpenMiscThoughtsView(t.Context(), "u", time.Unix(100, 0))
			want := err
			if want == nil {
				want = data.ErrRecordNotFound
			}
			if got != want {
				t.Fatalf("got error %v, want %v", got, want)
			}
		}
	})
}
