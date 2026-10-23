package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/tui/appearance"
)

const accessibilityControlCount = 6

func (m *Model) setBorderWeight(weight data.BorderWeight) {
	if !weight.Valid() {
		weight = data.BorderNormal
	}
	m.borderWeight = weight
	m.events.SetBorderWeight(weight)
}
func (m Model) acceptAccessibility(msg BrowserOptionsMsg) (tea.Model, tea.Cmd) {
	if !m.options.open || msg.ID != m.options.id || m.options.request == "" {
		return m, nil
	}
	switch {
	case msg.Action == "loaded" && m.options.request == "load":
		m.options.borderWeight = msg.BorderWeight
		if !m.options.borderWeight.Valid() {
			m.options.borderWeight = data.BorderNormal
		}
		m.options.textSize = msg.TextSize
		if !m.options.textSize.Valid() {
			m.options.textSize = data.TextStandard
		}
		m.options.heavierText, m.options.highContrast = msg.HeavierText, msg.HighContrast
		m.options.status = "Preview only. Apply to save."
	case msg.Action == "saved" && m.options.request == "applyAccessibility":
		m.setBorderWeight(msg.BorderWeight)
		m.options.open, m.options.accessibility = false, false
	case msg.Action == "failed":
		m.options.status = "Could not load or save accessibility settings. Try again."
	default:
		return m, nil
	}
	m.options.request = ""
	return m, nil
}
func (m *Model) adjustAccessibility(direction int) {
	switch m.options.focus {
	case 0:
		m.options.borderWeight = m.options.borderWeight.Step(direction)
	case 1:
		m.options.heavierText = !m.options.heavierText
	case 2:
		m.options.textSize = m.options.textSize.Step(direction)
	case 3:
		m.options.highContrast = !m.options.highContrast
	}
}
func (m *Model) activateAccessibility() {
	switch m.options.focus {
	case 4:
		m.options.status = "Saving…"
		m.requestOptions("applyAccessibility")
	case 5:
		m.closeAccessibility()
	default:
		m.adjustAccessibility(1)
	}
}
func (m *Model) closeAccessibility() {
	m.options.accessibility = false
	m.options.menu = true
	m.requestOptions("")
}
func (m Model) accessibilityVisibleControls() int {
	return max(1, min(accessibilityControlCount, m.height-6))
}
func (m *Model) revealAccessibilityControl() {
	visible := m.accessibilityVisibleControls()
	if m.options.focus < m.options.accessibilityOffset {
		m.options.accessibilityOffset = m.options.focus
	}
	if m.options.focus >= m.options.accessibilityOffset+visible {
		m.options.accessibilityOffset = m.options.focus - visible + 1
	}
	m.options.accessibilityOffset = min(m.options.accessibilityOffset, accessibilityControlCount-visible)
}
func (m Model) updateAccessibilityInput(message tea.Msg) (tea.Model, tea.Cmd) {
	m.revealAccessibilityControl()
	busy := m.options.request != ""
	switch key := message.(type) {
	case tea.KeyPressMsg:
		switch key.String() {
		case "q", "esc":
			if m.options.request != "applyAccessibility" {
				m.closeAccessibility()
			}
		case "down", "tab":
			m.options.focus = (m.options.focus + 1) % accessibilityControlCount
		case "up", "shift+tab":
			m.options.focus = (m.options.focus + accessibilityControlCount - 1) % accessibilityControlCount
		case "left", "right":
			if !busy && m.options.focus < 4 {
				direction := 1
				if key.String() == "left" {
					direction = -1
				}
				m.adjustAccessibility(direction)
			}
		case "enter", "space":
			if !busy {
				m.activateAccessibility()
			}
		}
	case tea.MouseClickMsg:
		if !busy && key.Button == tea.MouseLeft && key.Y >= 2 && key.Y < 2+m.accessibilityVisibleControls() {
			m.options.focus = key.Y - 2 + m.options.accessibilityOffset
			m.activateAccessibility()
		}
	}
	m.revealAccessibilityControl()
	return m, nil
}
func (m Model) viewAccessibility() tea.View {
	m.revealAccessibilityControl()
	onOff := func(enabled bool) string {
		if enabled {
			return "On"
		}
		return "Off"
	}
	labels := []string{"Border weight: " + m.options.borderWeight.Label(), "Heavier text: " + onOff(m.options.heavierText), "Text size: " + m.options.textSize.Label(), "High contrast: " + onOff(m.options.highContrast), "Apply", "Back"}
	rows := []string{m.subjects.list.Styles.Title.Render("Accessibility"), ""}
	for i := m.options.accessibilityOffset; i < min(len(labels), m.options.accessibilityOffset+m.accessibilityVisibleControls()); i++ {
		label := labels[i]
		if i == m.options.focus {
			label = m.subjects.list.Styles.Title.Render(label)
		}
		rows = append(rows, label)
	}
	if m.height >= 20 {
		sample := lipgloss.NewStyle().Width(max(2, min(44, m.width-2))).Border(appearance.Border(m.options.borderWeight)).Render("Sample text: Aa Bb 123")
		rows = append(rows, "")
		rows = append(rows, strings.Split(sample, "\n")...)
	}
	rows = append(rows, "", m.options.status, "↑/↓/Tab: focus · ←/→: adjust", "Enter: activate · Q/Esc: cancel")
	for i, row := range rows {
		rows[i] = ansi.Truncate(row, max(1, m.width), "…")
	}
	rows = rows[:min(len(rows), max(1, m.height))]
	view := tea.NewView(strings.Join(rows, "\n"))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}
