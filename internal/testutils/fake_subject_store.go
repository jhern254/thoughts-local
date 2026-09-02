package testutils

import (
	"context"

	"github.com/jhern254/go-thoughts/internal/data"
)

type FakeSubjectStore struct {
	subjects map[int64]data.Subject
	lastID   int64
}

func NewFakeSubjectStore() *FakeSubjectStore {
	return &FakeSubjectStore{subjects: make(map[int64]data.Subject)}
}

func (s *FakeSubjectStore) CreateSubject(ctx context.Context, subject *data.Subject) (*data.Subject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
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

var _ data.SubjectStore = (*FakeSubjectStore)(nil)
