package timeline

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

func (f thoughtReaderStub) BrowseThoughtsViewInRange(context.Context, string, time.Time, time.Time, data.ThoughtViewRequest) (data.ThoughtView, error) {
	panic("unexpected summary read")
}
func (f thoughtReaderStub) LatestThoughtInRange(context.Context, string, time.Time, time.Time) (*data.ThoughtSummary, error) {
	panic("unexpected latest read")
}

type metricsStub struct {
	from, until time.Time
	err         error
}

func (s metricsStub) ThoughtCountsByEvent(context.Context, string, time.Time, time.Time, time.Time) ([]data.EventThoughtCount, error) {
	return nil, s.err
}
func (s metricsStub) CountThoughtsInRange(context.Context, string, time.Time, time.Time) (int64, error) {
	return 230, s.err
}

type viewReader struct {
	thoughtReaderStub
	until []time.Time
}

func (s *viewReader) BrowseThoughtsViewInRange(_ context.Context, _ string, _, until time.Time, _ data.ThoughtViewRequest) (data.ThoughtView, error) {
	s.until = append(s.until, until)
	return data.ThoughtView{Items: []data.ThoughtSummary{{ThoughtID: 42}}}, nil
}

func TestService_ThoughtView(t *testing.T) {
	t.Run("freezes ongoing cutoff across cursor reads and preserves count errors", func(t *testing.T) {
		reader := &viewReader{}
		cause := errors.New("PRIVATE_COUNT_MARKER")
		s := NewService(eventReaderStub(func(context.Context, string, int64) (*data.Event, error) {
			return &data.Event{EventID: 7, StartedAt: time.Unix(10, 0)}, nil
		}), reader, metricsStub{err: cause})
		s.now = func() time.Time { return time.Unix(100, 999) }
		scope, err := s.OpenThoughtsView(t.Context(), "u", 7)
		if err != nil {
			t.Fatal(err)
		}
		s.now = func() time.Time { return time.Unix(200, 0) }
		for range 2 {
			got, err := s.BrowseThoughtsView(t.Context(), "u", scope, data.ThoughtViewRequest{})
			if err != nil || got.Items[0].ThoughtID != 42 {
				t.Fatalf("got %+v, %v", got, err)
			}
		}
		for _, until := range reader.until {
			if until.Unix() != 101 {
				t.Fatalf("got cutoff %v, want 101", until)
			}
		}
		count, err := s.CountThoughts(t.Context(), "u", scope)
		if count != 230 || err != cause {
			t.Fatalf("got %d, %v; want 230 and original error", count, err)
		}
		if _, err := s.BrowseThoughtsView(t.Context(), "other", scope, data.ThoughtViewRequest{}); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatal("accepted foreign scope")
		}
	})
}
