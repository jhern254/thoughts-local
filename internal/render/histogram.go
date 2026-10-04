package render

import "math"

// HistogramBar occupies one terminal row. Value is a nonnegative measurement,
// such as the full character count of a thought; Row may be outside the viewport.
type HistogramBar struct {
	Row   int
	Value int64
}

// HistogramOptions uses the same mode, boost and size conventions as curves.
// Reference is the whole event's maximum, including offscreen thoughts. It is
// deliberately not inferred from visible bars: scrolling must not change scale.
type HistogramOptions struct {
	Mode      Mode
	Reference int64
	Exponent  float64 // Positive power; invalid values use the curve default, 0.8.
	Size      float64 // Dimension multiplier, default 1, bounded to [0.5, 1.25].
}

// RenderHistogram draws discrete leftward bars in the existing Braille language.
// It returns height rows of width cells; nonpositive dimensions return nil.
// CurveIndex identifies the input bar for caller-owned coloring, or -1 for blank.
// Selection is not a drawing input and cannot change bar geometry.
//
// For full character count L and event maximum R:
//
//	ratio = min(max(L, 0) / R, 1)
//	maximumReach = min(lane dots - 1, (minAmplitude + amplitudeGain) * size)
//	reach = max(1, round(maximumReach * ratio^exponent))
//
// The default maximum is 16 dots, matching the curve's default peak. Lowering
// exponent boosts short thoughts; size changes maximum reach, never row height.
// Each positive bar spans all four dot rows of one cell and includes its right
// baseline. Outline keeps only that rectangle's perimeter. Zero/negative values
// and unknown references draw nothing, rather than inventing an aesthetic mound.
func RenderHistogram(width, height int, bars []HistogramBar, options HistogramOptions) [][]Cell {
	if width <= 0 || height <= 0 {
		return nil
	}
	rows := make([][]Cell, height)
	for y := range rows {
		rows[y] = make([]Cell, width)
		for x := range rows[y] {
			rows[y][x] = Cell{Glyph: ' ', CurveIndex: -1}
		}
	}
	if options.Reference <= 0 {
		return rows
	}
	exponent := options.Exponent
	if exponent <= 0 || math.IsNaN(exponent) || math.IsInf(exponent, 0) {
		exponent = countExponent
	}
	size := options.Size
	if size <= 0 || math.IsNaN(size) || math.IsInf(size, 0) {
		size = 1
	}
	size = min(maxSize, max(minSize, size))
	baseline := width*brailleColumns - 1
	maximum := min(float64(baseline), (minAmplitude+amplitudeGain)*size)
	for index, bar := range bars {
		if bar.Row < 0 || bar.Row >= height || bar.Value <= 0 {
			continue
		}
		ratio := min(1, float64(bar.Value)/float64(options.Reference))
		reach := min(baseline, max(1, int(math.Round(maximum*math.Pow(ratio, exponent)))))
		left := baseline - reach
		for x := left; x <= baseline; x++ {
			for y := 0; y < brailleRows; y++ {
				if options.Mode == Outline && x != left && x != baseline && y != 0 && y != brailleRows-1 {
					continue
				}
				cell := &rows[bar.Row][x/brailleColumns]
				if cell.Glyph == ' ' {
					cell.Glyph = brailleBlank
				}
				cell.Glyph |= brailleBit(x%brailleColumns, y)
				cell.CurveIndex = index
			}
		}
	}
	return rows
}
