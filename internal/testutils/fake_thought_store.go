package testutils

import (
	"context"

	"github.com/jhern254/go-thoughts/internal/data"
)

type FakeThoughtStore struct {
	thoughts map[int64]data.Thought
	lastID   int64
}

func NewFakeThoughtStore() *FakeThoughtStore {
	return &FakeThoughtStore{thoughts: make(map[int64]data.Thought)}
}

func (s *FakeThoughtStore) CreateThought(ctx context.Context, thought *data.Thought) (*data.Thought, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.lastID++
	created := *thought
	created.ThoughtID = s.lastID
	s.thoughts[created.ThoughtID] = created
	return &created, nil
}

func (s *FakeThoughtStore) GetThought(ctx context.Context, userID string, thoughtID int64) (*data.Thought, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	thought, ok := s.thoughts[thoughtID]
	if !ok || thought.UserID != userID {
		return nil, data.ErrRecordNotFound
	}
	return &thought, nil
}

var _ data.ThoughtStore = (*FakeThoughtStore)(nil)
