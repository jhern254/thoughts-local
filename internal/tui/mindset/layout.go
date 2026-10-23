package mindset

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

type textLine struct {
	item   int
	source int
	text   string
}

type panelLayout struct {
	lines        []textLine
	starts, ends []int
}

func (m *Model) reflow(anchor int) {
	layout := panelLayout{}
	for i, item := range m.items {
		layout.starts = append(layout.starts, len(layout.lines))
		text := readableText(item.Text)
		cursor := 0
		for _, line := range strings.Split(ansi.Wrap(text, m.textWidth(), ""), "\n") {
			// Locate the wrapped line in its source so resize can preserve the reading
			// position even when word wrapping changes the number of physical rows.
			position := strings.Index(text[cursor:], line)
			if position >= 0 {
				cursor += position
			}
			layout.lines = append(layout.lines, textLine{item: i, source: cursor, text: line})
			cursor += len(line)
			if cursor < len(text) && text[cursor] == '\n' {
				cursor++
			}
		}
		layout.ends = append(layout.ends, len(layout.lines))
	}
	m.layout = layout
	if len(m.items) == 0 {
		m.offset = 0
		return
	}
	m.offset = m.layout.starts[m.index]
	for row := m.layout.starts[m.index]; row < m.layout.ends[m.index]; row++ {
		if m.layout.lines[row].source > anchor {
			break
		}
		m.offset = row
	}
	m.keepVisible()
}

func readableText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\t", "    ")
	return strings.Map(func(r rune) rune {
		if r != '\n' && unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
}
