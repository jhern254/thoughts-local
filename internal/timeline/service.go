// Package timeline coordinates reads across entities for timeline presentation.
// Entity services retain their own operations; SQL remains in the stores.
package timeline

import (
	"context"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

type EventReader interface {
	Get(context.Context, string, int64) (*data.Event, error)
}

type ThoughtReader interface {
	ListThoughtsInRange(context.Context, string, time.Time, time.Time) ([]data.Thought, error)
}

type Service struct {
	events   EventReader
	thoughts ThoughtReader
	now      func() time.Time
}

// NewService requires both readers; neither is an optional dependency.
func NewService(events EventReader, thoughts ThoughtReader) *Service {
	return &Service{events: events, thoughts: thoughts, now: time.Now}
}

// ListThoughts matches observation times, not explicit event links or calendar
// days. Completed intervals exclude their end; ongoing intervals include the
// captured current second. The two reads are not a transactional snapshot.
func (s *Service) ListThoughts(ctx context.Context, userID string, eventID int64) ([]data.Thought, error) {
	item, err := s.events.Get(ctx, userID, eventID)
	if err != nil {
		return nil, err
	}
	var until time.Time
	if item.EndedAt == nil {
		until = s.now().UTC().Truncate(time.Second).Add(time.Second)
	} else {
		until = *item.EndedAt
	}
	return s.thoughts.ListThoughtsInRange(ctx, userID, item.StartedAt, until)
}
