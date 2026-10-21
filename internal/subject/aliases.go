package subject

import (
	"context"
	"maps"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/validator"
)

type AliasStore interface {
	ListSubjectAliases(ctx context.Context, userID string, subjectID int64) ([]data.SubjectAlias, error)
	ListUserSubjectAliases(ctx context.Context, userID string) ([]data.SubjectAlias, error)
	CreateSubjectAlias(ctx context.Context, alias *data.SubjectAlias) (*data.SubjectAlias, error)
	UpdateSubjectAlias(ctx context.Context, userID string, subjectID, aliasID int64, name string, updatedAt time.Time) (*data.SubjectAlias, error)
	RemoveSubjectAlias(ctx context.Context, userID string, subjectID, aliasID int64) error
	ResolveSubjectName(ctx context.Context, userID, name string) (*data.Subject, error)
}

func (s *Service) ListAliases(ctx context.Context, userID string, subjectID int64) ([]data.SubjectAlias, error) {
	return s.store.ListSubjectAliases(ctx, userID, subjectID)
}

func (s *Service) ListAliasesForUser(ctx context.Context, userID string) ([]data.SubjectAlias, error) {
	return s.store.ListUserSubjectAliases(ctx, userID)
}

func (s *Service) CreateAlias(ctx context.Context, userID string, subjectID int64, name string) (*data.SubjectAlias, error) {
	if err := validateAliasName(userID, name); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	alias := &data.SubjectAlias{
		SubjectID: subjectID,
		UserID:    userID,
		AliasName: name,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return s.store.CreateSubjectAlias(ctx, alias)
}

func (s *Service) UpdateAlias(ctx context.Context, userID string, subjectID, aliasID int64, name string) (*data.SubjectAlias, error) {
	if err := validateAliasName(userID, name); err != nil {
		return nil, err
	}
	return s.store.UpdateSubjectAlias(ctx, userID, subjectID, aliasID, name, time.Now().UTC())
}

func (s *Service) RemoveAlias(ctx context.Context, userID string, subjectID, aliasID int64) error {
	return s.store.RemoveSubjectAlias(ctx, userID, subjectID, aliasID)
}

// ResolveName accepts exact canonical names and aliases; it does not perform
// case folding, approximate matching, or changes to existing picker searches.
func (s *Service) ResolveName(ctx context.Context, userID, name string) (*data.Subject, error) {
	return s.store.ResolveSubjectName(ctx, userID, name)
}

func validateAliasName(userID, name string) error {
	v := validator.NewValidator()
	data.ValidateSubject(v, &data.Subject{UserID: userID, SubjectName: name})
	if v.Valid() {
		return nil
	}
	if message, ok := v.Errors["subject_name"]; ok {
		delete(v.Errors, "subject_name")
		v.Errors["alias_name"] = message
	}
	return &ValidationError{Fields: v.Errors, publicFields: maps.Clone(v.Errors)}
}
