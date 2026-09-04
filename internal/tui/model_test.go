package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
)

func TestModel_View(t *testing.T) {
	t.Run("renders local user and entity menu", func(t *testing.T) {
		handle := "local"
		model := NewModel(
			context.Background(),
			&data.User{UserID: "local-user-id", Handle: &handle},
			&subjectServiceStub{},
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
		service := &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) {
			return nil, nil
		}}
		model := newTestModel(service)

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
		service := &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) {
			return nil, nil
		}}
		model := openSubjects(t, newTestModel(service))

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
			model := newTestModel(&subjectServiceStub{})

			_, command := model.Update(tt.key)

			assertQuitCommand(t, command)
		})
	}
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
