package goals

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/jhern254/go-thoughts/internal/tui/appearance"
)

func (m Model) sectionDivider(focused bool) string {
	line := strings.Repeat(appearance.Border(m.borderWeight).Top, m.width)
	if focused {
		return lipgloss.NewStyle().Foreground(m.list.Styles.Title.GetBackground()).Render(line)
	}
	return line
}
