package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
)

func TestModel_View(t *testing.T) {
	t.Run("renders local user information", func(t *testing.T) {
		handle := "local"
		model := NewModel(&data.User{UserID: "local-user-id", Handle: &handle})

		view := model.View().Content

		for _, want := range []string{"Thoughts", "local-user-id", "local", "Press q to quit"} {
			if !strings.Contains(view, want) {
				t.Fatalf("view %q does not contain %q", view, want)
			}
		}
	})
}

func TestModel_Update(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{
			name: "q quits",
			key:  tea.KeyPressMsg(tea.Key{Code: 'q', Text: "q"}),
		},
		{
			name: "ctrl+c quits",
			key:  tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel(&data.User{UserID: "local-user-id"})

			_, command := model.Update(tt.key)

			if command == nil {
				t.Fatal("got nil command, want quit command")
			}
			message := command()
			if _, ok := message.(tea.QuitMsg); !ok {
				t.Fatalf("got command message %T, want tea.QuitMsg", message)
			}
		})
	}
}
