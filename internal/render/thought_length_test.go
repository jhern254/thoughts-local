package render

import (
	"math"
	"testing"
)

func TestRenderBars(t *testing.T) {
	t.Run("lengths stay ordered and scrolling preserves the event reference", func(t *testing.T) {
		bars := []Bar{{Row: 0, Value: 1}, {Row: 2, Value: 160}, {Row: 4, Value: 320}}
		options := BarOptions{Reference: 320}
		rows := RenderBars(10, 5, bars, options)
		occupied := func(row []Cell) int {
			n := 0
			for _, c := range row {
				if c.Glyph != ' ' {
					n++
				}
			}
			return n
		}
		if !(occupied(rows[0]) < occupied(rows[2]) && occupied(rows[2]) < occupied(rows[4])) {
			t.Fatal("longer thoughts must have longer bars")
		}
		cropped := RenderBars(10, 1, []Bar{{Row: 0, Value: 160}}, options)
		for x := range rows[2] {
			if rows[2][x].Glyph != cropped[0][x].Glyph {
				t.Fatal("scrolling changed scale")
			}
		}
		if occupied(rows[1]) != 1 || rows[1][9].Glyph != '⢸' || rows[1][9].CurveIndex != -1 {
			t.Fatal("separator must contain only the muted stem")
		}
	})
	t.Run("default contrast distinguishes similar lengths without changing equal values", func(t *testing.T) {
		bars := []Bar{{Row: 0, Value: 80}, {Row: 3, Value: 90}, {Row: 6, Value: 100}, {Row: 9, Value: 90}}
		rows := RenderBars(10, 10, bars, BarOptions{Reference: 100})
		previous := RenderBars(10, 10, bars, BarOptions{Reference: 100, Exponent: 0.8})
		width := func(row []Cell) int {
			n := 0
			for _, cell := range row {
				if cell.CurveIndex >= 0 {
					n++
				}
			}
			return n
		}
		if width(rows[0]) >= width(previous[0]) || width(rows[0]) >= width(rows[3]) || width(rows[3]) >= width(rows[6]) {
			t.Fatal("normal thought length contrast did not distinguish similar lengths")
		}
		if width(rows[3]) != width(rows[9]) {
			t.Fatal("equal lengths differ")
		}
	})
	t.Run("stem is continuous clipped and absent outside thought rows", func(t *testing.T) {
		bars := []Bar{{Row: 1, Value: 80}, {Row: 5, Value: 100}}
		rows := RenderBars(10, 7, bars, BarOptions{Reference: 100})
		for y := range rows {
			if (rows[y][9].Glyph != ' ') != (y >= 1 && y <= 5) {
				t.Fatalf("unexpected stem extent at row %d", y)
			}
		}
		crop := RenderBars(10, 3, []Bar{{Row: -1, Value: 80}, {Row: 3, Value: 100}}, BarOptions{Reference: 100})
		for y := range crop {
			for x := range crop[y] {
				if crop[y][x] != rows[y+2][x] {
					t.Fatal("cropping changed stem")
				}
			}
		}
	})

	t.Run("outline preserves bounds and removes interior dots", func(t *testing.T) {
		bars := []Bar{{Row: 0, Value: 100}}
		filled := RenderBars(10, 1, bars, BarOptions{Reference: 100})
		outline := RenderBars(10, 1, bars, BarOptions{Reference: 100, Mode: Outline})
		different := false
		for x, c := range filled[0] {
			o := outline[0][x]
			if (c.Glyph == ' ') != (o.Glyph == ' ') {
				t.Fatal("outline moved bounds")
			}
			different = different || c.Glyph != o.Glyph
		}
		if !different {
			t.Fatal("outline did not remove interior")
		}
	})
	t.Run("empty invalid and oversized inputs stay bounded", func(t *testing.T) {
		if RenderBars(0, 1, nil, BarOptions{}) != nil {
			t.Fatal("invalid width")
		}
		rows := RenderBars(1, 2, []Bar{{Row: -1, Value: 100}, {Row: 0, Value: 1000}, {Row: 1, Value: 0}}, BarOptions{Reference: 10, Size: 1.25})
		if len(rows) != 2 || len(rows[0]) != 1 || rows[0][0].Glyph == ' ' || rows[1][0].Glyph != ' ' {
			t.Fatalf("unexpected rows: %v", rows)
		}
	})
}

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
