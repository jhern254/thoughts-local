package goalassignment

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/goal"
)

type goalPreparerStub struct {
	item   *data.Goal
	err    error
	userID string
	input  goal.CreateGoalInput
}

func (s *goalPreparerStub) PrepareCreate(_ context.Context, userID string, input goal.CreateGoalInput) (*data.Goal, error) {
	s.userID = userID
	s.input = input
	return s.item, s.err
}

type assignmentStoreSpy struct {
	item            *data.Goal
	err             error
	calls           int
	userID          string
	goalID, version int64
	subjectIDs      []int64
}

func (s *assignmentStoreSpy) CreateGoalWithSubjects(_ context.Context, item *data.Goal, ids []int64) (*data.Goal, error) {
	s.calls++
	s.item = item
	s.subjectIDs = ids
	return item, s.err
}
func (s *assignmentStoreSpy) ReplaceGoalSubjects(_ context.Context, userID string, goalID, version int64, ids []int64) (*data.Goal, error) {
	s.calls++
	s.userID = userID
	s.goalID = goalID
	s.version = version
	s.subjectIDs = ids
	return s.item, s.err
}

func TestService_Create(t *testing.T) {
	t.Run("persists the prepared goal and requested selection together", func(t *testing.T) {
		prepared := &data.Goal{UserID: "owner", GoalName: "normalized", Version: 1}
		goals := &goalPreparerStub{item: prepared}
		store := &assignmentStoreSpy{}
		input := goal.CreateGoalInput{Name: " normalized ", TargetSeconds: 60}
		ids := []int64{2, 1}
		got, err := NewService(goals, store).Create(t.Context(), "owner", input, ids)
		if err != nil || got != prepared || store.calls != 1 || !reflect.DeepEqual(store.subjectIDs, ids) || goals.input != input || goals.userID != "owner" {
			t.Fatalf("create: got %+v, %v with store %+v, want prepared Goal and original selection", got, err, store)
		}
	})
	t.Run("does not persist an invalid goal", func(t *testing.T) {
		want := errors.New("invalid goal")
		store := &assignmentStoreSpy{}
		_, err := NewService(&goalPreparerStub{err: want}, store).Create(t.Context(), "owner", goal.CreateGoalInput{}, nil)
		if !errors.Is(err, want) || store.calls != 0 {
			t.Fatalf("create: got %v and %d writes, want validation error and zero writes", err, store.calls)
		}
	})
	t.Run("rejects duplicate IDs before persistence", func(t *testing.T) {
		store := &assignmentStoreSpy{}
		_, err := NewService(&goalPreparerStub{item: &data.Goal{}}, store).Create(t.Context(), "owner", goal.CreateGoalInput{}, []int64{1, 1})
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.PublicFields()["subject_ids"] == "" || store.calls != 0 {
			t.Fatalf("duplicate selection: got %v and %d writes, want validation and no writes", err, store.calls)
		}
	})
	t.Run("preserves transaction errors", func(t *testing.T) {
		want := errors.New("transaction failed")
		store := &assignmentStoreSpy{err: want}
		_, err := NewService(&goalPreparerStub{item: &data.Goal{}}, store).Create(t.Context(), "owner", goal.CreateGoalInput{}, nil)
		if !errors.Is(err, want) {
			t.Fatalf("create error: got %v, want %v", err, want)
		}
	})
}

func TestService_ReplaceSubjects(t *testing.T) {
	for _, tc := range []struct {
		name, user string
		version    int64
		ids        []int64
	}{
		{"missing user", "", 1, nil}, {"invalid version", "owner", 0, nil}, {"duplicate subjects", "owner", 1, []int64{2, 2}},
	} {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			store := &assignmentStoreSpy{}
			_, err := NewService(&goalPreparerStub{}, store).ReplaceSubjects(t.Context(), tc.user, 7, tc.version, tc.ids)
			var validation *ValidationError
			if !errors.As(err, &validation) || store.calls != 0 {
				t.Fatalf("replace: got %v and %d writes, want validation and no writes", err, store.calls)
			}
		})
	}
	t.Run("passes ownership version and empty selection to the atomic store", func(t *testing.T) {
		want := &data.Goal{GoalID: 7, Version: 4}
		store := &assignmentStoreSpy{item: want}
		got, err := NewService(&goalPreparerStub{}, store).ReplaceSubjects(t.Context(), "owner", 7, 3, nil)
		if err != nil || got != want || store.userID != "owner" || store.goalID != 7 || store.version != 3 || store.subjectIDs != nil {
			t.Fatalf("replace: got %+v, %v and store %+v, want unchanged input and stored Goal", got, err, store)
		}
	})
	t.Run("preserves version conflict errors", func(t *testing.T) {
		store := &assignmentStoreSpy{err: data.ErrVersionConflict}
		_, err := NewService(&goalPreparerStub{}, store).ReplaceSubjects(t.Context(), "owner", 7, 3, nil)
		if !errors.Is(err, data.ErrVersionConflict) {
			t.Fatalf("replace error: got %v, want version conflict", err)
		}
	})
}
