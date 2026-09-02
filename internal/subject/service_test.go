package subject

import (
	"context"
	"errors"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
)

type subjectServiceStoreStub struct {
	create       func(context.Context, *data.Subject) (*data.Subject, error)
	get          func(context.Context, string, int64) (*data.Subject, error)
	createCalled bool
}

func (s *subjectServiceStoreStub) CreateSubject(ctx context.Context, item *data.Subject) (*data.Subject, error) {
	s.createCalled = true
	if s.create == nil {
		panic("CreateSubject not stubbed")
	}
	return s.create(ctx, item)
}

func (s *subjectServiceStoreStub) GetSubject(ctx context.Context, userID string, subjectID int64) (*data.Subject, error) {
	if s.get == nil {
		panic("GetSubject not stubbed")
	}
	return s.get(ctx, userID, subjectID)
}

func TestSubjectService_Create(t *testing.T) {
	t.Run("creates normalized subject", func(t *testing.T) {
		store := &subjectServiceStoreStub{create: func(_ context.Context, item *data.Subject) (*data.Subject, error) {
			if item.UserID != "test-user" || item.SubjectName != "learn Go" {
				t.Fatalf("got subject %#v", item)
			}
			if item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() {
				t.Fatal("expected timestamps")
			}
			created := *item
			created.SubjectID = 1
			return &created, nil
		}}

		got, err := NewService(store).Create(context.Background(), "test-user", "  learn   Go  ")

		if err != nil {
			t.Fatal(err)
		}
		if got.SubjectID != 1 || got.SubjectName != "learn Go" {
			t.Fatalf("got subject %#v", got)
		}
	})

	t.Run("rejects invalid subject before persistence", func(t *testing.T) {
		store := &subjectServiceStoreStub{}

		_, err := NewService(store).Create(context.Background(), "test-user", " ")

		var validationErr *ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("got error %v, want validation error", err)
		}
		if _, ok := validationErr.Fields["subject_name"]; !ok {
			t.Errorf("got validation fields %v, want subject_name", validationErr.Fields)
		}
		if store.createCalled {
			t.Error("store was called for invalid subject")
		}
	})

	t.Run("returns store error", func(t *testing.T) {
		want := errors.New("boom")
		store := &subjectServiceStoreStub{create: func(context.Context, *data.Subject) (*data.Subject, error) { return nil, want }}

		_, err := NewService(store).Create(context.Background(), "test-user", "coding")

		if !errors.Is(err, want) {
			t.Fatalf("got %v, want %v", err, want)
		}
	})
}

func TestSubjectService_Get(t *testing.T) {
	t.Run("returns subject", func(t *testing.T) {
		want := &data.Subject{SubjectID: 7, UserID: "test-user", SubjectName: "coding"}
		store := &subjectServiceStoreStub{get: func(_ context.Context, userID string, subjectID int64) (*data.Subject, error) {
			if userID != "test-user" || subjectID != 7 {
				t.Fatalf("got user %q and subject ID %d", userID, subjectID)
			}
			return want, nil
		}}

		got, err := NewService(store).Get(context.Background(), "test-user", 7)

		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("returns store error", func(t *testing.T) {
		want := errors.New("boom")
		store := &subjectServiceStoreStub{get: func(context.Context, string, int64) (*data.Subject, error) { return nil, want }}

		_, err := NewService(store).Get(context.Background(), "test-user", 7)

		if !errors.Is(err, want) {
			t.Fatalf("got %v, want %v", err, want)
		}
	})
}
