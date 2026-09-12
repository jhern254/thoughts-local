package thought

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

type thoughtServiceStoreStub struct {
	browseView     func(context.Context, string, data.ThoughtViewRequest) (data.ThoughtView, error)
	listUnassigned func(context.Context, string) ([]data.Thought, error)
	list           func(context.Context, string, int64) ([]data.Thought, error)
	create         func(context.Context, *data.Thought) (*data.Thought, error)
	get            func(context.Context, string, int64) (*data.Thought, error)
	update         func(context.Context, string, int64, string, int64, time.Time) (*data.Thought, error)
	delete         func(context.Context, string, int64, int64) error
	createCalled   bool
}

func (s *thoughtServiceStoreStub) BrowseThoughtsView(ctx context.Context, userID string, request data.ThoughtViewRequest) (data.ThoughtView, error) {
	return s.browseView(ctx, userID, request)
}

func (s *thoughtServiceStoreStub) ListUnassignedThoughts(ctx context.Context, u string) ([]data.Thought, error) {
	return s.listUnassigned(ctx, u)
}

func TestThoughtService_ListUnassigned(t *testing.T) {
	t.Run("preserves scope results and original errors", func(t *testing.T) {
		ctx := context.Background()
		for _, wantErr := range []error{nil, errors.New("store failure")} {
			service := NewService(&thoughtServiceStoreStub{listUnassigned: func(got context.Context, u string) ([]data.Thought, error) {
				if got != ctx || u != "u" {
					t.Fatal("unassigned list lost scope")
				}
				return []data.Thought{{ThoughtID: 7}}, wantErr
			}})
			rows, err := service.ListUnassigned(ctx, "u")
			if err != wantErr || len(rows) != 1 || rows[0].ThoughtID != 7 {
				t.Fatalf("got %v, %v, want original rows and error", rows, err)
			}
		}
	})
}

func (s *thoughtServiceStoreStub) ListThoughts(ctx context.Context, u string, id int64) ([]data.Thought, error) {
	return s.list(ctx, u, id)
}

