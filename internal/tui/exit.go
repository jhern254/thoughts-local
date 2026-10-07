package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) canExit() bool {
	return m.screen == screenEvents && ((m.entityFocused && m.events.CanLeave()) || m.events.AtTopLevel())
}

func (m Model) updateExitPrompt(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "left", "right", "tab", "shift+tab":
		m.exitYes = !m.exitYes
	case "y":
		return m, tea.Quit
	case "enter":
		if m.exitYes {
			return m, tea.Quit
		}
		m.exitPromptOpen = false
	case "n", "q", "esc":
		m.exitPromptOpen = false
	}
	return m, nil
}

func (m Model) viewExitPrompt() string {
	yes, no := "  Yes", "  No"
	if m.exitYes {
		yes = m.subjects.list.Styles.Title.Render("> Yes")
	} else {
		no = m.subjects.list.Styles.Title.Render("> No")
	}
	rows := []string{
		"Exit app?",
		"",
		yes + "    " + no,
		"",
		"←/→ or Tab: select • Enter: accept",
		"Y: exit • N/Q/Esc: cancel",
	}
	for i := range rows {
		rows[i] = ansi.Truncate(rows[i], max(1, m.width), "…")
	}
	return strings.Join(rows, "\n")
}
