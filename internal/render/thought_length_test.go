package render

import (
	"math"
	"testing"
)

func TestThoughtLengthExponent(t *testing.T) {
	t.Run("shared boost preserves distinct defaults and direction", func(t *testing.T) {
		previous := math.Inf(1)
		for _, shared := range []float64{1, 0.8, 0.5, 0.25, 0.15} {
			exponent := ThoughtLengthExponent(shared)
			if exponent >= previous {
				t.Fatal("more boost did not lower thought length contrast")
			}
			previous = exponent
		}
		for _, shared := range []float64{0.8, 0, -1, math.NaN(), math.Inf(1)} {
			if got := ThoughtLengthExponent(shared); math.Abs(got-3) > 1e-12 {
				t.Fatalf("got default exponent %v, want 3", got)
			}
		}
	})
}
