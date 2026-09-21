// Package render draws terminal distributions without application state or styling.
package render

import "math"

// Mode selects a fill or continuous outline of the same Gaussian profile.
type Mode uint8

const (
	Filled  Mode = iota // The default fills from the curve to its right-hand baseline.
	Outline             // Only the connected Gaussian boundary is drawn.
)

type Options struct {
	Mode Mode // The zero value selects Filled.
	// CountReference can include counts whose curves are hidden (e.g. an expanded
	// card). The actual reference is max(20, CountReference, all valid input counts),
	// including offscreen inputs. Zero selects automatic whole-profile scaling.
	CountReference int64
	// CountExponent tunes contrast between counts, not the maximum dimensions.
	// Zero, negative and nonfinite values use 0.8. Lower positive values enlarge
	// smaller counts: try 0.5 for a moderate boost or 0.25 for a strong boost.
	// With reference 200, exponent 0.25 makes count 20 about as large as count 100
	// at exponent 0.8. Zero's mound and the maximum stay fixed; 1 is linear.
	CountExponent float64
	// Size scales the amplitude/height gains, preserving the zero-count mound.
	// 1 is the default; 0.5 shrinks the gains and 1.25 enlarges them. Values are
	// bounded to [0.5, 1.25]; zero, negative or nonfinite values use 1.
	// Width still fits the caller's lane; this never allocates additional rows.
	Size float64
}

type Distribution struct {
	// CenterY is in Braille dots, not terminal rows: one cell is 2x4 dots.
	// A card at row top with h rows has center 4*top + (4*h-1)/2.
	// Centers outside the viewport are valid; nonfinite centers are ignored.
	CenterY float64
	Count   int64 // Negative counts use the zero-count mound.
}

type Cell struct {
	Glyph      rune // A Braille glyph or an ordinary space.
	CurveIndex int  // Index in the input slice; -1 means blank or connecting baseline.
}

// Tuning values are in Braille dots unless otherwise noted. These defaults match
// the reviewed preview; changing them must not change the caller's card layout.
// Set Options.CountExponent to boost small counts relative to the day maximum.
// For gentler boosting of low counts, raise it toward 1. For a broader,
// shallower zero mound, raise minHeight and lower minAmplitude independently.
const (
	countReference = 20.0 // Minimum reference keeps quiet days from magnifying tiny counts.
	countExponent  = 0.8  // 1 is linear; lowering this makes small counts larger sooner.
	minAmplitude   = 3.0  // Zero's shallow leftward peak; increase for a stronger mound.
	amplitudeGain  = 13.0 // Default maximum amplitude is 16 dots; Size scales this gain.
	minHeight      = 12.0 // Zero's broad base: three rows including both tails.
	heightGain     = 20.0 // Default maximum height is eight rows; Size scales this gain.
	tailSigma      = 3.0  // Keep both tails through +/-3 standard deviations.
	samplesPerDot  = 8    // Sub-dot sampling plus connected raster points avoids holes.
)

type gaussian struct {
	center, sigma, amplitude float64
	index                    int
}

// mixtureAt adds activity contributions; the dominant kernel supplies color
// ownership only. Geometry must not switch kernels at their intersection.
func mixtureAt(curves []gaussian, y float64) (total, strongest float64, owner int) {
	owner = -1
	for _, curve := range curves {
		z := (y - curve.center) / curve.sigma
		if math.Abs(z) > tailSigma {
			continue
		}
		value := curve.amplitude * math.Exp(-0.5*z*z)
		total += value
		if value > strongest {
			strongest, owner = value, curve.index
		}
	}
	return total, strongest, owner
}

