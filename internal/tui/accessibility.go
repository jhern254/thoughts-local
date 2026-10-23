package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) acceptAccessibility(msg BrowserOptionsMsg) (tea.Model, tea.Cmd) {
	if !m.options.open || msg.ID != m.options.id || m.options.request == "" {
		return m, nil
	}
	switch {
	case msg.Action == "loaded" && m.options.request == "load":
		m.options.thickerCardBorders, m.options.heavierText = msg.ThickerCardBorders, msg.HeavierText
		m.options.status = "Preview only. Apply to save."
	case msg.Action == "saved" && m.options.request == "applyAccessibility":
		m.events.SetThickerCardBorders(msg.ThickerCardBorders)
		m.options.open, m.options.accessibility = false, false
	case msg.Action == "failed":
		m.options.status = "Could not load or save accessibility settings. Try again."
	default:
		return m, nil
	}
	m.options.request = ""
	return m, nil
}

func (m *Model) activateAccessibility() {
	switch m.options.focus {
	case 0:
		m.options.thickerCardBorders = !m.options.thickerCardBorders
	case 1:
		m.options.heavierText = !m.options.heavierText
	case 2:
		m.options.status = "Saving…"
		m.requestOptions("applyAccessibility")
	case 3:
		m.closeAccessibility()
	}
}
func (m *Model) closeAccessibility() {
	m.options.accessibility = false
	m.options.menu = true
	m.requestOptions("")
}
func (m Model) updateAccessibilityInput(message tea.Msg) (tea.Model, tea.Cmd) {
	busy := m.options.request != ""
	switch key := message.(type) {
	case tea.KeyPressMsg:
		switch key.String() {
		case "q", "esc":
			if m.options.request != "applyAccessibility" {
				m.closeAccessibility()
			}
		case "down", "tab":
			m.options.focus = (m.options.focus + 1) % 4
		case "up", "shift+tab":
			m.options.focus = (m.options.focus + 3) % 4
		case "left", "right":
			if !busy && m.options.focus < 2 {
				m.activateAccessibility()
			}
		case "enter", "space":
			if !busy {
				m.activateAccessibility()
			}
		}
	case tea.MouseClickMsg:
		if !busy && key.Button == tea.MouseLeft && key.Y >= 2 && key.Y <= 5 {
			m.options.focus = key.Y - 2
			m.activateAccessibility()
		}
	}
	return m, nil
}
func (m Model) viewAccessibility() tea.View {
	onOff := func(enabled bool) string {
		if enabled {
			return "On"
		}
		return "Off"
	}
	labels := []string{"Thicker card borders: " + onOff(m.options.thickerCardBorders), "Heavier text: " + onOff(m.options.heavierText), "Apply", "Back"}
	rows := []string{m.subjects.list.Styles.Title.Render("Accessibility"), ""}
	for i, label := range labels {
		if i == m.options.focus {
			label = m.subjects.list.Styles.Title.Render(label)
		}
		rows = append(rows, label)
	}
	if m.height >= 16 {
		border := lipgloss.RoundedBorder()
		if m.options.thickerCardBorders {
			border = lipgloss.ThickBorder()
		}
		sample := lipgloss.NewStyle().Width(max(2, min(44, m.width-2))).Border(border).Render("Sample text: Aa Bb 123")
		rows = append(rows, "")
		rows = append(rows, strings.Split(sample, "\n")...)
	}
	rows = append(rows, "", m.options.status, "", "↑/↓/Tab: focus · ←/→: toggle", "Enter: activate · Q/Esc: cancel")
	for i, row := range rows {
		rows[i] = ansi.Truncate(row, max(1, m.width), "…")
	}
	view := tea.NewView(strings.Join(rows, "\n"))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}
