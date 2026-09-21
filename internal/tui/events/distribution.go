package events

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/jhern254/go-thoughts/internal/render"
)

// Change only this setting to use the outline renderer. Geometry and navigation
// are independent of the mode; no user preference or additional read is needed.
const distributionMode = render.Filled

const distributionWidth = 10

func (m Model) cardColumn() int {
	if m.width < 60 {
		return 10 // Original time field and card width on narrow terminals.
	}
	return 24 // Time field (10), gutter (2), curve (10), gutter (2).
}

func (m Model) cardWidth() int { return max(1, m.width-m.cardColumn()-2) }

// addDistributions composes the entire day before the viewport crops it. All
// eligible cards, including offscreen cards, participate in the same mixture.
// Rows and card borders are already final; the lane never allocates extra rows.
func (m Model) addDistributions(lines []string, curves []render.Distribution, selected, end int) []string {
	if m.cardColumn() == 10 {
		return lines
	}
	cells := render.RenderDistributions(distributionWidth, end, curves, render.Options{Mode: distributionMode})
	normal := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	highlight := lipgloss.NewStyle().Foreground(lipgloss.Color("62"))
	baseline := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	for y, line := range lines {
		var lane strings.Builder
		if y < len(cells) {
			for _, cell := range cells[y] {
				if cell.Glyph == ' ' {
					lane.WriteByte(' ')
					continue
				}
				style := normal
				if cell.CurveIndex < 0 {
					style = baseline
				} else if cell.CurveIndex == selected && !m.blurred {
					style = highlight
				}
				lane.WriteString(style.Render(string(cell.Glyph)))
			}
		} else {
			lane.WriteString(strings.Repeat(" ", distributionWidth))
		}
		// The existing prefix is ten ASCII timestamp cells, or an empty separator.
		split := min(10, len(line))
		label, content := line[:split], line[split:]
		lines[y] = label + strings.Repeat(" ", 10-split) + "  " + lane.String() + "  " + content
	}
	return lines
}
