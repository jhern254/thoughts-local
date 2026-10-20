package testutils

import (
	"context"
	"sort"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

func (s *FakeSubjectStore) ListSubjectAliases(ctx context.Context, userID string, subjectID int64) ([]data.SubjectAlias, error) {
	if _, err := s.GetSubject(ctx, userID, subjectID); err != nil {
		return nil, err
	}
	aliases := make([]data.SubjectAlias, 0)
	for _, alias := range s.aliases {
		if alias.UserID == userID && alias.SubjectID == subjectID {
			aliases = append(aliases, alias)
		}
	}
	sort.Slice(aliases, func(i, j int) bool { return aliases[i].AliasID < aliases[j].AliasID })
	return aliases, nil
}

func (s *FakeSubjectStore) ListUserSubjectAliases(ctx context.Context, userID string) ([]data.SubjectAlias, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	aliases := make([]data.SubjectAlias, 0)
	for _, alias := range s.aliases {
		if alias.UserID == userID {
			aliases = append(aliases, alias)
		}
	}
	sort.Slice(aliases, func(i, j int) bool { return aliases[i].AliasName < aliases[j].AliasName })
	return aliases, nil
}

func (s *FakeSubjectStore) CreateSubjectAlias(ctx context.Context, alias *data.SubjectAlias) (*data.SubjectAlias, error) {
	if _, err := s.GetSubject(ctx, alias.UserID, alias.SubjectID); err != nil {
		return nil, err
	}
	if _, err := s.ResolveSubjectName(ctx, alias.UserID, alias.AliasName); err == nil {
		return nil, data.ErrDuplicateRecord
	}
	s.lastAliasID++
	created := *alias
	created.AliasID = s.lastAliasID
	s.aliases[created.AliasID] = created
	return &created, nil
}

func (s *FakeSubjectStore) UpdateSubjectAlias(ctx context.Context, userID string, subjectID, aliasID int64, name string, updatedAt time.Time) (*data.SubjectAlias, error) {
	if _, err := s.GetSubject(ctx, userID, subjectID); err != nil {
		return nil, err
	}
	alias, ok := s.aliases[aliasID]
	if !ok || alias.UserID != userID || alias.SubjectID != subjectID {
		return nil, data.ErrRecordNotFound
	}
	if name != alias.AliasName {
		if _, err := s.ResolveSubjectName(ctx, userID, name); err == nil {
			return nil, data.ErrDuplicateRecord
		}
	}
	alias.AliasName = name
	alias.UpdatedAt = updatedAt
	s.aliases[aliasID] = alias
	return &alias, nil
}

func (s *FakeSubjectStore) RemoveSubjectAlias(ctx context.Context, userID string, subjectID, aliasID int64) error {
	if _, err := s.GetSubject(ctx, userID, subjectID); err != nil {
		return err
	}
	alias, ok := s.aliases[aliasID]
	if !ok || alias.UserID != userID || alias.SubjectID != subjectID {
		return data.ErrRecordNotFound
	}
	delete(s.aliases, aliasID)
	return nil
}

func (s *FakeSubjectStore) ResolveSubjectName(ctx context.Context, userID, name string) (*data.Subject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, item := range s.subjects {
		if item.UserID == userID && item.SubjectName == name {
			return &item, nil
		}
	}
	for _, alias := range s.aliases {
		if alias.UserID == userID && alias.AliasName == name {
			return s.GetSubject(ctx, userID, alias.SubjectID)
		}
	}
	return nil, data.ErrRecordNotFound
}
