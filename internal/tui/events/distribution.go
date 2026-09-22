package events

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/jhern254/go-thoughts/internal/render"
)

const distributionWidth = 10

// These are session-local presentation settings, independent of day loads.
// Reset to default restores filled mode, normal boost (exponent 0.8), size 100%.
// Smaller exponents give low counts more presence without moving the maximum.
// Size scales the renderer's gains; the lane and card geometry stay fixed.
type distributionSettings struct {
	open  bool
	boost int
	size  int
	mode  render.Mode
}

func defaultDistributionSettings() distributionSettings {
	return distributionSettings{boost: 1, size: 100, mode: render.Filled}
}

func (s distributionSettings) boostValue() (string, float64) {
	levels := [...]struct {
		name     string
		exponent float64
	}{
		{"linear", 1}, {"normal", 0.8}, {"medium", 0.5}, {"strong", 0.25}, {"extra", 0.15},
	}
	level := levels[min(4, max(0, s.boost))]
	return level.name, level.exponent
}

func (m *Model) tuneDistributions(key string) {
	s := &m.distributions
	switch key {
	case "left", "h":
		s.boost = max(0, s.boost-1)
	case "right", "l":
		s.boost = min(4, s.boost+1)
	case "down", "j":
		s.size = max(50, s.size-5)
	case "up", "k":
		s.size = min(125, s.size+5)
	case "f":
		if s.mode == render.Filled {
			s.mode = render.Outline
		} else {
			s.mode = render.Filled
		}
	case "0":
		*s = defaultDistributionSettings()
		s.open = true
	case "esc", "enter", "d":
		s.open = false
	}
	m.load.retainedBody = ""
}

func (s distributionSettings) label() string {
	boost, _ := s.boostValue()
	mode := "filled"
	if s.mode == render.Outline {
		mode = "outline"
	}
	return fmt.Sprintf("Curves: %s boost · %d%% size · %s", boost, s.size, mode)
}

func (m *Model) cardColumn() int {
	if m.width < 60 {
		return 10 // Original time field and card width on narrow terminals.
	}
	return 24 // Time field (10), gutter (2), curve (10), gutter (2).
}

func (m *Model) cardWidth() int { return max(1, m.width-m.cardColumn()-2) }

// addDistributions draws the visible rows using the whole day's contributions.
// Translating every center by a whole number of cells preserves the fixed dot
// grid and global mixture gain. Neither selection nor scrolling changes scale.
func (m *Model) addDistributions(lines []string, curves []render.Distribution, selected, end, offset int) []string {
	if m.cardColumn() == 10 {
		return lines
	}
	var cells [][]render.Cell
	height := min(len(lines), max(0, end-offset)) // Never draw over the ending marker.
	if height > 0 && len(curves) > 0 {
		// Include expanded cards in the count reference even though their curves
		// are hidden, and retain offscreen inputs for normalization and overlap.
		var reference int64
		for _, count := range m.counts {
			reference = max(reference, count.Count)
		}
		shifted := make([]render.Distribution, len(curves))
		for i, curve := range curves {
			shifted[i] = curve
			shifted[i].CenterY -= float64(offset) * 4
		}
		_, exponent := m.distributions.boostValue()
		cells = render.RenderDistributions(distributionWidth, height, shifted, render.Options{
			Mode:           m.distributions.mode,
			CountReference: reference,
			CountExponent:  exponent,
			Size:           float64(m.distributions.size) / 100,
		})
	}
	styles := [...]lipgloss.Style{
		lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("62")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
	}
	for y, line := range lines {
		var lane strings.Builder
		lane.Grow(distributionWidth * 3)
		if y < len(cells) {
			var run strings.Builder
			activeStyle := -1
			flush := func() {
				if run.Len() > 0 {
					lane.WriteString(styles[activeStyle].Render(run.String()))
					run.Reset()
				}
			}
			for _, cell := range cells[y] {
				style := 0
				switch {
				case cell.Glyph == ' ':
					style = -1
				case cell.CurveIndex < 0:
					style = 2
				case cell.CurveIndex == selected && !m.blurred:
					style = 1
				}
				if style != activeStyle {
					flush()
					activeStyle = style
				}
				if style < 0 {
					lane.WriteByte(' ')
				} else {
					run.WriteRune(cell.Glyph)
				}
			}
			flush()
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
