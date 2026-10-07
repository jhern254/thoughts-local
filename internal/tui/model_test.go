package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
)

func TestModel_View(t *testing.T) {
	t.Run("renders Events heading and entity navigation", func(t *testing.T) {
		handle := "local"
		model := newScreenTestModel(
			context.Background(),
			&data.User{UserID: "local-user-id", Handle: &handle},
			&subjectServiceStub{},
			thought.NewService(testutils.NewFakeThoughtStore()), &metricsStub{},
			logging.Nop(),
		)

		view := model.View().Content
		if got := strings.Count(view, "\n") + 1; got > defaultHeight {
			t.Fatalf("got %d startup rows, want at most %d including panel divider", got, defaultHeight)
		}
		if !model.View().AltScreen {
			t.Fatal("home should use a fresh alternate screen")
		}

		if !strings.HasPrefix(view, "Local user: local (local-user-id)\n\n") {
			t.Fatalf("got header %q, want local user above Events", view)
		}
		for _, want := range []string{"Events", "Thoughts", "Subjects"} {
			if !strings.Contains(view, want) {
				t.Fatalf("view %q does not contain %q", view, want)
			}
		}
	})
}

func TestModel_Update(t *testing.T) {
	t.Run("selects Subjects entity", func(t *testing.T) {
		model := newRootTestModel()

		updated, command := model.Update(enterKey())
		got := updated.(Model)

		if got.screen != screenSubjectList {
			t.Fatalf("got screen %v, want subject list", got.screen)
		}
		if command == nil {
			t.Fatal("got nil command, want list-subjects command")
		}
	})

	t.Run("returns from Subjects to Events home", func(t *testing.T) {
		model := newRootTestModel()
		updated, _ := model.Update(enterKey())
		model = updated.(Model)

		updated, command := model.Update(escapeKey())
		got := updated.(Model)

		if command == nil {
			t.Fatal("got nil command, want home refresh")
		}
		if got.screen != screenEvents {
			t.Fatalf("got screen %v, want Events home", got.screen)
		}
	})
}

func newRootTestModel() Model {
	return newScreenTestModel(
		context.Background(),
		&data.User{UserID: "local-user-id"},
		&subjectServiceStub{},
		thought.NewService(testutils.NewFakeThoughtStore()), &metricsStub{},
		logging.Nop(),
	)
}

type metricsStub struct {
	total     int64
	totalErr  error
	miscCount int64
	miscErr   error
	counts    []data.SubjectThoughtCountView
	err       error
}

func (s *metricsStub) CountThoughts(context.Context, string) (int64, error) {
	return s.total, s.totalErr
}

func (s *metricsStub) CountUnassignedThoughts(context.Context, string) (int64, error) {
	return s.miscCount, s.miscErr
}

func (s *metricsStub) ThoughtCountsBySubject(context.Context, string) ([]data.SubjectThoughtCountView, error) {
	return s.counts, s.err
}

func assertQuitCommand(t *testing.T, command tea.Cmd) {
	t.Helper()
	if command == nil {
		t.Fatal("got nil command, want quit command")
	}
	message := command()
	if _, ok := message.(tea.QuitMsg); !ok {
		t.Fatalf("got command message %T, want tea.QuitMsg", message)
	}
}
