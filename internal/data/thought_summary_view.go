package data

import "time"

const ThoughtSummaryViewBatchSize = 50

type ThoughtDirection uint8

const (
	ThoughtsOlder ThoughtDirection = iota
	ThoughtsNewer
)

// ThoughtSummaryView is a bounded read projection, not a partially loaded Thought.
// CreatedAt is a cursor tie-breaker, not the displayed observation time.
type ThoughtSummaryView struct {
	ThoughtID   int64
	Preview     string
	SubjectName *string
	ObservedAt  time.Time
	CreatedAt   time.Time
}

type ThoughtCursor struct {
	ObservedAt time.Time
	CreatedAt  time.Time
	ThoughtID  int64
}

func (s ThoughtSummaryView) Cursor() ThoughtCursor {
	return ThoughtCursor{ObservedAt: s.ObservedAt, CreatedAt: s.CreatedAt, ThoughtID: s.ThoughtID}
}

type ThoughtSummaryViewRequest struct {
	Cursor    *ThoughtCursor
	Direction ThoughtDirection
}

// ThoughtSummaryViewResult is one bounded batch of summary-view rows.
// Items are always newest first. More refers to the requested direction.
type ThoughtSummaryViewResult struct {
	Items []ThoughtSummaryView
	More  bool
}
