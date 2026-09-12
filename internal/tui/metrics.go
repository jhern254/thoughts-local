package tui

import (
	"context"

	"github.com/jhern254/go-thoughts/internal/data"
)

type MetricsService interface {
	CountThoughts(context.Context, string) (int64, error)
	CountUnassignedThoughts(context.Context, string) (int64, error)
	ThoughtCountsBySubject(context.Context, string) ([]data.SubjectThoughtCount, error)
}
