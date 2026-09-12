package timeline

import (
	"context"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

// ThoughtScope freezes an owned event's interval for one browsing session.
// Only this service constructs scopes. Refresh resolves a new scope; paging
// neither changes its ongoing cutoff nor recounts the interval.
type ThoughtScope struct {
	userID string
	event  data.Event
	until  time.Time
}

func (s ThoughtScope) Event() data.Event { return s.event }

func (s *Service) OpenThoughtsView(ctx context.Context, userID string, eventID int64) (ThoughtScope, error) {
	item, err := s.events.Get(ctx, userID, eventID)
	if err != nil {
		return ThoughtScope{}, err
	}
	until := s.now().UTC().Truncate(time.Second).Add(time.Second)
	if item.EndedAt != nil {
		until = *item.EndedAt
	}
	return ThoughtScope{userID: userID, event: *item, until: until}, nil
}

func (s *Service) BrowseThoughtsView(ctx context.Context, userID string, scope ThoughtScope, request data.ThoughtViewRequest) (data.ThoughtView, error) {
	if scope.userID == "" || scope.userID != userID {
		return data.ThoughtView{}, data.ErrRecordNotFound
	}
	return s.thoughts.BrowseThoughtsViewInRange(ctx, userID, scope.event.StartedAt, scope.until, request)
}

func (s *Service) CountThoughts(ctx context.Context, userID string, scope ThoughtScope) (int64, error) {
	if scope.userID == "" || scope.userID != userID {
		return 0, data.ErrRecordNotFound
	}
	return s.metrics.CountThoughtsInRange(ctx, userID, scope.event.StartedAt, scope.until)
}

func (s *Service) LatestThought(ctx context.Context, userID string, eventID int64) (*data.ThoughtSummary, error) {
	scope, err := s.OpenThoughtsView(ctx, userID, eventID)
	if err != nil {
		return nil, err
	}
	return s.thoughts.LatestThoughtInRange(ctx, userID, scope.event.StartedAt, scope.until)
}

func (s *Service) ThoughtCounts(ctx context.Context, userID string, from, until time.Time) ([]data.EventThoughtCount, error) {
	return s.metrics.ThoughtCountsByEvent(ctx, userID, from, until, s.now().UTC().Truncate(time.Second).Add(time.Second))
}
