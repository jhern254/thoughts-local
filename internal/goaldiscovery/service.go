// Package goaldiscovery finds current goal candidates through an event's subject.
// Discovery does not assign goals or record progress.
package goaldiscovery

import (
	"context"

	"github.com/jhern254/go-thoughts/internal/data"
)

type EventReader interface {
	Get(context.Context, string, int64) (*data.Event, error)
}

type CandidateReader interface {
	ListActiveGoalsForSubject(context.Context, string, int64) ([]data.Goal, error)
}

// Recommender selects a duplicate-free subset of the supplied candidates.
// Implementations must not mutate candidates or introduce unrelated goals.
type Recommender interface {
	Recommend(context.Context, []data.Goal) ([]data.Goal, error)
}

type Result struct {
	PriorityGoals    []data.Goal
	RecommendedGoals []data.Goal
}

type Service struct {
	events      EventReader
	candidates  CandidateReader
	recommender Recommender
}

// NewService requires all three collaborators, including when recommendations are disabled per call.
func NewService(events EventReader, candidates CandidateReader, recommender Recommender) *Service {
	return &Service{events: events, candidates: candidates, recommender: recommender}
}

// DiscoverForEvent returns high-priority goals and optional recommendations from
// the remaining active linked goals. Results reflect current links and flags,
// including for completed events; they do not express historical eligibility.
// The event and candidate reads are not a transactional snapshot.
func (s *Service) DiscoverForEvent(ctx context.Context, userID string, eventID int64, includeRecommendations bool) (Result, error) {
	item, err := s.events.Get(ctx, userID, eventID)
	if err != nil {
		return Result{}, err
	}
	result := Result{PriorityGoals: []data.Goal{}, RecommendedGoals: []data.Goal{}}
	if item.SubjectID == nil {
		return result, nil
	}
	candidates, err := s.candidates.ListActiveGoalsForSubject(ctx, userID, *item.SubjectID)
	if err != nil {
		return Result{}, err
	}
	var remaining []data.Goal
	for _, candidate := range candidates {
		if candidate.Priority == "high" {
			result.PriorityGoals = append(result.PriorityGoals, candidate)
		} else {
			remaining = append(remaining, candidate)
		}
	}
	if includeRecommendations && len(remaining) > 0 {
		recommended, err := s.recommender.Recommend(ctx, remaining)
		if err != nil {
			return Result{}, err
		}
		result.RecommendedGoals = append(result.RecommendedGoals, recommended...)
	}
	return result, nil
}

// AllCandidatesRecommender is a placeholder that keeps every candidate in input order.
type AllCandidatesRecommender struct{}

func (AllCandidatesRecommender) Recommend(ctx context.Context, candidates []data.Goal) ([]data.Goal, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return candidates, nil
}
