// Package render draws terminal distributions without application state or styling.
package render

import "math"

// Mode changes the interior, never the distribution's boundary or position.
type Mode uint8

const (
	Filled  Mode = iota // The default fills from the curve to its right-hand baseline.
	Outline             // Only the connected Gaussian boundary is drawn.
)

type Options struct {
	Mode Mode // The zero value selects Filled.
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
// For gentler boosting of low counts, raise countExponent toward 1. For a broader,
// shallower zero mound, raise minHeight and lower minAmplitude independently.
const (
	countCap      = 20.0 // Counts above this have the same maximum dimensions.
	countExponent = 0.8  // 1 is linear; lowering this makes small counts larger sooner.
	minAmplitude  = 3.0  // Zero's shallow leftward peak; increase for a stronger mound.
	amplitudeGain = 13.0 // Maximum amplitude is minAmplitude + amplitudeGain (16 dots).
	minHeight     = 12.0 // Zero's broad base: three rows including both tails.
	heightGain    = 20.0 // Maximum height is minHeight + heightGain (eight rows).
	tailSigma     = 3.0  // Keep both tails through +/-3 standard deviations.
	samplesPerDot = 8    // Sub-dot sampling plus connected raster points avoids holes.
)

type gaussian struct {
	center, sigma, amplitude float64
	index                    int
}

// RenderDistributions returns exactly height rows of width cells, or nil for a
// nonpositive dimension. Empty input yields blank cells. Invalid modes use Filled.
// Colors belong to the caller; selection therefore cannot change geometry.
//
// This is an unnormalized, sideways Gaussian activity summary, not a probability
// density or a fit to thought timestamps. For count n and caller-supplied center mu:
//
//	s = (clamp(n, 0, countCap) / countCap)^countExponent
//	A = minAmplitude + amplitudeGain*s
//	H = round(minHeight + heightGain*s)
//	sigma = (H-1)/(2*tailSigma)
//	x(y) = baseline - A*exp(-0.5*((y-mu)/sigma)^2)
//
// H includes the two endpoints, hence H-1. The baseline is the rightmost dot in
// the lane. A is clamped to the available width; y is clipped, never rescaled.
// The outermost curve wins overlaps (not a sum); input order breaks exact ties.
// A baseline connects the first tail to the last, including idle gaps. A terminal
// cell has only one color, so its strongest visible contribution owns the cell.
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
	baseline := 2*width - 1
	curves := make([]gaussian, 0, len(distributions))
	first, last := math.Inf(1), math.Inf(-1)
	for i, distribution := range distributions {
		if math.IsNaN(distribution.CenterY) || math.IsInf(distribution.CenterY, 0) {
			continue
		}
		s := math.Pow(min(max(float64(distribution.Count), 0), countCap)/countCap, countExponent)
		h := math.Round(minHeight + heightGain*s)
		g := gaussian{distribution.CenterY, (h - 1) / (2 * tailSigma), min(minAmplitude+amplitudeGain*s, float64(baseline)), i}
		curves = append(curves, g)
		first = min(first, g.center-tailSigma*g.sigma)
		last = max(last, g.center+tailSigma*g.sigma)
	}
	if len(curves) == 0 || first > float64(height*4)-0.5 || last < -0.5 {
		return rows
	}

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

	// Sample on a fixed dot grid, including points that round into edge pixels.
	// This makes clipping identical to cropping a larger render at a cell boundary.
	start := int(math.Ceil(max(first, -0.5) * samplesPerDot))
	end := int(math.Floor(min(last, float64(height*4)-0.5) * samplesPerDot))
	previousX, previousY := 0, 0
	for sample := start; sample <= end; sample++ {
		y := float64(sample) / samplesPerDot
		strength, owner := 0.0, -1
		for _, curve := range curves {
			z := (y - curve.center) / curve.sigma
			if math.Abs(z) > tailSigma {
				continue
			}
			value := curve.amplitude * math.Exp(-0.5*z*z)
			if value > strength {
				strength, owner = value, curve.index
			}
		}
		x, py := int(math.Round(float64(baseline)-strength)), int(math.Round(y))
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
			until := px
			if options.Mode != Outline {
				until = baseline
			}
			for column := px; column <= until; column++ {
				plot(column, row, owner, strength)
			}
		}
		// A sample exactly between dot rows belongs to both pixels. Always
		// rounding half upward would make the lower half of a bell heavier.
		if y-math.Floor(y) == 0.5 {
			until := x
			if options.Mode != Outline {
				until = baseline
			}
			for column := x; column <= until; column++ {
				plot(column, int(math.Floor(y)), owner, strength)
				plot(column, int(math.Ceil(y)), owner, strength)
			}
		}
		previousX, previousY = x, py
	}
	return rows
}
