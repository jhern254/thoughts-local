package subject

import (
	"context"
	"maps"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/validator"
)

// CreateFromAlias explicitly replaces an alias with a new, independent Subject.
// The store owns the atomic removal/create boundary, including ownership checks.
func (s *Service) CreateFromAlias(ctx context.Context, userID string, subjectID, aliasID int64, name string) (*data.Subject, error) {
	now := time.Now().UTC()
	created := &data.Subject{
		UserID:      userID,
		SubjectName: name,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	validation := validator.NewValidator()
	data.ValidateSubject(validation, created)
	if !validation.Valid() {
		return nil, &ValidationError{Fields: validation.Errors, publicFields: maps.Clone(validation.Errors)}
	}
	return s.store.CreateSubjectFromAlias(ctx, created, subjectID, aliasID)
}
