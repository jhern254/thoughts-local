package testutils

import (
	"context"
	"sort"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

type FakeSubjectStore struct {
	subjects    map[int64]data.Subject
	lastID      int64
	aliases     map[int64]data.SubjectAlias
	lastAliasID int64
}

func NewFakeSubjectStore() *FakeSubjectStore {
	return &FakeSubjectStore{
		subjects: make(map[int64]data.Subject),
		aliases:  make(map[int64]data.SubjectAlias),
	}
}

func (s *FakeSubjectStore) CreateSubject(ctx context.Context, subject *data.Subject) (*data.Subject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, alias := range s.aliases {
		if alias.UserID == subject.UserID && alias.AliasName == subject.SubjectName {
			canonical := s.subjects[alias.SubjectID]
			return nil, &data.SubjectAliasConflictError{
				AliasName:   alias.AliasName,
				SubjectID:   canonical.SubjectID,
				SubjectName: canonical.SubjectName,
			}
		}
	}
	s.lastID++
	created := *subject
	created.SubjectID = s.lastID
	s.subjects[created.SubjectID] = created
	return &created, nil
}

func (s *FakeSubjectStore) GetSubject(ctx context.Context, userID string, subjectID int64) (*data.Subject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	subject, ok := s.subjects[subjectID]
	if !ok || subject.UserID != userID {
		return nil, data.ErrRecordNotFound
	}
	return &subject, nil
}

func (s *FakeSubjectStore) ListSubjects(ctx context.Context, userID string) ([]data.Subject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	subjects := make([]data.Subject, 0)
	for _, subject := range s.subjects {
		if subject.UserID == userID {
			subjects = append(subjects, subject)
		}
	}
	sort.Slice(subjects, func(i, j int) bool {
		return subjects[i].SubjectID < subjects[j].SubjectID
	})
	return subjects, nil
}

func (s *FakeSubjectStore) UpdateSubject(ctx context.Context, userID string, subjectID int64, name string, updatedAt time.Time) (*data.Subject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	subject, ok := s.subjects[subjectID]
	if !ok || subject.UserID != userID {
		return nil, data.ErrRecordNotFound
	}
	for id, existing := range s.subjects {
		if id != subjectID && existing.UserID == userID && existing.SubjectName == name {
			return nil, data.ErrDuplicateRecord
		}
	}
	subject.SubjectName = name
	for _, alias := range s.aliases {
		if alias.UserID == userID && alias.AliasName == name {
			return nil, data.ErrDuplicateRecord
		}
	}
	subject.UpdatedAt = updatedAt
	s.subjects[subjectID] = subject
	return &subject, nil
}

func (s *FakeSubjectStore) DeleteSubject(ctx context.Context, userID string, subjectID int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	subject, ok := s.subjects[subjectID]
	if !ok || subject.UserID != userID {
		return data.ErrRecordNotFound
	}
	delete(s.subjects, subjectID)
	for id, alias := range s.aliases {
		if alias.SubjectID == subjectID && alias.UserID == userID {
			delete(s.aliases, id)
		}
	}
	return nil
}

var _ data.SubjectStore = (*FakeSubjectStore)(nil)
