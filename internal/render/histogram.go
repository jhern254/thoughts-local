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
	Exponent  float64 // Positive power; invalid values use the histogram default, 3.0.
	Size      float64 // Dimension multiplier, default 1, bounded to [0.5, 1.25].
}

// A cubic default spreads similar lengths near the event maximum farther apart.
// This is separate from countExponent: collapsed event curves keep their 0.8
// default. Raise histogramExponent for more contrast; lower it for fuller bars.
const histogramExponent = 3.0

// HistogramExponent maps the single distribution boost control to the histogram's
// own default. Normal curve boost (0.8) gives cubic bars (3.0); increasing boost
// lowers both exponents, making small values fuller in both views. This shared
// mapping also keeps the preview command consistent with Events.
func HistogramExponent(curveExponent float64) float64 {
	if curveExponent <= 0 || math.IsNaN(curveExponent) || math.IsInf(curveExponent, 0) {
		return histogramExponent
	}
	return histogramExponent * curveExponent / countExponent
}

// RenderHistogram draws discrete leftward bars in the existing Braille language.
// It returns height rows of width cells; nonpositive dimensions return nil.
// CurveIndex identifies the input bar for caller-owned coloring, or -1 for blank or the muted connecting stem.
// Selection is not a drawing input and cannot change bar geometry.
//
// For full character count L and event maximum R:
//
//	ratio = min(max(L, 0) / R, 1)
//	maximumReach = min(lane dots - 1, (minAmplitude + amplitudeGain) * size)
//	reach = max(1, round(maximumReach * ratio^exponent))
//
// The default maximum is 16 dots, matching the curve's default peak. Lowering
// exponent boosts short thoughts; raising it emphasizes differences near the
// maximum. The histogram default is cubic (3.0); size changes reach, not height.
// Each positive bar spans all four dot rows of one cell and includes its right
// baseline. A one-dot-wide stem connects the first positive bar to the last,
// including clipped/offscreen bars, but never extends into headers or footers.
// Outline keeps only each rectangle's perimeter and the same stem. Zero/negative values
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
		exponent = histogramExponent
	}
	size := options.Size
	if size <= 0 || math.IsNaN(size) || math.IsInf(size, 0) {
		size = 1
	}
	size = min(maxSize, max(minSize, size))
	baseline := width*brailleColumns - 1
	maximum := min(float64(baseline), (minAmplitude+amplitudeGain)*size)
	// Find support before clipping so cropping a viewport cannot shorten the stem.
	first, last := height, -1
	for _, bar := range bars {
		if bar.Value > 0 {
			first = min(first, bar.Row)
			last = max(last, bar.Row)
		}
	}
	var stem rune = brailleBlank
	for dot := 0; dot < brailleRows; dot++ {
		stem |= brailleBit(baseline%brailleColumns, dot)
	}
	for row := max(0, first); row <= min(height-1, last); row++ {
		rows[row][width-1] = Cell{Glyph: stem, CurveIndex: -1}
	}
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
