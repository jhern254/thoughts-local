package thought

import (
	"context"
	"maps"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/validator"
)

func (s *Service) SetMindset(ctx context.Context, userID string, thoughtID int64, active bool, at time.Time) (bool, error) {
	v := validator.NewValidator()
	v.Check(userID != "", "user_id", "must be provided")
	v.Check(!at.IsZero(), "action_time", "must be provided")
	if !v.Valid() {
		return false, &ValidationError{Fields: v.Errors, publicFields: maps.Clone(v.Errors)}
	}
	return s.store.SetThoughtMindset(ctx, userID, thoughtID, active, at.UTC())
}

func (s *Service) ListMindsetsInRange(ctx context.Context, userID string, from, until time.Time) ([]data.MindsetThought, error) {
	v := validator.NewValidator()
	v.Check(userID != "", "user_id", "must be provided")
	v.Check(until.After(from), "range", "must end after it starts")
	if !v.Valid() {
		return nil, &ValidationError{Fields: v.Errors, publicFields: maps.Clone(v.Errors)}
	}
	return s.store.ListMindsetsInRange(ctx, userID, from.UTC(), until.UTC())
}
