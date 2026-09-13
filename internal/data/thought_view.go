package data

import "time"

const ThoughtViewBatchSize = 50

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

type ThoughtViewRequest struct {
	Cursor    *ThoughtCursor
	Direction ThoughtDirection
}

// Items are always newest first. More refers to the requested direction.
type ThoughtView struct {
	Items []ThoughtSummaryView
	More  bool
}
