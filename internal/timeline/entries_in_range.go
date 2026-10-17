package timeline

import (
	"context"
	"slices"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

// EntryKey keeps persisted event IDs and generated timestamp groups in separate
// namespaces. A zero key identifies no selection; constructors normalize timestamps to UTC.
type EntryKey struct {
	EventID    int64
	ObservedAt time.Time
}

func (k EntryKey) IsZero() bool { return k == (EntryKey{}) }

// Entry contains exactly one of Event or ThoughtsOutsideEvents. Thought groups
// outside events carry no event ID, activity, or duration and must never be passed
// to event mutation operations.
type Entry struct {
	Event                 *data.Event
	ThoughtsOutsideEvents *data.ThoughtGroupOutsideEvents
}

func (e Entry) Key() EntryKey {
	if e.Event != nil {
		return EntryKey{EventID: e.Event.EventID}
	}
	return EntryKey{ObservedAt: e.ThoughtsOutsideEvents.ObservedAt.UTC()}
}

func (e Entry) StartedAt() time.Time {
	if e.Event != nil {
		return e.Event.StartedAt
	}
	return e.ThoughtsOutsideEvents.ObservedAt
}

// ListEntriesInRange returns real events and thoughts outside event intervals
// in chronological order. The two reads do not form a transactional snapshot.
func (s *Service) ListEntriesInRange(ctx context.Context, userID string, from, until time.Time) ([]Entry, error) {
	events, err := s.events.List(ctx, userID, from, until)
	if err != nil {
		return nil, err
	}
	thoughtGroupsOutsideEvents, err := s.metrics.ListThoughtGroupsOutsideEvents(ctx, userID, from, until, s.now().UTC().Truncate(time.Second).Add(time.Second))
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(events)+len(thoughtGroupsOutsideEvents))
	for i := range events {
		entries = append(entries, Entry{Event: &events[i]})
	}
	for i := range thoughtGroupsOutsideEvents {
		entries = append(entries, Entry{ThoughtsOutsideEvents: &thoughtGroupsOutsideEvents[i]})
	}
	slices.SortStableFunc(entries, func(a, b Entry) int { return a.StartedAt().Compare(b.StartedAt()) })
	return entries, nil
}

func (s *Service) OpenThoughtsOutsideEventsView(ctx context.Context, userID string, observedAt time.Time) (ThoughtScope, error) {
	from := observedAt.UTC().Truncate(time.Second)
	until := from.Add(time.Second)
	thoughtGroupsOutsideEvents, err := s.metrics.ListThoughtGroupsOutsideEvents(ctx, userID, from, until, s.now().UTC().Truncate(time.Second).Add(time.Second))
	if err != nil {
		return ThoughtScope{}, err
	}
	if len(thoughtGroupsOutsideEvents) == 0 {
		return ThoughtScope{}, data.ErrRecordNotFound
	}
	return ThoughtScope{userID: userID, key: EntryKey{ObservedAt: from}, from: from, until: until}, nil
}
