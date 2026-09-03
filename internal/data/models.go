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
