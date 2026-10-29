// Package goalassignment coordinates atomic Goal creation and live Subject membership.
// A Goal with no live Subjects is Unassigned; no sentinel Subject is stored.
package goalassignment

import (
	"context"
	"maps"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/goal"
	"github.com/jhern254/go-thoughts/internal/validator"
)

type GoalPreparer interface {
	PrepareCreate(context.Context, string, goal.CreateGoalInput) (*data.Goal, error)
}

type Store interface {
	CreateGoalWithSubjects(context.Context, *data.Goal, []int64) (*data.Goal, error)
	ReplaceGoalSubjects(context.Context, string, int64, int64, []int64) (*data.Goal, error)
}

type Service struct {
	goals GoalPreparer
	store Store
}

func NewService(goals GoalPreparer, store Store) *Service {
	return &Service{goals: goals, store: store}
}

func (s *Service) Create(ctx context.Context, userID string, input goal.CreateGoalInput, subjectIDs []int64) (*data.Goal, error) {
	item, err := s.goals.PrepareCreate(ctx, userID, input)
	if err != nil {
		return nil, err
	}
	if err := validateSelection(userID, 1, subjectIDs); err != nil {
		return nil, err
	}
	return s.store.CreateGoalWithSubjects(ctx, item, subjectIDs)
}

// ReplaceSubjects changes only live membership. A changed set advances the Goal
// version once; an identical set preserves it but still requires the current version.
func (s *Service) ReplaceSubjects(ctx context.Context, userID string, goalID, expectedVersion int64, subjectIDs []int64) (*data.Goal, error) {
	if err := validateSelection(userID, expectedVersion, subjectIDs); err != nil {
		return nil, err
	}
	return s.store.ReplaceGoalSubjects(ctx, userID, goalID, expectedVersion, subjectIDs)
}

type ValidationError struct {
	Fields       map[string]string
	publicFields map[string]string
}

func (e *ValidationError) PublicFields() map[string]string {
	if e == nil {
		return nil
	}
	return maps.Clone(e.publicFields)
}

func (e *ValidationError) Error() string { return "goal assignment validation failed" }

func validateSelection(userID string, version int64, subjectIDs []int64) error {
	v := validator.NewValidator()
	v.Check(userID != "", "user_id", "must be provided")
	v.Check(version > 0, "version", "must be greater than zero")
	seen := make(map[int64]bool, len(subjectIDs))
	for _, id := range subjectIDs {
		v.Check(!seen[id], "subject_ids", "must not contain duplicates")
		seen[id] = true
	}
	if !v.Valid() {
		return &ValidationError{Fields: v.Errors, publicFields: maps.Clone(v.Errors)}
	}
	return nil
}
