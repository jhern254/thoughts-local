package render

import (
	"math"
	"math/bits"
)

// DayActivity contains average daily counts for one calendar bucket. It is not
// a probability density: interpolation connects samples without adding peaks.
type DayActivity struct{ Events, Thoughts float64 }

// DayActivityDefaultExponent retains quieter slopes while preserving the measured
// ordering of the samples; it does not add decorative peaks or smoothing.
const DayActivityDefaultExponent = .8

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
	// FirstDay is a civil-day index from a fixed epoch; Days is the visible
	// calendar span (zero uses one day per sample). These anchor panning.
	FirstDay, Days int
	// Samples may cover whole, calendar-aligned buckets beyond the visible range.
	// Zero SampleDays keeps the evenly spaced standalone-preview convention.
	SampleFirstDay, SampleDays int
	// Reference is the weighted historical peak at this bucket resolution. Zero
	// derives it from the supplied samples; panning callers supply a stable peak.
	Reference float64
	// Adjacent buckets continue slopes beyond the viewport. Nil keeps the
	// endpoint flat when a caller has no neighboring data.
	Before, After           *DayActivity
	Total, Events, Thoughts bool
	Outline                 bool
	Exponent                float64
	// Size is relative to normal (1 = 100%), bounded to [0.5, 3].
	Size float64
}

const (
	dayEventWeight   = .7
	dayThoughtWeight = .3
	// Preserve the former 300% fill at the new relative-size-1 baseline.
	dayActivityBaselineGain = 9
)

// DayActivityReference returns the largest weighted count, before visual boost.
func DayActivityReference(values []DayActivity) float64 {
	var maximum float64
	for _, value := range values {
		maximum = max(maximum, dayEventWeight*finiteCount(value.Events)+dayThoughtWeight*finiteCount(value.Thoughts))
	}
	return maximum
}

// Event and Thought counts share one weighted-total reference. An Event has
// greater per-record weight, but numerous Thoughts can naturally dominate.
// Boost shapes the total; measured shares determine the component ratio.
// Both components use the same 0.75 height factor, reserving a quarter of the
// canvas below Total's crest without changing their count-driven ratio.
func DayActivityLevels(values []DayActivity, exponent, reference float64) []DayActivityLevel {
	if exponent <= 0 || math.IsNaN(exponent) || math.IsInf(exponent, 0) {
		exponent = DayActivityDefaultExponent
	}
	const componentHeightScale = .75
	maximum := finiteCount(reference)
	if maximum == 0 {
		maximum = DayActivityReference(values)
	}
	levels := make([]DayActivityLevel, len(values))
	if maximum == 0 {
		return levels
	}
	for i, value := range values {
		events, thoughts := dayEventWeight*finiteCount(value.Events), dayThoughtWeight*finiteCount(value.Thoughts)
		total := events + thoughts
		if total == 0 {
			continue
		}
		level := math.Pow(min(1, total/maximum), exponent)
		levels[i].Events = componentHeightScale * level * (events / total)
		levels[i].Thoughts = componentHeightScale * level * (thoughts / total)
		levels[i].Total = level
	}
	return levels
}
func finiteCount(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0
	}
	return value
}

