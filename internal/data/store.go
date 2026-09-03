package data

import (
	"context"
	"time"
)

type UserStore interface {
	CreateUser(context.Context, *User) (*User, error)
	GetUserByHandle(context.Context, string) (*User, error)
}

type SubjectStore interface {
	CreateSubject(context.Context, *Subject) (*Subject, error)
	GetSubject(context.Context, string, int64) (*Subject, error)
	ListSubjects(context.Context, string) ([]Subject, error)
	UpdateSubject(context.Context, string, int64, string, time.Time) (*Subject, error)
	DeleteSubject(context.Context, string, int64) error
}

type ThoughtStore interface {
	CreateThought(context.Context, *Thought) (*Thought, error)
	GetThought(context.Context, string, int64) (*Thought, error)
}
