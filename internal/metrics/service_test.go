package metrics

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

type storeStub struct {
	err                 error
	ctx                 context.Context
	userID              string
	from, until, cutoff time.Time
}

func (s *storeStub) ThoughtCountsByEvent(ctx context.Context, u string, _, _, _ time.Time) ([]data.EventThoughtCountView, error) {
	s.ctx, s.userID = ctx, u
	return []data.EventThoughtCountView{{EventID: 7, Count: 2}}, s.err
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

func (s *storeStub) ThoughtCountsBySubject(ctx context.Context, u string) ([]data.SubjectThoughtCountView, error) {
	s.ctx, s.userID = ctx, u
	return []data.SubjectThoughtCountView{{SubjectID: 7, Count: 2}}, s.err
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

func (s *storeStub) ThoughtStatsInRange(ctx context.Context, u string, from, until time.Time) (data.ThoughtIntervalStats, error) {
	s.ctx, s.userID = ctx, u
	return data.ThoughtIntervalStats{Count: 2, MaxCharacters: 320}, s.err
}

func (s *storeStub) ListThoughtGroupsOutsideEvents(context.Context, string, time.Time, time.Time, time.Time) ([]data.ThoughtGroupOutsideEvents, error) {
	return nil, s.err
}

func (s *storeStub) SubjectTimeInRange(ctx context.Context, user string, from, until, cutoff time.Time) ([]data.SubjectTimeView, error) {
	s.ctx, s.userID, s.from, s.until, s.cutoff = ctx, user, from, until, cutoff
	return []data.SubjectTimeView{{SubjectID: 7, TimeSpentSeconds: 42}}, s.err
}

func TestService_SubjectTimeInRange(t *testing.T) {
	t.Run("preserves context interval cutoff results and original error", func(t *testing.T) {
		from, until, cutoff := time.Unix(1, 0), time.Unix(100, 0), time.Unix(50, 0)
		for _, cause := range []error{nil, errors.New("store failure")} {
			store := &storeStub{err: cause}
			rows, err := NewService(store).SubjectTimeInRange(t.Context(), "u", from, until, cutoff)
			if store.ctx != t.Context() || store.userID != "u" || store.from != from || store.until != until || store.cutoff != cutoff || len(rows) != 1 || rows[0].TimeSpentSeconds != 42 || err != cause {
				t.Fatalf("got rows %v, error %v, scope %+v; want original interval, cutoff and error", rows, err, store)
			}
		}
	})
}
