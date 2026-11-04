// Package goalprogress reports how much time a Goal has earned from completed
// Events, including time earned by its child Goals. A shared Event counts once,
// even when several child Goals contribute that Event to the same parent.
package goalprogress

import (
	"context"

	"github.com/jhern254/go-thoughts/internal/data"
)

type SummaryReader interface {
	GetGoalProgressSummary(context.Context, string, int64) (data.GoalProgressSummary, error)
}
type Service struct{ summaries SummaryReader }

func NewService(summaries SummaryReader) *Service { return &Service{summaries: summaries} }
func (s *Service) GetSummary(ctx context.Context, userID string, goalID int64) (data.GoalProgressSummary, error) {
	return s.summaries.GetGoalProgressSummary(ctx, userID, goalID)
}
