package subject

import (
	"context"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/validator"
)

type Store interface {
	CreateSubject(ctx context.Context, subject *data.Subject) (*data.Subject, error)
	GetSubject(ctx context.Context, userID string, subjectID int64) (*data.Subject, error)
	ListSubjects(ctx context.Context, userID string) ([]data.Subject, error)
	UpdateSubject(ctx context.Context, userID string, subjectID int64, name string, updatedAt time.Time) (*data.Subject, error)
	DeleteSubject(ctx context.Context, userID string, subjectID int64) error
}

type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	return "subject validation failed"
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Get(ctx context.Context, userID string, subjectID int64) (*data.Subject, error) {
	return s.store.GetSubject(ctx, userID, subjectID)
}

func (s *Service) List(ctx context.Context, userID string) ([]data.Subject, error) {
	return s.store.ListSubjects(ctx, userID)
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
	data.ValidateSubject(v, subject)
	if !v.Valid() {
		return nil, &ValidationError{Fields: v.Errors}
	}

	return s.store.CreateSubject(ctx, subject)
}

func (s *Service) Update(ctx context.Context, userID string, subjectID int64, name string) (*data.Subject, error) {
	subject := &data.Subject{UserID: userID, SubjectName: name}
	v := validator.NewValidator()
	data.ValidateSubject(v, subject)
	if !v.Valid() {
		return nil, &ValidationError{Fields: v.Errors}
	}
	return s.store.UpdateSubject(ctx, userID, subjectID, name, time.Now().UTC())
}

func (s *Service) Delete(ctx context.Context, userID string, subjectID int64) error {
	return s.store.DeleteSubject(ctx, userID, subjectID)
}
