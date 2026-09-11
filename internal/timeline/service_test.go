package timeline

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

type thoughtReaderStub func(context.Context, string, time.Time, time.Time) ([]data.Thought, error)

type eventReaderStub func(context.Context, string, int64) (*data.Event, error)

func (f eventReaderStub) Get(ctx context.Context, user string, id int64) (*data.Event, error) {
	return f(ctx, user, id)
}

func (f thoughtReaderStub) ListThoughtsInRange(ctx context.Context, user string, from, until time.Time) ([]data.Thought, error) {
	return f(ctx, user, from, until)
}

func TestService_ListThoughts(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		end   *time.Time
		until int64
	}{
		{"uses the whole completed interval across midnight", timePointer(90000), 90000},
		{"uses equal bounds for an empty completed interval", timePointer(80000), 80000},
		{"includes the captured current second for an ongoing event", nil, 100001},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			item := &data.Event{EventID: 7, UserID: "u", StartedAt: time.Unix(80000, 0).UTC(), EndedAt: scenario.end}
			original := *item
			cause := errors.Join(data.ErrDatabaseBusy, errors.New("PRIVATE_MARKER"))
			reader := thoughtReaderStub(func(ctx context.Context, user string, from, until time.Time) ([]data.Thought, error) {
				if ctx != t.Context() || user != "u" || from != item.StartedAt || until != time.Unix(scenario.until, 0).UTC() {
					t.Fatalf("got %s [%v,%v), want u [80000,%d)", user, from, until, scenario.until)
				}
				return []data.Thought{{ThoughtID: 42}}, cause
			})
			s := NewService(eventReaderStub(func(ctx context.Context, user string, id int64) (*data.Event, error) {
				if ctx != t.Context() || user != "u" || id != 7 {
					t.Fatal("event lookup lost request scope")
				}
				return item, nil
			}), reader)
			calls := 0
			s.now = func() time.Time { calls++; return time.Unix(100000, 999).In(time.FixedZone("offset", 3600)) }
			got, err := s.ListThoughts(t.Context(), "u", 7)
			if len(got) != 1 || got[0].ThoughtID != 42 || err != cause || *item != original {
				t.Fatalf("got %v, %v, want original results/error and unchanged event", got, err)
			}
			if scenario.end == nil && calls != 1 {
				t.Fatalf("got %d clock calls, want 1", calls)
			}
		})
	}
	t.Run("preserves lookup failures without querying thoughts", func(t *testing.T) {
		cause := errors.Join(data.ErrRecordNotFound, errors.New("PRIVATE_MARKER"))
		s := NewService(eventReaderStub(func(context.Context, string, int64) (*data.Event, error) { return nil, cause }), thoughtReaderStub(func(context.Context, string, time.Time, time.Time) ([]data.Thought, error) {
			t.Fatal("queried thoughts after failed event lookup")
			return nil, nil
		}))
		if got, err := s.ListThoughts(t.Context(), "u", 7); got != nil || err != cause {
			t.Fatalf("got %v, %v, want nil and original failure", got, err)
		}
	})
}

func timePointer(second int64) *time.Time {
	value := time.Unix(second, 0).UTC()
	return &value
}
