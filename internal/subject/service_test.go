package subject

import (
	"context"
	"errors"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
)

type createStoreStub struct {
	called bool
}

func (s *createStoreStub) CreateSubject(context.Context, *data.Subject) (*data.Subject, error) {
	s.called = true
	return &data.Subject{SubjectID: 1}, nil
}

func (s *createStoreStub) GetSubject(context.Context, string, int64) (*data.Subject, error) {
	return nil, data.ErrRecordNotFound
}

func TestSubjectService_Create(t *testing.T) {
	t.Run("rejects invalid subject before persistence", func(t *testing.T) {
		store := &createStoreStub{}
		service := NewService(store)

		_, err := service.Create(context.Background(), "test-user", " ")

		var validationErr *ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("got error %v, want validation error", err)
		}
		if _, ok := validationErr.Fields["subject_name"]; !ok {
			t.Errorf("got validation fields %v, want subject_name", validationErr.Fields)
		}
		if store.called {
			t.Error("store was called for invalid subject")
		}
	})
}
