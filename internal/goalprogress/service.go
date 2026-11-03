// Package goalprogress reads explicit and automatic lifetime progress without
// changing Goal settings or materializing inherited parent contributions.
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
