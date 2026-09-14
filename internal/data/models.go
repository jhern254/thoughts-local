package data

import "time"

type User struct {
	UserID    string
	Handle    *string
	AltHandle *string
	Email     *string
	Version   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Subject struct {
	SubjectID   int64
	UserID      string
	SubjectName string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Thought struct {
	ThoughtID  int64
	UserID     string
	SubjectID  *int64
	EventID    *int64
	Thought    string
	Version    int64
	ObservedAt time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Event is an activity interval. A nil EndedAt means the event is ongoing.
type Event struct {
	EventID      int64
	UserID       string
	ActivityType *string
	StartedAt    time.Time
	EndedAt      *time.Time
	Version      int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Goal stores explicit settings. Dates are nullable calendar strings, not instants.
type Goal struct {
	GoalID         int64
	UserID         string
	GoalName       string
	TargetSeconds  int64
	StartDate      *string
	EndDate        *string
	IsActive       bool
	Cadence        string
	TZ             string
	WeekStart      string
	DefaultCadence *string
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// GoalProgress records an explicit contribution and optional provenance.
type GoalProgress struct {
	ProgressID   int64
	GoalID       int64
	UserID       string
	OccurredAt   time.Time
	TimeSpentSec int64
	EventID      *int64
	ThoughtID    *int64
	ProgressNote *string
	CreatedAt    time.Time
}

// GoalParent links a child GoalID to a broader ParentGoalID with the same owner.
type GoalParent struct {
	GoalID       int64
	ParentGoalID int64
	UserID       string
}
