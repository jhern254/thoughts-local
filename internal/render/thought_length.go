package render

import "math"

// A cubic default spreads similar lengths near the event maximum farther apart.
// This is separate from countExponent: collapsed event curves keep their 0.8
// default. Raise thoughtLengthExponent for more contrast; lower it for fuller bars.
const thoughtLengthExponent = defaultHorizontalBarExponent

// ThoughtLengthExponent maps the single distribution boost control to the thought length bars'
// own default. Normal curve boost (0.8) gives cubic bars (3.0); increasing boost
// lowers both exponents, making small values fuller in both views. This shared
// mapping also keeps the preview command consistent with Events.
func ThoughtLengthExponent(curveExponent float64) float64 {
	if curveExponent <= 0 || math.IsNaN(curveExponent) || math.IsInf(curveExponent, 0) {
		return thoughtLengthExponent
	}
	return thoughtLengthExponent * curveExponent / countExponent
}
