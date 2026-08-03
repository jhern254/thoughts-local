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

func (s *createStoreStub) CaptureSubject(context.Context, string, *data.Subject) (int64, error) {
	s.called = true
	return 1, nil
}

func TestCreateRejectsInvalidSubjectBeforePersistence(t *testing.T) {
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
}
