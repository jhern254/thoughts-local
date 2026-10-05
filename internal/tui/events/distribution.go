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
// Reset restores filled mode, size 100%, and normal boost: curves use exponent
// 0.8, while histograms use their separate cubic default via HistogramExponent.
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
		boostLinear: {"low", 1},
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
	return fmt.Sprintf("Distributions: %s boost · %d%% size · %s", boost, s.size, mode)
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
func (m *Model) addDistributions(lines []string, layout timelineLayout, offset int) []string {
	curves, selected, end := layout.curves, layout.selectedCurve, layout.curveEnd
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

	// Expanded cards reserve their lane rows, including blank header/metadata rows.
	// Paint only the visible intersection; the picker supplies its own scroll mapping.
	histogramTop, histogramEnd, histogramSelected := -1, -1, -1
	var histogram [][]render.Cell
	if top, ok := layout.positions[m.expanded]; ok && m.expanded != 0 {
		for _, card := range layout.cards {
			if card.top != top {
				continue
			}
			histogramTop = max(0, top-offset)
			histogramEnd = min(len(lines), top+len(card.content)+2-offset)
			if histogramTop >= histogramEnd || m.opening || m.err != nil {
				break
			}
			previews, reference := m.picker.EventThoughtBars()
			bars := make([]render.HistogramBar, 0, len(previews))
			for _, preview := range previews {
				// Border, heading and blank line precede the picker; its count row is
				// already included in preview.Row. All offsets are terminal rows.
				bars = append(bars, render.HistogramBar{Row: top + 3 + preview.Row - offset - histogramTop, Value: preview.Characters})
				if preview.Selected {
					histogramSelected = len(bars) - 1
				}
			}
			if len(bars) > 0 && reference > 0 {
				_, exponent := m.distributions.boostValue()
				histogram = render.RenderHistogram(distributionWidth, histogramEnd-histogramTop, bars, render.HistogramOptions{
					Mode:      m.distributions.mode,
					Reference: reference,
					Exponent:  render.HistogramExponent(exponent),
					Size:      float64(m.distributions.size) / 100,
				})
			}
			break
		}
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
		var rowCells []render.Cell
		rowSelected := selected
		if y >= histogramTop && y < histogramEnd {
			rowSelected = histogramSelected
			if y-histogramTop < len(histogram) {
				rowCells = histogram[y-histogramTop]
			}
		} else if y < len(cells) {
			rowCells = cells[y]
		}
		if len(rowCells) > 0 {
			var run strings.Builder
			activeStyle := plainStyle
			flush := func() {
				if run.Len() > 0 {
					lane.WriteString(styles[activeStyle].Render(run.String()))
					run.Reset()
				}
			}
			for _, cell := range rowCells {
				style := normalStyle
				switch {
				case cell.Glyph == ' ':
					style = plainStyle
				case cell.CurveIndex < 0:
					style = baselineStyle
				case cell.CurveIndex == rowSelected && !m.blurred:
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
