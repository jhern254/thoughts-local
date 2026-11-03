package goalprogress

import (
	"context"
	"errors"
	"github.com/jhern254/go-thoughts/internal/data"
	"testing"
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
			want := data.GoalProgressSummary{DirectAutomaticSeconds: 60, EffectiveAutomaticSeconds: 120, DirectManualSeconds: 5, EffectiveManualSeconds: 10}
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
