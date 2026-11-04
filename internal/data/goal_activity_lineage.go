package data

import "time"

const GoalEventPageSize = 25

// GoalEventLineage describes one completed Event that earned time for a Goal or
// a Goal beneath it. It is a read projection, not an Event mutation target.
type GoalEventLineage struct {
	EventID        int64
	Activity       string
	StartedAt      time.Time
	EndedAt        time.Time
	ElapsedSeconds int64
}

type GoalEventCursor struct {
	StartedAt time.Time
	EventID   int64
}

func (e GoalEventLineage) Cursor() GoalEventCursor {
	return GoalEventCursor{StartedAt: e.StartedAt, EventID: e.EventID}
}

type GoalEventDirection uint8

const (
	EarnedEventsOlder GoalEventDirection = iota
	EarnedEventsNewer
)

type GoalEventPageRequest struct {
	Cursor    *GoalEventCursor
	Direction GoalEventDirection
}

// Pages are always newest first; More refers to the requested direction.
type GoalEventPage struct {
	Events []GoalEventLineage
	More   bool
}

type GoalEventThoughtPage struct {
	Event    GoalEventLineage
	Thoughts ThoughtSummaryViewResult
}
