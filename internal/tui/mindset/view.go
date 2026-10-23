package mindset

import (
	"image/color"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) heading() string {
	heading := lipgloss.NewStyle().Background(color.White).Foreground(color.Black).Padding(0, 1).Render("Mindset")
	return ansi.Truncate(heading, max(0, m.width-3), "")
}

func (m Model) textWidth() int {
	// Reserve the picker rail and one cell inside the shared right boundary.
	return max(1, m.width-ansi.StringWidth(m.heading())-2)
}

func (m Model) View() string {
	heading := m.heading()
	styles := list.NewDefaultItemStyles(true)
	status := ""
	switch {
	case m.loading:
		status = "Loading mindsets…"
	case m.err != nil:
		status = "Could not load mindsets. r: retry"
	case len(m.items) == 0:
		if m.focused {
			return ansi.Truncate(heading+styles.SelectedTitle.PaddingLeft(0).Render("..."), m.width, "")
		}
		status = "..."
	}
	if status != "" {
		return ansi.Truncate(heading+" "+status, m.width, "…")
	}
	first := m.offset
	if !m.focused {
		first = m.layout.starts[m.index]
	}
	last := first + m.Height()
	rows := make([]string, 0, last-first)
	indent := strings.Repeat(" ", ansi.StringWidth(heading))
	for row := first; row < last; row++ {
		line := m.layout.lines[row]
		prefix := indent
		if row == first {
			prefix = heading
		}
		text := " " + line.text
		if m.focused {
			style := styles.NormalTitle.PaddingLeft(1)
			if line.item == m.index {
				style = styles.SelectedTitle.PaddingLeft(0)
			}
			text = style.Render(line.text)
		}
		rows = append(rows, ansi.Truncate(prefix+text, max(1, m.width-1), ""))
	}
	return strings.Join(rows, "\n")
}
