// Package subjectgoal manages classification links without assigning progress.
package subjectgoal

import (
	"context"

	"github.com/jhern254/go-thoughts/internal/data"
)

type Store interface {
	AddSubjectGoal(context.Context, string, int64, int64) error
	RemoveSubjectGoal(context.Context, string, int64, int64) error
	ListSubjectGoals(context.Context, string, int64) ([]data.SubjectGoal, error)
	ListGoalSubjects(context.Context, string, int64) ([]data.SubjectGoal, error)
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

// Add links undeleted same-owner endpoints, including inactive goals.
// The store checks availability atomically and rejects duplicate links.
func (s *Service) Add(ctx context.Context, userID string, subjectID, goalID int64) error {
	return s.store.AddSubjectGoal(ctx, userID, subjectID, goalID)
}

// Remove deletes only the link, including links to retained deleted endpoints.
func (s *Service) Remove(ctx context.Context, userID string, subjectID, goalID int64) error {
	return s.store.RemoveSubjectGoal(ctx, userID, subjectID, goalID)
}

// ListForSubject returns retained links in goal ID order, including deleted endpoints.
func (s *Service) ListForSubject(ctx context.Context, userID string, subjectID int64) ([]data.SubjectGoal, error) {
	return s.store.ListSubjectGoals(ctx, userID, subjectID)
}

// ListForGoal returns retained links in subject ID order, including deleted endpoints.
func (s *Service) ListForGoal(ctx context.Context, userID string, goalID int64) ([]data.SubjectGoal, error) {
	return s.store.ListGoalSubjects(ctx, userID, goalID)
}
