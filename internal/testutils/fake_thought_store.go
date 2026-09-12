package testutils

import (
	"context"
	"slices"
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

func (s *FakeThoughtStore) CountThoughts(ctx context.Context, userID string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	var count int64
	for _, item := range s.thoughts {
		if item.UserID == userID {
			count++
		}
	}
	return count, nil
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

func (s *FakeThoughtStore) BrowseThoughtsView(ctx context.Context, userID string, request data.ThoughtViewRequest) (data.ThoughtView, error) {
	if err := ctx.Err(); err != nil {
		return data.ThoughtView{}, err
	}
	items := []data.ThoughtSummary{}
	for _, item := range s.thoughts {
		if item.UserID != userID {
			continue
		}
		preview := []rune(item.Thought)
		if len(preview) > 80 {
			preview = append(preview[:80], '…')
		}
		items = append(items, data.ThoughtSummary{ThoughtID: item.ThoughtID, Preview: string(preview), ObservedAt: item.ObservedAt, CreatedAt: item.CreatedAt})
	}
	compare := func(a, b data.ThoughtSummary) int {
		if c := b.ObservedAt.Compare(a.ObservedAt); c != 0 {
			return c
		}
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		if a.ThoughtID > b.ThoughtID {
			return -1
		}
		if a.ThoughtID < b.ThoughtID {
			return 1
		}
		return 0
	}
	slices.SortFunc(items, compare)
	if cursor := request.Cursor; cursor != nil {
		anchor := data.ThoughtSummary{ThoughtID: cursor.ThoughtID, ObservedAt: cursor.ObservedAt, CreatedAt: cursor.CreatedAt}
		items = slices.DeleteFunc(items, func(item data.ThoughtSummary) bool {
			if request.Direction == data.ThoughtsNewer {
				return compare(item, anchor) >= 0
			}
			return compare(item, anchor) <= 0
		})
	}
	if request.Direction == data.ThoughtsNewer {
		slices.Reverse(items)
	}
	more := len(items) > data.ThoughtViewBatchSize
	if more {
		items = items[:data.ThoughtViewBatchSize]
	}
	if request.Direction == data.ThoughtsNewer {
		slices.Reverse(items)
	}
	return data.ThoughtView{Items: items, More: more}, nil
}

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
