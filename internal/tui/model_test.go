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
	t.Run("renders local user and entity menu", func(t *testing.T) {
		handle := "local"
		model := NewModel(
			context.Background(),
			&data.User{UserID: "local-user-id", Handle: &handle},
			&subjectServiceStub{},
			thought.NewService(testutils.NewFakeThoughtStore()), &metricsStub{},
			logging.Nop(),
		)

		view := model.View().Content

		for _, want := range []string{"Thoughts", "local-user-id", "local", "Subjects"} {
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

	t.Run("returns from Subjects to entity menu", func(t *testing.T) {
		model := newRootTestModel()
		updated, _ := model.Update(enterKey())
		model = updated.(Model)

		updated, command := model.Update(escapeKey())
		got := updated.(Model)

		if command != nil {
			t.Fatal("got command, want cached entity menu")
		}
		if got.screen != screenEntities {
			t.Fatalf("got screen %v, want entity menu", got.screen)
		}
	})

	for _, tt := range []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{name: "q quits", key: runeKey('q')},
		{name: "ctrl+c quits", key: tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl})},
	} {
		t.Run(tt.name, func(t *testing.T) {
			model := newRootTestModel()

			_, command := model.Update(tt.key)

			assertQuitCommand(t, command)
		})
	}
}

func TestModel_ThoughtQuitKeys(t *testing.T) {
	t.Run("q quits thought detail including during reload", func(t *testing.T) {
		m := rootOpenThoughts(t, filterRoot(t))
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
		m = runModelCommand(t, m, enterKey())
		_, cmd := rootUpdate(m, runeKey('q'))
		assertQuitCommand(t, cmd)
		if !strings.Contains(m.View().Content, "q: quit") {
			t.Fatal("thought detail help does not advertise q: quit")
		}
		m, _ = rootUpdate(m, runeKey('r'))
		_, cmd = rootUpdate(m, runeKey('q'))
		assertQuitCommand(t, cmd)
		_, cmd = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
		assertQuitCommand(t, cmd)
	})
	for _, state := range []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"editor", enterKey()},
		{"active filter", runeKey('/')},
	} {
		t.Run(state.name+" accepts q while Ctrl+C quits", func(t *testing.T) {
			m := rootOpenThoughts(t, filterRoot(t))
			m, _ = rootUpdate(m, state.key)
			m, cmd := rootUpdate(m, runeKey('q'))
			if cmd != nil {
				if _, quits := cmd().(tea.QuitMsg); quits {
					t.Fatal("q quit instead of reaching text input")
				}
			}
			if strings.Contains(m.View().Content, "q: quit") {
				t.Fatal("text input help incorrectly advertises q: quit")
			}
			_, cmd = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
			assertQuitCommand(t, cmd)
		})
	}
}

func newRootTestModel() Model {
	return NewModel(
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
	counts    []data.SubjectThoughtCount
	err       error
}

func (s *metricsStub) CountThoughts(context.Context, string) (int64, error) {
	return s.total, s.totalErr
}

func (s *metricsStub) CountUnassignedThoughts(context.Context, string) (int64, error) {
	return s.miscCount, s.miscErr
}

func (s *metricsStub) ThoughtCountsBySubject(context.Context, string) ([]data.SubjectThoughtCount, error) {
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
