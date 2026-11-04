// Package goalprogress reports how much time a Goal has earned from completed
// Events, including time earned by its child Goals. A shared Event counts once,
// even when several child Goals contribute that Event to the same parent.
// It also exposes the earned Events and their Subject-matched Thoughts for browsing.
package goalprogress

import (
	"context"

	"github.com/jhern254/go-thoughts/internal/data"
)

type Reader interface {
	GetGoalProgressSummary(context.Context, string, int64) (data.GoalProgressSummary, error)
	ListEarnedEvents(context.Context, string, int64, data.GoalEventPageRequest) (data.GoalEventPage, error)
	ListEventLineageThoughts(context.Context, string, int64, int64, data.ThoughtSummaryViewRequest) (data.GoalEventThoughtPage, error)
	GetEventLineageThought(context.Context, string, int64, int64, int64) (*data.Thought, error)
}
type Service struct{ reader Reader }

func NewService(reader Reader) *Service { return &Service{reader: reader} }
func (s *Service) GetSummary(ctx context.Context, userID string, goalID int64) (data.GoalProgressSummary, error) {
	return s.reader.GetGoalProgressSummary(ctx, userID, goalID)
}

func (s *Service) ListEarnedEvents(ctx context.Context, userID string, goalID int64, page data.GoalEventPageRequest) (data.GoalEventPage, error) {
	return s.reader.ListEarnedEvents(ctx, userID, goalID, page)
}
func (s *Service) ListEventLineageThoughts(ctx context.Context, userID string, goalID, eventID int64, page data.ThoughtSummaryViewRequest) (data.GoalEventThoughtPage, error) {
	return s.reader.ListEventLineageThoughts(ctx, userID, goalID, eventID, page)
}
func (s *Service) GetEventLineageThought(ctx context.Context, userID string, goalID, eventID, thoughtID int64) (*data.Thought, error) {
	return s.reader.GetEventLineageThought(ctx, userID, goalID, eventID, thoughtID)
}
