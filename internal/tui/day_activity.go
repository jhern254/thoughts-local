package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

func (m Model) dayDivider() string {
	marker := min(max(0, m.days.Marker()), max(0, m.width-1))
	line := strings.Repeat("─", marker) + "┬" + strings.Repeat("─", max(0, m.width-marker-1))
	if m.homeFocus() == homePanelEvents || m.homeFocus() == homePanelDays {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Render(line)
	}
	return line
}
