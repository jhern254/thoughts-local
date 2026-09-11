// Package metrics provides read-only aggregate queries shared by delivery surfaces.
package metrics

import (
	"context"

	"github.com/jhern254/go-thoughts/internal/data"
)

type Store interface {
	CountThoughts(context.Context, string) (int64, error)
	CountUnassignedThoughts(context.Context, string) (int64, error)
	ThoughtCountsBySubject(context.Context, string) ([]data.SubjectThoughtCount, error)
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) CountThoughts(ctx context.Context, userID string) (int64, error) {
	return s.store.CountThoughts(ctx, userID)
}

func (s *Service) CountUnassignedThoughts(ctx context.Context, userID string) (int64, error) {
	return s.store.CountUnassignedThoughts(ctx, userID)
}

func (s *Service) ThoughtCountsBySubject(ctx context.Context, userID string) ([]data.SubjectThoughtCount, error) {
	return s.store.ThoughtCountsBySubject(ctx, userID)
}
