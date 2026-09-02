package thought

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

type thoughtServiceStoreStub struct {
	create       func(context.Context, *data.Thought) (*data.Thought, error)
	get          func(context.Context, string, int64) (*data.Thought, error)
	createCalled bool
}

func (s *thoughtServiceStoreStub) CreateThought(ctx context.Context, item *data.Thought) (*data.Thought, error) {
	s.createCalled = true
	if s.create == nil {
		panic("CreateThought not stubbed")
	}
	return s.create(ctx, item)
}

func (s *thoughtServiceStoreStub) GetThought(ctx context.Context, userID string, thoughtID int64) (*data.Thought, error) {
	if s.get == nil {
		panic("GetThought not stubbed")
	}
	return s.get(ctx, userID, thoughtID)
}

func TestThoughtService_Create(t *testing.T) {
	t.Run("creates thought with defaults without rewriting whitespace", func(t *testing.T) {
		subjectID := int64(3)
		store := &thoughtServiceStoreStub{create: func(_ context.Context, item *data.Thought) (*data.Thought, error) {
			if item.UserID != "test-user" || item.Thought != "  learn Go  " || item.Version != 1 {
				t.Fatalf("got thought %#v", item)
			}
			if item.SubjectID == nil || *item.SubjectID != subjectID {
				t.Fatalf("got subject ID %#v", item.SubjectID)
			}
			if item.ObservedAt.IsZero() || item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() {
				t.Fatal("expected timestamps")
			}
			created := *item
			created.ThoughtID = 1
			return &created, nil
		}}

		got, err := NewService(store).Create(context.Background(), "test-user", "  learn Go  ", &subjectID, time.Time{})

		if err != nil {
			t.Fatal(err)
		}
		if got.ThoughtID != 1 || got.Thought != "  learn Go  " {
			t.Fatalf("got thought %#v", got)
		}
	})

	t.Run("accepts one million Unicode characters", func(t *testing.T) {
		body := strings.Repeat("界", maxThoughtCharacters)
		store := &thoughtServiceStoreStub{create: func(_ context.Context, item *data.Thought) (*data.Thought, error) {
			return item, nil
		}}

		_, err := NewService(store).Create(context.Background(), "test-user", body, nil, time.Time{})

		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("preserves observed at in UTC", func(t *testing.T) {
		observedAt := time.Date(2026, time.August, 1, 5, 0, 0, 0, time.FixedZone("test", -7*60*60))
		store := &thoughtServiceStoreStub{create: func(_ context.Context, item *data.Thought) (*data.Thought, error) {
			if !item.ObservedAt.Equal(observedAt) || item.ObservedAt.Location() != time.UTC {
				t.Fatalf("got observed at %v", item.ObservedAt)
			}
			return item, nil
		}}

		_, err := NewService(store).Create(context.Background(), "test-user", "learn Go", nil, observedAt)

		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("rejects invalid thought before persistence", func(t *testing.T) {
		tests := []struct {
			name      string
			userID    string
			body      string
			subjectID *int64
			field     string
		}{
			{name: "missing user", body: "learn Go", field: "user_id"},
			{name: "empty body", userID: "test-user", body: " ", field: "thought"},
			{name: "oversized body", userID: "test-user", body: strings.Repeat("x", maxThoughtCharacters+1), field: "thought"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				store := &thoughtServiceStoreStub{}

				_, err := NewService(store).Create(context.Background(), tt.userID, tt.body, tt.subjectID, time.Time{})

				var validationErr *ValidationError
				if !errors.As(err, &validationErr) {
					t.Fatalf("got error %v, want validation error", err)
				}
				if _, ok := validationErr.Fields[tt.field]; !ok {
					t.Errorf("got validation fields %v, want %s", validationErr.Fields, tt.field)
				}
				if store.createCalled {
					t.Error("store was called for invalid thought")
				}
			})
		}
	})

	t.Run("delegates subject ID validity to the store", func(t *testing.T) {
		subjectID := int64(0)
		store := &thoughtServiceStoreStub{create: func(_ context.Context, item *data.Thought) (*data.Thought, error) {
			if item.SubjectID == nil || *item.SubjectID != subjectID {
				t.Fatalf("got subject ID %#v", item.SubjectID)
			}
			return nil, data.ErrRecordNotFound
		}}

		_, err := NewService(store).Create(context.Background(), "test-user", "learn Go", &subjectID, time.Time{})

		if !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got %v, want %v", err, data.ErrRecordNotFound)
		}
	})

	t.Run("returns store error", func(t *testing.T) {
		want := errors.New("boom")
		store := &thoughtServiceStoreStub{create: func(context.Context, *data.Thought) (*data.Thought, error) { return nil, want }}

		_, err := NewService(store).Create(context.Background(), "test-user", "learn Go", nil, time.Time{})

		if !errors.Is(err, want) {
			t.Fatalf("got %v, want %v", err, want)
		}
	})
}

func TestThoughtService_Get(t *testing.T) {
	t.Run("returns thought", func(t *testing.T) {
		want := &data.Thought{ThoughtID: 7, UserID: "test-user", Thought: "learn Go"}
		store := &thoughtServiceStoreStub{get: func(_ context.Context, userID string, thoughtID int64) (*data.Thought, error) {
			if userID != "test-user" || thoughtID != 7 {
				t.Fatalf("got user %q and thought ID %d", userID, thoughtID)
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
		store := &thoughtServiceStoreStub{get: func(context.Context, string, int64) (*data.Thought, error) { return nil, want }}

		_, err := NewService(store).Get(context.Background(), "test-user", 7)

		if !errors.Is(err, want) {
			t.Fatalf("got %v, want %v", err, want)
		}
	})
}
