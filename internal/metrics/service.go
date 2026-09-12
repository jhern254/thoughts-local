// Package metrics provides read-only aggregate queries shared by delivery surfaces.
package metrics

import (
	"context"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

type Store interface {
	ThoughtCountsByEvent(context.Context, string, time.Time, time.Time, time.Time) ([]data.EventThoughtCount, error)
	CountThoughtsInRange(context.Context, string, time.Time, time.Time) (int64, error)
	CountThoughts(context.Context, string) (int64, error)
	CountUnassignedThoughts(context.Context, string) (int64, error)
	ThoughtCountsBySubject(context.Context, string) ([]data.SubjectThoughtCount, error)
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) ThoughtCountsByEvent(ctx context.Context, userID string, from, until, ongoingUntil time.Time) ([]data.EventThoughtCount, error) {
	return s.store.ThoughtCountsByEvent(ctx, userID, from, until, ongoingUntil)
}

func (s *Service) CountThoughtsInRange(ctx context.Context, userID string, from, until time.Time) (int64, error) {
	return s.store.CountThoughtsInRange(ctx, userID, from, until)
}

func (s *Service) CountThoughts(ctx context.Context, userID string) (int64, error) {
	return s.store.CountThoughts(ctx, userID)
}

func (s *Service) CountUnassignedThoughts(ctx context.Context, userID string) (int64, error) {
	return s.store.CountUnassignedThoughts(ctx, userID)
}

func (s *Service) ThoughtCountsBySubject(ctx context.Context, userID string) ([]data.SubjectThoughtCount, error) {
	return s.store.ThoughtCountsBySubject(ctx, userID)
}