// mixtureGain fits the entire supplied profile, not just its visible portion.
// A single gain preserves valleys and peak ratios without flat clipping or
// changes when scrolling. Scan kernel supports rather than potentially huge
// idle gaps, on the same fixed sub-dot grid used for drawing below.
func mixtureGain(curves []gaussian, maximum float64) float64 {
	peak := 0.0
	for _, curve := range curves {
		radius := tailSigma * curve.sigma
		start := math.Ceil((curve.center-radius)*samplesPerDot) / samplesPerDot
		steps := int(math.Ceil(2*radius*samplesPerDot)) + 1
		for step := 0; step <= steps; step++ {
			y := start + float64(step)/samplesPerDot
			if y > curve.center+radius {
				break
			}
			total, _, _ := mixtureAt(curves, y)
			peak = max(peak, total)
		}
	}
	if peak > maximum {
		return maximum / peak
	}
	return 1
}

// RenderDistributions returns exactly height rows of width cells, or nil for a
// nonpositive dimension. Empty input yields blank cells. Invalid modes use Filled.
// Colors belong to the caller; selection therefore cannot change geometry.
//
// This is an unnormalized, sideways Gaussian activity summary, not a probability
// density or a fit to thought timestamps. For count n and caller-supplied center mu:
//
//	R = max(countReference, options.CountReference, all valid input counts)
//	s = (max(n, 0) / R)^exponent
//	A = minAmplitude + amplitudeGain*size*s
//	H = round(minHeight + heightGain*size*s)
//	sigma = (H-1)/(2*tailSigma)
//	g_i(y) = A_i*exp(-0.5*((y-mu_i)/sigma_i)^2)
//	S(y) = sum(g_i(y))
//	gain = min(1, maximumDisplayAmplitude / max_y(S(y)))
//	x(y) = baseline - gain*S(y)
//
// exponent is options.CountExponent or the default countExponent. R uses the
// whole supplied timeline, never just the viewport. size is options.Size or 1,
// bounded to [0.5, 1.25]. Counts above 20 participate in relative scaling;
// maximum dimensions remain bounded. Braille rounding can hide small differences.
// H includes the two endpoints, hence H-1. The baseline is the rightmost dot in
// the lane. maximumDisplayAmplitude is min(minAmplitude+amplitudeGain*size, lane width
// in dots minus 1). The global gain only shrinks profiles that exceed that limit;
// isolated curves retain their original scale. y is clipped, never rescaled.
// Adding kernels allows multiple peaks and shared valleys; close peaks can merge.
// A baseline connects the first tail to the last, including idle gaps. Color is
// assigned to the strongest visible contribution, with input order breaking ties.
func RenderDistributions(width, height int, distributions []Distribution, options Options) [][]Cell {
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
	reference := max(countReference, float64(options.CountReference))
	for _, distribution := range distributions {
		if !math.IsNaN(distribution.CenterY) && !math.IsInf(distribution.CenterY, 0) {
			reference = max(reference, float64(distribution.Count))
		}
	}
	exponent := options.CountExponent
	if exponent <= 0 || math.IsNaN(exponent) || math.IsInf(exponent, 0) {
		exponent = countExponent
	}
	size := options.Size
	if size <= 0 || math.IsNaN(size) || math.IsInf(size, 0) {
		size = 1
	}
	size = min(1.25, max(0.5, size))
	baseline := 2*width - 1
	curves := make([]gaussian, 0, len(distributions))
	first, last := math.Inf(1), math.Inf(-1)
	for i, distribution := range distributions {
		if math.IsNaN(distribution.CenterY) || math.IsInf(distribution.CenterY, 0) {
			continue
		}
		s := math.Pow(max(float64(distribution.Count), 0)/reference, exponent)
		h := math.Round(minHeight + heightGain*size*s)
		g := gaussian{distribution.CenterY, (h - 1) / (2 * tailSigma), minAmplitude + amplitudeGain*size*s, i}
		curves = append(curves, g)
		first = min(first, g.center-tailSigma*g.sigma)
		last = max(last, g.center+tailSigma*g.sigma)
	}
	if len(curves) == 0 || first > float64(height*4)-0.5 || last < -0.5 {
		return rows
	}
	gain := mixtureGain(curves, min(minAmplitude+amplitudeGain*size, float64(baseline)))

	strengths := make([]float64, width*height)
	bits := [2][4]rune{{1, 2, 4, 64}, {8, 16, 32, 128}}
	plot := func(x, y, owner int, strength float64) {
		if x < 0 || x > baseline || y < 0 || y >= height*4 {
			return
		}
		cell := &rows[y/4][x/2]
		if cell.Glyph == ' ' {
			cell.Glyph = 0x2800
		}
		cell.Glyph |= bits[x%2][y%4]
		index := (y/4)*width + x/2
		if owner >= 0 && (strength > strengths[index] || (strength == strengths[index] && owner < cell.CurveIndex)) {
			cell.CurveIndex = owner
			strengths[index] = strength
		}
	}

	if options.Mode == Outline {
		drawOutline(curves, gain, baseline, height*4, first, last, plot)
		return rows
	}

	// Sample on a fixed dot grid, including points that round into edge pixels.
	// This makes clipping identical to cropping a larger render at a cell boundary.
	start := int(math.Ceil(max(first, -0.5) * samplesPerDot))
	end := int(math.Floor(min(last, float64(height*4)-0.5) * samplesPerDot))
	previousX, previousY := 0, 0
	for sample := start; sample <= end; sample++ {
		y := float64(sample) / samplesPerDot
		total, strength, owner := mixtureAt(curves, y)
		strength *= gain
		x, py := int(math.Round(float64(baseline)-gain*total)), int(math.Round(y))
		if x == baseline {
			owner, strength = -1, 0 // No visible departure from the connecting baseline.
		}
		if sample == start {
			previousX, previousY = x, py
		}
		steps := max(1, int(math.Abs(float64(x-previousX))), int(math.Abs(float64(py-previousY))))
		for step := 1; step <= steps; step++ {
			fraction := float64(step) / float64(steps)
			px := previousX + int(math.Round(float64(x-previousX)*fraction))
			row := previousY + int(math.Round(float64(py-previousY)*fraction))
			for column := px; column <= baseline; column++ {
				plot(column, row, owner, strength)
			}
		}
		// A sample exactly between dot rows belongs to both pixels. Always
		// rounding half upward would make the lower half of a bell heavier.
		if y-math.Floor(y) == 0.5 {
			for column := x; column <= baseline; column++ {
				plot(column, int(math.Floor(y)), owner, strength)
				plot(column, int(math.Ceil(y)), owner, strength)
			}
		}
		previousX, previousY = x, py
	}
	return rows
}

