package subject

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

type subjectServiceStoreStub struct {
	create       func(context.Context, *data.Subject) (*data.Subject, error)
	get          func(context.Context, string, int64) (*data.Subject, error)
	list         func(context.Context, string) ([]data.Subject, error)
	update       func(context.Context, string, int64, string, time.Time) (*data.Subject, error)
	delete       func(context.Context, string, int64) error
	createCalled bool
	updateCalled bool
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

func (s *subjectServiceStoreStub) ListSubjects(ctx context.Context, userID string) ([]data.Subject, error) {
	if s.list == nil {
		panic("ListSubjects not stubbed")
	}
	return s.list(ctx, userID)
}

func (s *subjectServiceStoreStub) UpdateSubject(ctx context.Context, userID string, subjectID int64, name string, updatedAt time.Time) (*data.Subject, error) {
	s.updateCalled = true
	if s.update == nil {
		panic("UpdateSubject not stubbed")
	}
	return s.update(ctx, userID, subjectID, name, updatedAt)
}

func (s *subjectServiceStoreStub) DeleteSubject(ctx context.Context, userID string, subjectID int64) error {
	if s.delete == nil {
		panic("DeleteSubject not stubbed")
	}
	return s.delete(ctx, userID, subjectID)
}

func TestSubjectService_Create(t *testing.T) {
	t.Run("creates subject without rewriting schema-valid whitespace", func(t *testing.T) {
		store := &subjectServiceStoreStub{create: func(_ context.Context, item *data.Subject) (*data.Subject, error) {
			if item.UserID != "test-user" || item.SubjectName != "  learn   Go  " {
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
		if got.SubjectID != 1 || got.SubjectName != "  learn   Go  " {
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

func TestSubjectService_List(t *testing.T) {
	t.Run("returns subjects from store", func(t *testing.T) {
		want := []data.Subject{
			{SubjectID: 1, UserID: "test-user", SubjectName: "coding"},
			{SubjectID: 2, UserID: "test-user", SubjectName: "writing"},
		}
		store := &subjectServiceStoreStub{list: func(_ context.Context, userID string) ([]data.Subject, error) {
			if userID != "test-user" {
				t.Fatalf("got user %q", userID)
			}
			return want, nil
		}}

		got, err := NewService(store).List(context.Background(), "test-user")

		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].SubjectID != 1 || got[1].SubjectID != 2 {
			t.Fatalf("got subjects %#v, want %#v", got, want)
		}
	})

	t.Run("returns store error", func(t *testing.T) {
		want := errors.New("boom")
		store := &subjectServiceStoreStub{list: func(context.Context, string) ([]data.Subject, error) { return nil, want }}

		_, err := NewService(store).List(context.Background(), "test-user")

		if !errors.Is(err, want) {
			t.Fatalf("got %v, want %v", err, want)
		}
	})
}

func TestSubjectService_Update(t *testing.T) {
	t.Run("updates subject without rewriting schema-valid whitespace", func(t *testing.T) {
		before := time.Now().UTC()
		store := &subjectServiceStoreStub{update: func(_ context.Context, userID string, subjectID int64, name string, updatedAt time.Time) (*data.Subject, error) {
			if userID != "test-user" || subjectID != 7 || name != "  learn   Go  " {
				t.Fatalf("got user %q, subject ID %d, and name %q", userID, subjectID, name)
			}
			if updatedAt.Before(before) || updatedAt.Location() != time.UTC {
				t.Fatalf("got updated at %v", updatedAt)
			}
			return &data.Subject{SubjectID: subjectID, UserID: userID, SubjectName: name, UpdatedAt: updatedAt}, nil
		}}

		got, err := NewService(store).Update(context.Background(), "test-user", 7, "  learn   Go  ")

		if err != nil {
			t.Fatal(err)
		}
		if got.SubjectID != 7 || got.SubjectName != "  learn   Go  " {
			t.Fatalf("got subject %#v", got)
		}
	})

	t.Run("rejects invalid subject before persistence", func(t *testing.T) {
		tests := []struct {
			name   string
			userID string
			value  string
			field  string
		}{
			{name: "missing user", value: "coding", field: "user_id"},
			{name: "empty name", userID: "test-user", value: " ", field: "subject_name"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				store := &subjectServiceStoreStub{}

				_, err := NewService(store).Update(context.Background(), tt.userID, 7, tt.value)

				var validationErr *ValidationError
				if !errors.As(err, &validationErr) {
					t.Fatalf("got error %v, want validation error", err)
				}
				if _, ok := validationErr.Fields[tt.field]; !ok {
					t.Fatalf("got validation fields %v, want %s", validationErr.Fields, tt.field)
				}
				if store.updateCalled {
					t.Fatal("store was called for invalid subject")
				}
			})
		}
	})

	t.Run("returns store error", func(t *testing.T) {
		want := data.ErrDuplicateRecord
		store := &subjectServiceStoreStub{update: func(context.Context, string, int64, string, time.Time) (*data.Subject, error) {
			return nil, want
		}}

		_, err := NewService(store).Update(context.Background(), "test-user", 7, "coding")

		if !errors.Is(err, want) {
			t.Fatalf("got %v, want %v", err, want)
		}
	})
}

func TestSubjectService_Delete(t *testing.T) {
	t.Run("deletes subject for user", func(t *testing.T) {
		store := &subjectServiceStoreStub{delete: func(_ context.Context, userID string, subjectID int64) error {
			if userID != "test-user" || subjectID != 7 {
				t.Fatalf("got user %q and subject ID %d", userID, subjectID)
			}
			return nil
		}}

		err := NewService(store).Delete(context.Background(), "test-user", 7)

		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("returns store error", func(t *testing.T) {
		want := data.ErrRecordNotFound
		store := &subjectServiceStoreStub{delete: func(context.Context, string, int64) error { return want }}

		err := NewService(store).Delete(context.Background(), "test-user", 7)

		if !errors.Is(err, want) {
			t.Fatalf("got %v, want %v", err, want)
		}
	})
}
