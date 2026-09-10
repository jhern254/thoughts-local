package metrics

import (
	"context"
	"errors"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
)

type storeStub struct {
	err    error
	ctx    context.Context
	userID string
}

func (s *storeStub) ThoughtCountsBySubject(ctx context.Context, u string) ([]data.SubjectThoughtCount, error) {
	s.ctx, s.userID = ctx, u
	return []data.SubjectThoughtCount{{SubjectID: 7, Count: 2}}, s.err
}
func TestService_ThoughtCountsBySubject(t *testing.T) {
	t.Run("preserves scope results and original error", func(t *testing.T) {
		ctx := context.Background()
		for _, err := range []error{nil, errors.New("store failure")} {
			store := &storeStub{err: err}
			rows, gotErr := NewService(store).ThoughtCountsBySubject(ctx, "u")
			if store.ctx != ctx || store.userID != "u" || len(rows) != 1 || rows[0].Count != 2 || gotErr != err {
				t.Fatalf("got %v, %v with scope %q", rows, gotErr, store.userID)
			}
		}
	})
}
