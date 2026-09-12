package metrics

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

type storeStub struct {
	err    error
	ctx    context.Context
	userID string
}

func (s *storeStub) ThoughtCountsByEvent(ctx context.Context, u string, _, _, _ time.Time) ([]data.EventThoughtCount, error) {
	s.ctx, s.userID = ctx, u
	return []data.EventThoughtCount{{EventID: 7, Count: 2}}, s.err
}
func (s *storeStub) CountThoughtsInRange(ctx context.Context, u string, _, _ time.Time) (int64, error) {
	s.ctx, s.userID = ctx, u
	return 2, s.err
}

func (s *storeStub) CountThoughts(ctx context.Context, u string) (int64, error) {
	s.ctx, s.userID = ctx, u
	return 230, s.err
}

func TestService_CountThoughts(t *testing.T) {
	t.Run("preserves scope total and original error", func(t *testing.T) {
		for _, err := range []error{nil, errors.New("store failure")} {
			store := &storeStub{err: err}
			ctx := t.Context()
			count, gotErr := NewService(store).CountThoughts(ctx, "u")
			if store.ctx != ctx || store.userID != "u" || count != 230 || gotErr != err {
				t.Fatalf("got count %d, error %v, scope %q; want 230, %v, u", count, gotErr, store.userID, err)
			}
		}
	})
}

func (s *storeStub) ThoughtCountsBySubject(ctx context.Context, u string) ([]data.SubjectThoughtCount, error) {
	s.ctx, s.userID = ctx, u
	return []data.SubjectThoughtCount{{SubjectID: 7, Count: 2}}, s.err
}

func (s *storeStub) CountUnassignedThoughts(ctx context.Context, u string) (int64, error) {
	s.ctx, s.userID = ctx, u
	return 2, s.err
}

func TestService_CountUnassignedThoughts(t *testing.T) {
	t.Run("preserves scope count and original error", func(t *testing.T) {
		ctx := context.Background()
		for _, err := range []error{nil, errors.New("store failure")} {
			store := &storeStub{err: err}
			count, gotErr := NewService(store).CountUnassignedThoughts(ctx, "u")
			if store.ctx != ctx || store.userID != "u" || count != 2 || gotErr != err {
				t.Fatalf("got count %d, error %v, scope %q", count, gotErr, store.userID)
			}
		}
	})
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
