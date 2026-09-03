package user

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jhern254/go-thoughts/internal/data"
)

type userServiceStoreStub struct {
	create       func(context.Context, *data.User) (*data.User, error)
	getByHandle  func(context.Context, string) (*data.User, error)
	createCalled bool
}

func (s *userServiceStoreStub) CreateUser(ctx context.Context, user *data.User) (*data.User, error) {
	s.createCalled = true
	return s.create(ctx, user)
}

func (s *userServiceStoreStub) GetUserByHandle(ctx context.Context, handle string) (*data.User, error) {
	return s.getByHandle(ctx, handle)
}

func TestUserService_EnsureLocalUser(t *testing.T) {
	t.Run("returns existing local user", func(t *testing.T) {
		type contextKey string
		ctx := context.WithValue(context.Background(), contextKey("request"), "test-value")
		handle := localUserHandle
		want := &data.User{UserID: "existing-id", Handle: &handle, Version: 3}
		store := &userServiceStoreStub{
			getByHandle: func(gotCtx context.Context, gotHandle string) (*data.User, error) {
				if gotCtx.Value(contextKey("request")) != "test-value" || gotHandle != localUserHandle {
					t.Fatalf("got context %v and handle %q", gotCtx, gotHandle)
				}
				return want, nil
			},
		}

		got, err := NewService(store).EnsureLocalUser(ctx)

		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("got user %#v, want %#v", got, want)
		}
		if store.createCalled {
			t.Fatal("created user when local user already existed")
		}
	})

	t.Run("creates local user when missing", func(t *testing.T) {
		type contextKey string
		ctx := context.WithValue(context.Background(), contextKey("request"), "test-value")
		store := &userServiceStoreStub{
			getByHandle: func(gotCtx context.Context, handle string) (*data.User, error) {
				if gotCtx.Value(contextKey("request")) != "test-value" || handle != localUserHandle {
					t.Fatalf("got context %v and handle %q", gotCtx, handle)
				}
				return nil, data.ErrRecordNotFound
			},
			create: func(gotCtx context.Context, user *data.User) (*data.User, error) {
				if gotCtx.Value(contextKey("request")) != "test-value" {
					t.Fatalf("got context %v", gotCtx)
				}
				if _, err := uuid.Parse(user.UserID); err != nil {
					t.Fatalf("got user ID %q: %v", user.UserID, err)
				}
				if user.Handle == nil || *user.Handle != localUserHandle {
					t.Fatalf("got handle %#v", user.Handle)
				}
				if user.Version != 1 || user.CreatedAt.IsZero() || user.UpdatedAt.IsZero() {
					t.Fatalf("got user %#v", user)
				}
				return user, nil
			},
		}

		created, err := NewService(store).EnsureLocalUser(ctx)

		if err != nil {
			t.Fatal(err)
		}
		if created == nil || !store.createCalled {
			t.Fatalf("got created user %#v and create called %t", created, store.createCalled)
		}
	})

	t.Run("returns store lookup error", func(t *testing.T) {
		want := errors.New("lookup failed")
		store := &userServiceStoreStub{
			getByHandle: func(context.Context, string) (*data.User, error) {
				return nil, want
			},
		}

		_, err := NewService(store).EnsureLocalUser(context.Background())

		if err != want {
			t.Fatalf("got error %v, want %v", err, want)
		}
		if store.createCalled {
			t.Fatal("created user after lookup error")
		}
	})

	t.Run("returns store create error", func(t *testing.T) {
		want := errors.New("create failed")
		store := &userServiceStoreStub{
			getByHandle: func(context.Context, string) (*data.User, error) {
				return nil, data.ErrRecordNotFound
			},
			create: func(context.Context, *data.User) (*data.User, error) {
				return nil, want
			},
		}

		_, err := NewService(store).EnsureLocalUser(context.Background())

		if err != want {
			t.Fatalf("got error %v, want %v", err, want)
		}
	})
}
