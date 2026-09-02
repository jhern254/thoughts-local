package data

import "context"

type SubjectStore interface {
	CreateSubject(context.Context, *Subject) (*Subject, error)
	GetSubject(context.Context, string, int64) (*Subject, error)
}

type ThoughtStore interface {
	CreateThought(context.Context, *Thought) (*Thought, error)
	GetThought(context.Context, string, int64) (*Thought, error)
}
