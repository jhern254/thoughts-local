package events

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/jhern254/go-thoughts/internal/render"
)

const (
	// Layout is measured in terminal cells, not bytes or Braille dots.
	timelineLabelPadding = "          " // Eight-character timestamp plus two spaces.
	timelineLabelWidth   = len(timelineLabelPadding)
	distributionWidth    = 10
	distributionGutter   = "  "
	distributionMinWidth = 60   // Below this panel width, return the lane to the cards.
	cardBorderColumns    = 2    // Left and right borders.
	selectedEventColor   = "62" // Xterm 256-color purple, shared by card and curve.

	// Percent controls mirror the renderer's supported Size range [0.5, 1.25].
	distributionMinSize     = 50
	distributionMaxSize     = 125
	distributionDefaultSize = 100
	distributionSizeStep    = 5
)

// Preset order is the left/right control order; normal matches the renderer default.
const (
	boostLinear = iota
	boostNormal
	boostMedium
	boostStrong
	boostExtra
	boostCount
)

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
	return distributionSettings{boost: boostNormal, size: distributionDefaultSize, mode: render.Filled}
}

func (s distributionSettings) boostValue() (string, float64) {
	levels := [...]struct {
		name     string
		exponent float64
	}{
		boostLinear: {"linear", 1},
		boostNormal: {"normal", 0.8},
		boostMedium: {"medium", 0.5},
		boostStrong: {"strong", 0.25},
		boostExtra:  {"extra", 0.15},
	}
	level := levels[min(len(levels)-1, max(boostLinear, s.boost))]
	return level.name, level.exponent
}

func (m *Model) tuneDistributions(key string) {
	s := &m.distributions
	switch key {
	case "left", "h":
		s.boost = max(boostLinear, s.boost-1)
	case "right", "l":
		s.boost = min(boostCount-1, s.boost+1)
	case "down", "j":
		s.size = max(distributionMinSize, s.size-distributionSizeStep)
	case "up", "k":
		s.size = min(distributionMaxSize, s.size+distributionSizeStep)
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
	if m.width < distributionMinWidth {
		return timelineLabelWidth
	}
	return timelineLabelWidth + len(distributionGutter) + distributionWidth + len(distributionGutter)
}

func (m *Model) cardWidth() int { return max(1, m.width-m.cardColumn()-cardBorderColumns) }

// addDistributions draws the visible rows using the whole day's contributions.
// Translating every center by a whole number of cells preserves the fixed dot
// grid and global mixture gain. Neither selection nor scrolling changes scale.
func (m *Model) addDistributions(lines []string, curves []render.Distribution, selected, end, offset int) []string {
	if m.cardColumn() == timelineLabelWidth {
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
			shifted[i].CenterY -= float64(offset) * 4 // Four Braille dot rows per terminal cell.
		}
		_, exponent := m.distributions.boostValue()
		cells = render.RenderDistributions(distributionWidth, height, shifted, render.Options{
			Mode:           m.distributions.mode,
			CountReference: reference,
			CountExponent:  exponent,
			Size:           float64(m.distributions.size) / 100,
		})
	}
	// Indexes identify style runs, not curve ownership or palette numbers.
	const (
		normalStyle = iota
		selectedStyle
		baselineStyle
		plainStyle = -1
	)
	styles := [...]lipgloss.Style{
		normalStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("245")), // Muted gray.
		selectedStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(selectedEventColor)),
		baselineStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("240")), // Darker connecting rail.
	}
	for y, line := range lines {
		var lane strings.Builder
		lane.Grow(distributionWidth * 3) // A Braille rune occupies three UTF-8 bytes.
		if y < len(cells) {
			var run strings.Builder
			activeStyle := plainStyle
			flush := func() {
				if run.Len() > 0 {
					lane.WriteString(styles[activeStyle].Render(run.String()))
					run.Reset()
				}
			}
			for _, cell := range cells[y] {
				style := normalStyle
				switch {
				case cell.Glyph == ' ':
					style = plainStyle
				case cell.CurveIndex < 0:
					style = baselineStyle
				case cell.CurveIndex == selected && !m.blurred:
					style = selectedStyle
				}
				if style != activeStyle {
					flush()
					activeStyle = style
				}
				if style == plainStyle {
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
		split := min(timelineLabelWidth, len(line))
		label, content := line[:split], line[split:]
		lines[y] = label + strings.Repeat(" ", timelineLabelWidth-split) + distributionGutter + lane.String() + distributionGutter + content
	}
	return lines
}
