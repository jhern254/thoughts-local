package thought

import (
	"context"
	"strings"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/validator"
)

const maxThoughtBytes = 1_000_000

type Store interface {
	CreateThought(context.Context, *data.Thought) (*data.Thought, error)
	GetThought(context.Context, string, int64) (*data.Thought, error)
}

type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string { return "thought validation failed" }

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) Get(ctx context.Context, userID string, thoughtID int64) (*data.Thought, error) {
	return s.store.GetThought(ctx, userID, thoughtID)
}

func (s *Service) Create(ctx context.Context, userID, body string, subjectID *int64, observedAt time.Time) (*data.Thought, error) {
	now := time.Now().UTC()
	if observedAt.IsZero() {
		observedAt = now
	}
	item := &data.Thought{UserID: userID, SubjectID: subjectID, Thought: strings.TrimSpace(body), Version: 1, ObservedAt: observedAt.UTC(), CreatedAt: now, UpdatedAt: now}
	v := validator.NewValidator()
	v.Check(item.UserID != "", "user_id", "must be provided")
	v.Check(item.Thought != "", "thought", "must be provided")
	v.Check(len(item.Thought) <= maxThoughtBytes, "thought", "must not be more than 1000000 bytes long")
	if item.SubjectID != nil {
		v.Check(*item.SubjectID > 0, "subject_id", "must be positive")
	}
	if !v.Valid() {
		return nil, &ValidationError{Fields: v.Errors}
	}
	return s.store.CreateThought(ctx, item)
}
