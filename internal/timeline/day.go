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

// Entry contains exactly one of Event or Misc. Misc groups carry no event ID,
// activity, or duration and must never be passed to event mutation operations.
type Entry struct {
	Event *data.Event
	Misc  *data.MiscThoughtGroup
}

func (e Entry) Key() EntryKey {
	if e.Event != nil {
		return EntryKey{EventID: e.Event.EventID}
	}
	return EntryKey{ObservedAt: e.Misc.ObservedAt.UTC()}
}

func (e Entry) StartedAt() time.Time {
	if e.Event != nil {
		return e.Event.StartedAt
	}
	return e.Misc.ObservedAt
}

// Day coordinates the two reads without claiming a transactional snapshot.
// Generated groups are a shared read projection, available to any delivery surface.
func (s *Service) Day(ctx context.Context, userID string, from, until time.Time) ([]Entry, error) {
	events, err := s.events.List(ctx, userID, from, until)
	if err != nil {
		return nil, err
	}
	groups, err := s.metrics.MiscThoughtGroups(ctx, userID, from, until, s.now().UTC().Truncate(time.Second).Add(time.Second))
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(events)+len(groups))
	for i := range events {
		entries = append(entries, Entry{Event: &events[i]})
	}
	for i := range groups {
		entries = append(entries, Entry{Misc: &groups[i]})
	}
	slices.SortStableFunc(entries, func(a, b Entry) int { return a.StartedAt().Compare(b.StartedAt()) })
	return entries, nil
}

func (s *Service) OpenMiscThoughtsView(ctx context.Context, userID string, observedAt time.Time) (ThoughtScope, error) {
	from := observedAt.UTC().Truncate(time.Second)
	until := from.Add(time.Second)
	groups, err := s.metrics.MiscThoughtGroups(ctx, userID, from, until, s.now().UTC().Truncate(time.Second).Add(time.Second))
	if err != nil {
		return ThoughtScope{}, err
	}
	if len(groups) == 0 {
		return ThoughtScope{}, data.ErrRecordNotFound
	}
	return ThoughtScope{userID: userID, key: EntryKey{ObservedAt: from}, from: from, until: until}, nil
}
