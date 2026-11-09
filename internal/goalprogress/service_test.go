package goalprogress

import (
	"context"
	"errors"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
)

type summaryReaderStub func(context.Context, string, int64) (data.GoalProgressSummary, error)

func (f summaryReaderStub) GetGoalProgressSummary(ctx context.Context, user string, id int64) (data.GoalProgressSummary, error) {
	return f(ctx, user, id)
}
func TestService_GetSummary(t *testing.T) {
	for _, cause := range []error{nil, data.ErrRecordNotFound, context.Canceled} {
		name := "returns stored totals"
		if cause != nil {
			name = "preserves " + cause.Error()
		}
		t.Run(name, func(t *testing.T) {
			want := data.GoalProgressSummary{LifetimeEarnedSeconds: 120}
			if cause != nil {
				want = data.GoalProgressSummary{}
			}
			service := NewService(summaryReaderStub(func(ctx context.Context, user string, id int64) (data.GoalProgressSummary, error) {
				if ctx != t.Context() || user != "owner" || id != 7 {
					t.Fatal("summary lost request scope")
				}
				return want, cause
			}))
			got, err := service.GetSummary(t.Context(), "owner", 7)
			if got != want || !errors.Is(err, cause) {
				t.Fatalf("summary: got %+v,%v, want %+v,%v", got, err, want, cause)
			}
		})
	}
}

func (f summaryReaderStub) ListEarnedEvents(context.Context, string, int64, data.GoalEventPageRequest) (data.GoalEventPage, error) {
	panic("unexpected lineage read")
}
func (f summaryReaderStub) ListEventLineageThoughts(context.Context, string, int64, int64, data.ThoughtSummaryViewRequest) (data.GoalEventThoughtPage, error) {
	panic("unexpected lineage read")
}
func (f summaryReaderStub) GetEventLineageThought(context.Context, string, int64, int64, int64) (*data.Thought, error) {
	panic("unexpected lineage read")
}

type lineageReaderStub struct {
	summaryReaderStub
	events   func(context.Context, string, int64, data.GoalEventPageRequest) (data.GoalEventPage, error)
	thoughts func(context.Context, string, int64, int64, data.ThoughtSummaryViewRequest) (data.GoalEventThoughtPage, error)
	thought  func(context.Context, string, int64, int64, int64) (*data.Thought, error)
}

func (s lineageReaderStub) ListEarnedEvents(ctx context.Context, user string, goal int64, page data.GoalEventPageRequest) (data.GoalEventPage, error) {
	return s.events(ctx, user, goal, page)
}
func (s lineageReaderStub) ListEventLineageThoughts(ctx context.Context, user string, goal, event int64, page data.ThoughtSummaryViewRequest) (data.GoalEventThoughtPage, error) {
	return s.thoughts(ctx, user, goal, event, page)
}
func (s lineageReaderStub) GetEventLineageThought(ctx context.Context, user string, goal, event, thought int64) (*data.Thought, error) {
	return s.thought(ctx, user, goal, event, thought)
}

func TestService_ActivityLineage(t *testing.T) {
	for _, cause := range []error{nil, data.ErrRecordNotFound, context.Canceled} {
		name := "returns stored history"
		if cause != nil {
			name = "preserves " + cause.Error()
		}
		t.Run(name, func(t *testing.T) {
			eventRequest := data.GoalEventPageRequest{Cursor: &data.GoalEventCursor{EventID: 8}, Direction: data.EarnedEventsOlder}
			thoughtRequest := data.ThoughtSummaryViewRequest{Cursor: &data.ThoughtCursor{ThoughtID: 9}, Direction: data.ThoughtsNewer}
			full := &data.Thought{ThoughtID: 9, Thought: "Full text only after selection"}
			reader := lineageReaderStub{
				events: func(ctx context.Context, user string, id int64, page data.GoalEventPageRequest) (data.GoalEventPage, error) {
					if ctx != t.Context() || user != "owner" || id != 7 || page != eventRequest {
						t.Fatal("earned Events lost request scope")
					}
					return data.GoalEventPage{Events: []data.GoalEventLineage{{EventID: 8}}}, cause
				},
				thoughts: func(ctx context.Context, user string, goalID, eventID int64, page data.ThoughtSummaryViewRequest) (data.GoalEventThoughtPage, error) {
					if ctx != t.Context() || user != "owner" || goalID != 7 || eventID != 8 || page != thoughtRequest {
						t.Fatal("lineage Thoughts lost request scope")
					}
					return data.GoalEventThoughtPage{Event: data.GoalEventLineage{EventID: 8}}, cause
				},
				thought: func(ctx context.Context, user string, goalID, eventID, thoughtID int64) (*data.Thought, error) {
					if ctx != t.Context() || user != "owner" || goalID != 7 || eventID != 8 || thoughtID != 9 {
						t.Fatal("full Thought lost request scope")
					}
					return full, cause
				},
			}
			service := NewService(reader)
			events, err := service.ListEarnedEvents(t.Context(), "owner", 7, eventRequest)
			if !errors.Is(err, cause) || len(events.Events) != 1 {
				t.Fatalf("Events: got %+v,%v", events, err)
			}
			thoughts, err := service.ListEventLineageThoughts(t.Context(), "owner", 7, 8, thoughtRequest)
			if !errors.Is(err, cause) || thoughts.Event.EventID != 8 {
				t.Fatalf("Thought page: got %+v,%v", thoughts, err)
			}
			thought, err := service.GetEventLineageThought(t.Context(), "owner", 7, 8, 9)
			if !errors.Is(err, cause) || thought != full {
				t.Fatalf("Thought: got %+v,%v", thought, err)
			}
		})
	}
}