// RenderDayActivity layers solid silhouettes from back to front: Total,
// Thoughts, then Events. Each foreground mountain fills to the baseline.
// A Braille cell has one foreground color, so the frontmost silhouette owns a
// shared cell. Its color follows the largest visible dot area; ties favor the
// foreground. A tiny lower layer cannot recolor a whole compact cell.
func RenderDayActivity(width, height int, values []DayActivity, options DayActivityOptions) [][]DayActivityCell {
	if width <= 0 || height <= 0 {
		return nil
	}
	rows := make([][]DayActivityCell, height)
	masks := make([][3]uint8, width*height)
	for y := range rows {
		rows[y] = make([]DayActivityCell, width)
		for x := range rows[y] {
			rows[y][x].Glyph = ' '
		}
	}
	if len(values) == 0 {
		return rows
	}

	samples, padding := values, 0
	if options.Before != nil || options.After != nil {
		padding = 1
		samples = make([]DayActivity, len(values)+2)
		samples[0], samples[len(samples)-1] = values[0], values[len(values)-1]
		if options.Before != nil {
			samples[0] = *options.Before
		}
		if options.After != nil {
			samples[len(samples)-1] = *options.After
		}
		copy(samples[1:], values)
	}
	size := options.Size
	if size <= 0 || math.IsNaN(size) || math.IsInf(size, 0) {
		size = 1
	}
	size = min(3, max(.5, size))
	levels := DayActivityLevels(samples, options.Exponent, options.Reference)
	for i := range levels {
		gain := size
		if size > 1 {
			// Above normal, lift smaller ridges toward the full-height summit
			// asymptotically. Unlike clipping, this preserves distinct peaks;
			// one common gain also retains the component ratios and Total ridge.
			gain /= 1 + (size-1)*levels[i].Total
		}
		levels[i].Total *= gain
		levels[i].Events *= gain
		levels[i].Thoughts *= gain
	}
	days := options.Days
	if days <= 0 {
		days = len(values)
	}
	points := make([]int, len(samples))
	for i := range points {
		day := float64(options.FirstDay) + (float64(i-padding)+.5)*float64(days)/float64(len(values))
		if options.SampleDays > 0 {
			day = float64(options.SampleFirstDay) + (float64(i-padding)+.5)*float64(options.SampleDays)
		}
		points[i] = dayActivityPixel(width, days, options.FirstDay, day)
	}

	// Normal is the former full mountain profile, now at relative size 1.
	maximum := float64(height*brailleRows-1) * (1 - math.Exp(-1.6*dayActivityBaselineGain))

	put := func(x, y int, series DayActivitySeries) {
		cell := &rows[y/brailleRows][x/brailleColumns]
		if cell.Glyph == ' ' {
			cell.Glyph = brailleBlank
		}
		bit := brailleBit(x%brailleColumns, y%brailleRows)
		cell.Glyph |= bit
		masks[(y/brailleRows)*width+x/brailleColumns][series] |= uint8(bit)
	}
	draw := func(series DayActivitySeries, filled bool) {
		previous, index := -1, 0
		for x := 0; x < width*brailleColumns; x++ {
			for index+1 < len(points) && x >= points[index+1] {
				index++
			}
			a, b := levels[index], levels[min(index+1, len(levels)-1)]
			left := [...]float64{a.Total, a.Events, a.Thoughts}[series]
			right := [...]float64{b.Total, b.Events, b.Thoughts}[series]
			value := left
			if x > points[index] && index+1 < len(points) && points[index+1] > points[index] {
				value += (right - left) * float64(x-points[index]) / float64(points[index+1]-points[index])
			}
			if value <= 0 {
				previous = -1
				continue
			}
			top := height*brailleRows - 1 - int(math.Round(value*maximum))
			bottom := top
			if filled {
				bottom = height*brailleRows - 1
			} else if options.Outline && previous >= 0 {
				bottom = max(top, previous)
				top = min(top, previous)
			}
			for y := top; y <= bottom; y++ {
				put(x, y, series)
			}
			previous = height*brailleRows - 1 - int(math.Round(value*maximum))
		}
	}
	if options.Total {
		draw(DayTotal, !options.Outline)
	}
	if options.Thoughts {
		draw(DayThoughts, !options.Outline)
	}
	if options.Events {
		draw(DayEvents, !options.Outline)
	}
	// Count only exposed dots after front layers occlude the layers behind them.
	// The glyph retains all silhouette dots; the terminal allows one color per cell.
	for y, row := range rows {
		for x := range row {
			mask := masks[y*width+x]
			visible := [3]uint8{
				DayTotal:    mask[DayTotal] &^ (mask[DayThoughts] | mask[DayEvents]),
				DayThoughts: mask[DayThoughts] &^ mask[DayEvents],
				DayEvents:   mask[DayEvents],
			}
			largest := 0
			for _, series := range [...]DayActivitySeries{DayTotal, DayThoughts, DayEvents} {
				count := bits.OnesCount8(visible[series])
				if count > 0 && count >= largest {
					row[x].Series, largest = series, count
				}
			}
		}
	}

	return rows
}

// Round vertices on an absolute calendar grid, then subtract a whole-cell
// origin. Panning translates existing Braille cells instead of changing the
// rounding of peaks or pairing their dots into differently colored cells.
func dayActivityPixel(width, days, firstDay int, day float64) int {
	origin := int(math.Floor(float64(firstDay)*float64(width)/float64(days))) * brailleColumns
	return int(math.Floor(day*float64(width*brailleColumns)/float64(days))) - origin
}

// DayActivityDayColumn places the accepted date on the same grid as the curves.
func DayActivityDayColumn(width, days, firstDay, selectedDay int) int {
	if width <= 0 || days <= 0 {
		return 0
	}
	x := dayActivityPixel(width, days, firstDay, float64(selectedDay)+.5)
	return max(0, min(width-1, x/brailleColumns))
}
