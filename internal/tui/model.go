package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
)

type Model struct {
	user *data.User
}

func NewModel(user *data.User) Model {
	return Model{user: user}
}

func (Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m Model) View() tea.View {
	localUser := m.user.UserID
	if m.user.Handle != nil {
		localUser = fmt.Sprintf("%s (%s)", *m.user.Handle, m.user.UserID)
	}

	return tea.NewView(fmt.Sprintf("Thoughts\n\nLocal user: %s\n\nPress q to quit.\n", localUser))
}