func TestThoughtService_List(t *testing.T) {
	t.Run("passes scope and preserves store failures", func(t *testing.T) {
		ctx := context.Background()
		wantErr := errors.New("store failure")
		service := NewService(&thoughtServiceStoreStub{list: func(got context.Context, u string, id int64) ([]data.Thought, error) {
			if got != ctx || u != "u" || id != 7 {
				t.Fatal("list lost scope")
			}
			return nil, wantErr
		}})
		if _, err := service.List(ctx, "u", 7); err != wantErr {
			t.Fatalf("got %v, want original error", err)
		}
	})
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

func (s *thoughtServiceStoreStub) UpdateThought(ctx context.Context, userID string, id int64, body string, version int64, updatedAt time.Time) (*data.Thought, error) {
	return s.update(ctx, userID, id, body, version, updatedAt)
}
func (s *thoughtServiceStoreStub) DeleteThought(ctx context.Context, userID string, id, version int64) error {
	return s.delete(ctx, userID, id, version)
}

func TestThoughtService_Update(t *testing.T) {
	t.Run("passes text and version unchanged with a UTC update time", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		before := time.Now().UTC()
		want := &data.Thought{ThoughtID: 7, Thought: "  PRIVATE-THOUGHT\nbody  ", Version: 4}
		store := &thoughtServiceStoreStub{update: func(gotCtx context.Context, user string, id int64, body string, version int64, updated time.Time) (*data.Thought, error) {
			if gotCtx != ctx || user != "owner" || id != 7 || body != want.Thought || version != 3 {
				t.Fatalf("got update arguments %v %q %d %q %d, want original context, owner, 7, unchanged body, 3", gotCtx, user, id, body, version)
			}
			if updated.Location() != time.UTC || updated.Before(before) || updated.After(time.Now()) {
				t.Errorf("got update time %v, want current UTC time", updated)
			}
			return want, nil
		}}
		got, err := NewService(store).Update(ctx, "owner", 7, want.Thought, 3)
		if got != want || err != nil {
			t.Fatalf("got result %v, error %v, want original result and nil", got, err)
		}
	})
	t.Run("accepts the schema Unicode limit without trimming text", func(t *testing.T) {
		body := strings.Repeat("界", maxThoughtCharacters)
		store := &thoughtServiceStoreStub{update: func(_ context.Context, _ string, _ int64, got string, _ int64, _ time.Time) (*data.Thought, error) {
			if got != body {
				t.Fatal("got changed body, want original Unicode content")
			}
			return &data.Thought{Thought: got}, nil
		}}
		if _, err := NewService(store).Update(context.Background(), "owner", 7, body, 1); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("rejects invalid input with public guidance before persistence", func(t *testing.T) {
		for _, tt := range []struct {
			name, user, body, field, message string
			version                          int64
		}{
			{"missing owner", "", "PRIVATE-INPUT", "user_id", "must be provided", 1},
			{"empty text", "owner", "   ", "thought", "must be provided", 1},
			{"oversized text", "owner", strings.Repeat("界", maxThoughtCharacters) + "PRIVATE-INPUT", "thought", "must not be more than 1000000 characters long", 1},
			{"zero version", "owner", "PRIVATE-INPUT", "version", "must be greater than zero", 0},
			{"negative version", "owner", "PRIVATE-INPUT", "version", "must be greater than zero", -1},
		} {
			t.Run(tt.name, func(t *testing.T) {
				store := &thoughtServiceStoreStub{update: func(context.Context, string, int64, string, int64, time.Time) (*data.Thought, error) {
					t.Fatal("store called for invalid input")
					return nil, nil
				}}
				_, err := NewService(store).Update(context.Background(), tt.user, 7, tt.body, tt.version)
				var validation *ValidationError
				if !errors.As(err, &validation) {
					t.Fatalf("got error %v, want ValidationError", err)
				}
				if got := validation.PublicFields()[tt.field]; got != tt.message {
					t.Errorf("got public guidance %q, want %q", got, tt.message)
				}
				if strings.Contains(fmt.Sprint(validation.PublicFields()), "PRIVATE-INPUT") {
					t.Fatal("got private input in public guidance")
				}
				public := validation.PublicFields()
				public[tt.field] = "changed"
				if got := validation.PublicFields()[tt.field]; got != tt.message {
					t.Errorf("got modified public guidance %q, want %q", got, tt.message)
				}
			})
		}
	})
	t.Run("preserves wrapped store errors", func(t *testing.T) {
		for _, cause := range []error{data.ErrRecordNotFound, data.ErrVersionConflict, data.ErrDatabaseBusy, data.ErrDatabaseReadOnly, context.Canceled, errors.New("unexpected")} {
			want := fmt.Errorf("PRIVATE-WRAPPER: %w", cause)
			store := &thoughtServiceStoreStub{update: func(context.Context, string, int64, string, int64, time.Time) (*data.Thought, error) {
				return nil, want
			}}
			_, err := NewService(store).Update(context.Background(), "owner", 7, "body", 1)
			if err != want || !errors.Is(err, cause) {
				t.Errorf("got error %v, want original wrapped error %v", err, want)
			}
		}
	})
}

func TestThoughtService_Delete(t *testing.T) {
	t.Run("passes scope and version and preserves store errors", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		for _, cause := range []error{nil, data.ErrRecordNotFound, data.ErrVersionConflict, data.ErrDatabaseBusy, data.ErrDatabaseReadOnly, context.Canceled, errors.New("PRIVATE-FAILURE")} {
			store := &thoughtServiceStoreStub{delete: func(got context.Context, user string, id, version int64) error {
				if got != ctx || user != "owner" || id != 7 || version != 3 {
					t.Fatalf("got delete arguments %v %q %d %d, want original context, owner, 7, 3", got, user, id, version)
				}
				return cause
			}}
			if err := NewService(store).Delete(ctx, "owner", 7, 3); err != cause {
				t.Errorf("got error %v, want original %v", err, cause)
			}
		}
	})
	t.Run("rejects missing owner and nonpositive versions before persistence", func(t *testing.T) {
		for _, tt := range []struct {
			name, user, field string
			version           int64
		}{{"missing owner", "", "user_id", 1}, {"zero version", "owner", "version", 0}, {"negative version", "owner", "version", -1}} {
			t.Run(tt.name, func(t *testing.T) {
				err := NewService(&thoughtServiceStoreStub{}).Delete(context.Background(), tt.user, 7, tt.version)
				var validation *ValidationError
				if !errors.As(err, &validation) {
					t.Fatalf("got error %v, want ValidationError", err)
				}
				if got := validation.PublicFields()[tt.field]; got == "" {
					t.Errorf("got empty guidance, want rule for %s", tt.field)
				}
			})
		}
	})
}
