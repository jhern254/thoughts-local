package tui

import (
	"context"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/tui/daynav"
)

type MetricsService interface {
	daynav.Reader
	CountThoughts(context.Context, string) (int64, error)
	CountUnassignedThoughts(context.Context, string) (int64, error)
	ThoughtCountsBySubject(context.Context, string) ([]data.SubjectThoughtCountView, error)
}
