package goaldiscovery

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
)

type eventReaderStub func(context.Context, string, int64) (*data.Event, error)

func (f eventReaderStub) Get(ctx context.Context, user string, id int64) (*data.Event, error) {
	return f(ctx, user, id)
}

type candidateReaderStub func(context.Context, string, int64) ([]data.Goal, error)

func (f candidateReaderStub) ListActiveGoalsForSubject(ctx context.Context, user string, id int64) ([]data.Goal, error) {
	return f(ctx, user, id)
}

type recommenderStub func(context.Context, []data.Goal) ([]data.Goal, error)

func (f recommenderStub) Recommend(ctx context.Context, candidates []data.Goal) ([]data.Goal, error) {
	return f(ctx, candidates)
}

func TestAllCandidatesRecommender_Recommend(t *testing.T) {
	candidates := []data.Goal{{GoalID: 2, Priority: "normal"}, {GoalID: 1, Priority: "low"}}
	t.Run("keeps all candidates in input order", func(t *testing.T) {
		got, err := (AllCandidatesRecommender{}).Recommend(t.Context(), candidates)
		if err != nil || !reflect.DeepEqual(got, candidates) {
			t.Fatalf("recommendations: got %+v, %v; want %+v, nil", got, err, candidates)
		}
	})
	t.Run("preserves cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		got, err := (AllCandidatesRecommender{}).Recommend(ctx, candidates)
		if got != nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled recommendations: got %+v, %v; want nil, context.Canceled", got, err)
		}
	})
}

func TestService_DiscoverForEvent(t *testing.T) {
	high := data.Goal{GoalID: 1, Priority: "high"}
	normal := data.Goal{GoalID: 2, Priority: "normal"}
	low := data.Goal{GoalID: 3, Priority: "low"}
	for _, tc := range []struct {
		name                            string
		enabled                         bool
		candidates, priority, remaining []data.Goal
	}{
		{"shows only high priority when recommendations are disabled", false, []data.Goal{high, normal, low}, []data.Goal{high}, nil},
		{"uses the engine selection from remaining candidates", true, []data.Goal{high, normal, low}, []data.Goal{high}, []data.Goal{normal, low}},
		{"skips the engine when every candidate has high priority", true, []data.Goal{high}, []data.Goal{high}, nil},
		{"returns empty groups when there are no candidates", true, nil, []data.Goal{}, nil},
		{"normal priority is not an explicit user mark", false, []data.Goal{normal}, []data.Goal{}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			subjectID := int64(7)
			item := &data.Event{EventID: 9, UserID: "owner", SubjectID: &subjectID}
			original := *item
			calls := 0
			s := NewService(eventReaderStub(func(ctx context.Context, user string, id int64) (*data.Event, error) {
				if ctx != t.Context() || user != "owner" || id != 9 {
					t.Fatal("event lookup lost request scope")
				}
				return item, nil
			}), candidateReaderStub(func(ctx context.Context, user string, id int64) ([]data.Goal, error) {
				if ctx != t.Context() || user != "owner" || id != subjectID {
					t.Fatal("candidate lookup lost request scope")
				}
				return tc.candidates, nil
			}), recommenderStub(func(ctx context.Context, candidates []data.Goal) ([]data.Goal, error) {
				calls++
				if ctx != t.Context() || !reflect.DeepEqual(candidates, tc.remaining) {
					t.Fatalf("engine input: got %+v, want %+v with original context", candidates, tc.remaining)
				}
				// Select only the low-priority goal to prove discovery uses the engine's result.
				return []data.Goal{low}, nil
			}))
			got, err := s.DiscoverForEvent(t.Context(), "owner", 9, tc.enabled)
			want := Result{PriorityGoals: tc.priority, RecommendedGoals: []data.Goal{}}
			wantCalls := 0
			if len(tc.remaining) > 0 {
				want.RecommendedGoals = []data.Goal{low}
				wantCalls = 1
			}
			if err != nil || !reflect.DeepEqual(got, want) || calls != wantCalls {
				t.Fatalf("discovery: got %+v, %v, calls=%d; want %+v, nil, calls=%d", got, err, calls, want, wantCalls)
			}
			if *item != original {
				t.Fatalf("event: got %+v, want unchanged %+v", *item, original)
			}
		})
	}

	t.Run("unassigned events avoid candidate and recommendation reads", func(t *testing.T) {
		s := NewService(eventReaderStub(func(context.Context, string, int64) (*data.Event, error) { return &data.Event{}, nil }),
			candidateReaderStub(func(context.Context, string, int64) ([]data.Goal, error) {
				t.Fatal("queried candidates for unassigned event")
				return nil, nil
			}),
			recommenderStub(func(context.Context, []data.Goal) ([]data.Goal, error) {
				t.Fatal("recommended for unassigned event")
				return nil, nil
			}))
		got, err := s.DiscoverForEvent(t.Context(), "owner", 9, true)
		want := Result{PriorityGoals: []data.Goal{}, RecommendedGoals: []data.Goal{}}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("unassigned event: got %+v, %v; want %+v, nil", got, err, want)
		}
	})

	for _, stage := range []string{"event", "candidates", "recommendations"} {
		t.Run("preserves "+stage+" errors without partial suggestions", func(t *testing.T) {
			cause := errors.Join(context.Canceled, errors.New("PRIVATE_MARKER"))
			subjectID := int64(7)
			s := NewService(eventReaderStub(func(context.Context, string, int64) (*data.Event, error) {
				if stage == "event" {
					return nil, cause
				}
				return &data.Event{SubjectID: &subjectID}, nil
			}), candidateReaderStub(func(context.Context, string, int64) ([]data.Goal, error) {
				if stage == "event" {
					t.Fatal("queried candidates after event failure")
				}
				if stage == "candidates" {
					return nil, cause
				}
				return []data.Goal{high, normal}, nil
			}), recommenderStub(func(context.Context, []data.Goal) ([]data.Goal, error) {
				if stage != "recommendations" {
					t.Fatal("recommended after a lookup failure")
				}
				return []data.Goal{normal}, cause
			}))
			got, err := s.DiscoverForEvent(t.Context(), "owner", 9, true)
			if err != cause || got.PriorityGoals != nil || got.RecommendedGoals != nil {
				t.Fatalf("failed discovery: got %+v, %v; want zero result and original error %v", got, err, cause)
			}
		})
	}

	t.Run("normalizes an empty engine selection", func(t *testing.T) {
		subjectID := int64(7)
		s := NewService(eventReaderStub(func(context.Context, string, int64) (*data.Event, error) {
			return &data.Event{SubjectID: &subjectID}, nil
		}),
			candidateReaderStub(func(context.Context, string, int64) ([]data.Goal, error) { return []data.Goal{normal}, nil }),
			recommenderStub(func(context.Context, []data.Goal) ([]data.Goal, error) { return nil, nil }))
		got, err := s.DiscoverForEvent(t.Context(), "owner", 9, true)
		want := Result{PriorityGoals: []data.Goal{}, RecommendedGoals: []data.Goal{}}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("empty engine result: got %+v, %v; want %+v, nil", got, err, want)
		}
	})
}
