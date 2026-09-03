package user

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jhern254/go-thoughts/internal/data"
)

const localUserHandle = "local"

type Store interface {
	CreateUser(ctx context.Context, user *data.User) (*data.User, error)
	GetUserByHandle(ctx context.Context, handle string) (*data.User, error)
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) EnsureLocalUser(ctx context.Context) (*data.User, error) {
	existing, err := s.store.GetUserByHandle(ctx, localUserHandle)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, data.ErrRecordNotFound) {
		return nil, err
	}

	now := time.Now().UTC()
	handle := localUserHandle
	return s.store.CreateUser(ctx, &data.User{
		UserID:    uuid.NewString(),
		Handle:    &handle,
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	})
}