// drawOutline traces the Gaussian at dot-row centers. Connect each pair through
// the curve at their shared half-row so incoming and outgoing strokes meet.
// This avoids growing horizontal stubs from the leftmost sample in each row.
// Adjacent rows share a dot column, avoiding diagonal-only gaps between glyphs.
func drawOutline(curves []gaussian, gain float64, baseline, height int, first, last float64, plot func(int, int, int, float64)) {
	type point struct {
		x, owner int
		strength float64
	}
	at := func(y float64) point {
		total, strength, owner := mixtureAt(curves, y)
		return point{int(math.Round(float64(baseline) - gain*total)), owner, gain * strength}
	}
	stroke := func(p point, y, from, to int) {
		for x := min(from, to); x <= max(from, to); x++ {
			if x == baseline {
				plot(x, y, -1, 0)
			} else {
				plot(x, y, p.owner, p.strength)
			}
		}
	}
	// Keep one neighboring row beyond each viewport edge so cropped strokes
	// match the same rows of a larger render, including their color ownership.
	start := int(math.Ceil(max(first-0.5, -1)))
	end := int(math.Floor(min(last+0.5, float64(height))))
	previous := at(float64(start))
	stroke(previous, start, previous.x, previous.x)
	for y := start + 1; y <= end; y++ {
		current := at(float64(y))
		join := at(float64(y) - 0.5).x
		join = min(max(join, min(previous.x, current.x)), max(previous.x, current.x))
		stroke(previous, y-1, previous.x, join)
		stroke(current, y, join, current.x)
		previous = current
	}
}
