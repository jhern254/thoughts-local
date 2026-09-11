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
