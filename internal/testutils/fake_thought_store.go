package testutils

import (
	"context"
	"sort"

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

func (s *FakeThoughtStore) ListThoughts(ctx context.Context, userID string, subjectID int64) ([]data.Thought, error) {
	return s.list(ctx, userID, &subjectID)
}

func (s *FakeThoughtStore) ListUnassignedThoughts(ctx context.Context, userID string) ([]data.Thought, error) {
	return s.list(ctx, userID, nil)
}

func (s *FakeThoughtStore) list(ctx context.Context, userID string, subjectID *int64) ([]data.Thought, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows := []data.Thought{}
	for _, item := range s.thoughts {
		if item.UserID == userID && ((item.SubjectID == nil && subjectID == nil) || (item.SubjectID != nil && subjectID != nil && *item.SubjectID == *subjectID)) {
			rows = append(rows, item)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ObservedAt.Equal(rows[j].ObservedAt) {
			return rows[i].ThoughtID > rows[j].ThoughtID
		}
		return rows[i].ObservedAt.After(rows[j].ObservedAt)
	})
	return rows, nil
}
