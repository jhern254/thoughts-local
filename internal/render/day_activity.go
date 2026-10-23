package render

import "math"

// DayActivity contains average daily counts for one calendar bucket. It is not
// a probability density: interpolation connects samples without adding peaks.
type DayActivity struct{ Events, Thoughts float64 }

// DayActivityDefaultExponent deepens quiet valleys while retaining the measured
// ordering of the samples; it does not add decorative peaks or smoothing.
const DayActivityDefaultExponent = 1.1

type DayActivityLevel struct{ Total, Events, Thoughts float64 }
type DayActivitySeries uint8

const (
	DayTotal DayActivitySeries = iota
	DayEvents
	DayThoughts
)

type DayActivityCell struct {
	Glyph  rune
	Series DayActivitySeries
}
type DayActivityOptions struct {
	Total, Events, Thoughts bool
	Outline                 bool
	Exponent, Size          float64
}

// Each channel uses its visible-range maximum before weighting. These fixed
// weights keep prolific Thought writing from overwhelming Event activity.
// Boost acts before weighting, so Total remains the sum of its components.
func DayActivityLevels(values []DayActivity, exponent float64) []DayActivityLevel {
	if exponent <= 0 || math.IsNaN(exponent) || math.IsInf(exponent, 0) {
		exponent = DayActivityDefaultExponent
	}
	var maxEvents, maxThoughts float64
	for _, value := range values {
		maxEvents = max(maxEvents, finiteCount(value.Events))
		maxThoughts = max(maxThoughts, finiteCount(value.Thoughts))
	}
	levels := make([]DayActivityLevel, len(values))
	for i, value := range values {
		if maxEvents > 0 {
			levels[i].Events = .7 * math.Pow(finiteCount(value.Events)/maxEvents, exponent)
		}
		if maxThoughts > 0 {
			levels[i].Thoughts = .3 * math.Pow(finiteCount(value.Thoughts)/maxThoughts, exponent)
		}
		levels[i].Total = levels[i].Events + levels[i].Thoughts
	}
	return levels
}
func finiteCount(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0
	}
	return value
}

// RenderDayActivity uses a shared scale even when series are hidden. Total is
// the background fill; component traces win cell color, with stronger weighted
// activity taking priority and Events breaking ties. A Braille cell can carry
// only one foreground color, although its dots may belong to several series.
func RenderDayActivity(width, height int, values []DayActivity, options DayActivityOptions) [][]DayActivityCell {
	if width <= 0 || height <= 0 {
		return nil
	}
	rows := make([][]DayActivityCell, height)
	strength := make([][]float64, height)
	for y := range rows {
		rows[y] = make([]DayActivityCell, width)
		strength[y] = make([]float64, width)
		for x := range rows[y] {
			rows[y][x].Glyph = ' '
		}
	}
	if len(values) == 0 {
		return rows
	}
	levels := DayActivityLevels(values, options.Exponent)
	size := options.Size
	if size <= 0 || math.IsNaN(size) || math.IsInf(size, 0) {
		size = 1
	}
	size = min(3, max(.5, size))
	// Amplitude = 1-exp(-1.6*size): about 80% of the canvas at 100%,
	// approaching 99% at 300%. Larger sizes keep sharp summits instead of
	// clipping them into a flat ridge. The presentation also adds plot rows.
	maximum := float64(height*brailleRows-1) * (1 - math.Exp(-1.6*size))
	put := func(x, y int, series DayActivitySeries, value float64) {
		cell := &rows[y/brailleRows][x/brailleColumns]
		if cell.Glyph == ' ' {
			cell.Glyph = brailleBlank
		}
		cell.Glyph |= brailleBit(x%brailleColumns, y%brailleRows)
		if series != DayTotal && value > strength[y/brailleRows][x/brailleColumns] {
			cell.Series = series
			strength[y/brailleRows][x/brailleColumns] = value
		}
	}
	for series, visible := range []bool{options.Total, options.Events, options.Thoughts} {
		if !visible {
			continue
		}
		previous := -1
		for x := 0; x < width*brailleColumns; x++ {
			position := max(0, min(float64(len(levels)-1), (float64(x)+.5)/float64(width*brailleColumns)*float64(len(levels))-.5))
			index := int(position)
			a, b := levels[index], levels[min(index+1, len(levels)-1)]
			left := [...]float64{a.Total, a.Events, a.Thoughts}[series]
			right := [...]float64{b.Total, b.Events, b.Thoughts}[series]
			value := left + (right-left)*(position-float64(index))
			if value <= 0 {
				previous = -1
				continue
			}
			top := height*brailleRows - 1 - int(math.Round(value*maximum))
			bottom := top
			if series == 0 && !options.Outline {
				bottom = height*brailleRows - 1
			} else if previous >= 0 {
				bottom = max(top, previous)
				top = min(top, previous)
			}
			for y := top; y <= bottom; y++ {
				put(x, y, DayActivitySeries(series), value)
			}
			previous = height*brailleRows - 1 - int(math.Round(value*maximum))
		}
	}
	return rows
}
