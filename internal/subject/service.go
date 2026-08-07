package subject

import (
	"context"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/validator"
)

type CreateStore interface {
	CreateSubject(ctx context.Context, subject *data.Subject) (*data.Subject, error)
}

type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	return "subject validation failed"
}

type Service struct {
	store CreateStore
}

func NewService(store CreateStore) *Service {
	return &Service{store: store}
}

func (s *Service) Create(ctx context.Context, userID, name string) (*data.Subject, error) {
	now := time.Now().UTC()
	subject := &data.Subject{
		UserID:      userID,
		SubjectName: name,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	v := validator.NewValidator()
	data.ValidateSubjectCreate(v, subject)
	if !v.Valid() {
		return nil, &ValidationError{Fields: v.Errors}
	}

	return s.store.CreateSubject(ctx, subject)
}
