package thought

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/validator"
)

const maxThoughtCharacters = 1_000_000

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
	item := &data.Thought{UserID: userID, SubjectID: subjectID, Thought: body, Version: 1, ObservedAt: observedAt.UTC(), CreatedAt: now, UpdatedAt: now}
	v := validator.NewValidator()
	v.Check(item.UserID != "", "user_id", "must be provided")
	v.Check(utf8.RuneCountInString(strings.Trim(item.Thought, " ")) > 0, "thought", "must be provided")
	v.Check(utf8.RuneCountInString(item.Thought) <= maxThoughtCharacters, "thought", "must not be more than 1000000 characters long")
	if !v.Valid() {
		return nil, &ValidationError{Fields: v.Errors}
	}
	return s.store.CreateThought(ctx, item)
}
