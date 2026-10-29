package render

import (
	"math"
	"math/bits"
)

// DayActivity contains average daily counts for one calendar bucket. It is not
// a probability density: terrain interprets bounded shoulders around real counts.
type DayActivity struct{ Events, Thoughts float64 }

// DayActivityDefaultExponent retains quieter slopes while preserving the measured
// ordering of the samples before the bounded terrain envelope is applied.
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
	// Optional single-bucket context for standalone callers. The navigator
	// supplies all six neighboring buckets in Samples using SampleFirstDay.
	Before, After           *DayActivity
	Total, Events, Thoughts bool
	Outline                 bool
	// Baseline joins the bottom Braille dots across the canvas, including gaps.
	Baseline bool
	Exponent float64
	// Size is relative to normal (1 = 100%), bounded to [0.5, 3].
	Size float64
}

// DayActivityContextBuckets covers the widest mountain shoulder. Callers load
// this many whole buckets beyond each edge, within their existing query limit.
const DayActivityContextBuckets = 6

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
			if options.Baseline && y == height-1 {
				// Bits 6 and 7 are the two bottom dots of a Braille cell.
				rows[y][x].Glyph = brailleBlank | 0xc0
			}
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
	terrain := dayActivityTerrain(width, days, padding, len(values), levels, options)

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
		previous := -1
		for x := 0; x < width*brailleColumns; x++ {
			level := terrain[x+1]
			value := [...]float64{level.Total, level.Events, level.Thoughts}[series]
			if x == 0 && options.Outline {
				// Continue the contour from the preceding absolute dot, including at
				// viewport edges. Restarting here changes a clipped outline on pan.
				prior := terrain[0]
				if v := ([...]float64{prior.Total, prior.Events, prior.Thoughts})[series]; v > 0 {
					previous = height*brailleRows - 1 - int(math.Round(v*maximum))
				}
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

// dayActivityTerrain takes the maximum of finite asymmetric raised-cosine
// envelopes, not their sum: nearby samples form shoulders without inflating the
// historical reference or inventing measurements on otherwise empty days.
// The extra first value is the preceding dot, needed for clipped outlines.
func dayActivityTerrain(width, days, padding, visibleSamples int, levels []DayActivityLevel, options DayActivityOptions) []DayActivityLevel {
	type mountain struct {
		center                 int
		left, right, roughness float64
		level                  DayActivityLevel
	}
	bucketDays := float64(days) / float64(visibleSamples)
	if options.SampleDays > 0 {
		bucketDays = float64(options.SampleDays)
	}
	pixelsPerBucket := bucketDays * float64(width*brailleColumns) / float64(days)
	mountains := make([]mountain, 0, len(levels))
	for i, level := range levels {
		if level.Total == 0 {
			continue
		}
		day := float64(options.FirstDay) + (float64(i-padding)+.5)*bucketDays
		if options.SampleDays > 0 {
			day = float64(options.SampleFirstDay) + (float64(i-padding)+.5)*bucketDays
		}
		// A stable integer mix varies shoulders by calendar bucket, never by screen
		// position. This is visual interpretation, not random or intraday activity.
		seed := uint64(int64(math.Floor(day)))
		seed ^= seed >> 16
		seed *= 0x45d9f3b
		seed ^= seed >> 16
		fraction := float64(seed&0xffff) / 65535
		mountains = append(mountains, mountain{
			center:    dayActivityPixel(width, days, options.FirstDay, day),
			left:      1.8 + .9*fraction,
			right:     1.2 + .6*float64((seed>>16)&0xffff)/65535,
			roughness: .02 + .02*fraction,
			level:     level,
		})
	}
	terrain := make([]DayActivityLevel, width*brailleColumns+1)
	for column := range terrain {
		x := column - 1
		for _, peak := range mountains {
			distance := float64(x-peak.center) / pixelsPerBucket
			radius := peak.right
			if distance < 0 {
				radius = peak.left
			}
			if math.Abs(distance) >= radius {
				continue
			}
			// Equal cosine and linear contributions retain shoulders while
			// separating nearby summits, as in the selected browser comparison.
			slope := math.Abs(distance) / radius
			kernel := .5*(1+math.Cos(math.Pi*slope))/2 + .5*(1-slope)
			current := &terrain[column]
			if kernel*peak.level.Total <= current.Total && kernel*peak.level.Events <= current.Events && kernel*peak.level.Thoughts <= current.Thoughts {
				continue
			}
			// Restrained multiscale erosion only lowers this envelope (2–4% maximum).
			// Both scales vanish at the measured summit and stay calendar-anchored.
			coarse, fine := math.Sin(math.Pi*distance/1.6), math.Sin(math.Pi*distance/.65)
			kernel *= 1 - peak.roughness*(.65*coarse*coarse+.35*fine*fine)
			current.Total = max(current.Total, kernel*peak.level.Total)
			current.Events = max(current.Events, kernel*peak.level.Events)
			current.Thoughts = max(current.Thoughts, kernel*peak.level.Thoughts)
		}
	}
	return terrain
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
