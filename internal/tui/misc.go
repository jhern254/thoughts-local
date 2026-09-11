package tui

import tea "charm.land/bubbletea/v2"

func (m Model) updateMiscThoughts(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyPressMsg); ok && m.thoughts.Browsing() {
		switch key.String() {
		case "q":
			return m, tea.Quit
		case "esc":
			m.thoughts.Reset()
			m.screen = screenSubjectList
			if m.subjects.listStale {
				return m.openSubjects()
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.thoughts, cmd = m.thoughts.Update(message)
	return m, cmd
}
