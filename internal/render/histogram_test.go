package render

import "testing"

func TestRenderHistogram(t *testing.T) {
	t.Run("lengths stay ordered and scrolling preserves the event reference", func(t *testing.T) {
		bars := []HistogramBar{{Row: 0, Value: 1}, {Row: 2, Value: 90}, {Row: 4, Value: 320}}
		options := HistogramOptions{Reference: 320}
		rows := RenderHistogram(10, 5, bars, options)
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
		cropped := RenderHistogram(10, 1, []HistogramBar{{Row: 0, Value: 90}}, options)
		for x := range rows[2] {
			if rows[2][x].Glyph != cropped[0][x].Glyph {
				t.Fatal("scrolling changed scale")
			}
		}
		if occupied(rows[1]) != 0 {
			t.Fatal("separator contains dots")
		}
	})
	t.Run("outline preserves bounds and removes interior dots", func(t *testing.T) {
		bars := []HistogramBar{{Row: 0, Value: 100}}
		filled := RenderHistogram(10, 1, bars, HistogramOptions{Reference: 100})
		outline := RenderHistogram(10, 1, bars, HistogramOptions{Reference: 100, Mode: Outline})
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
		if RenderHistogram(0, 1, nil, HistogramOptions{}) != nil {
			t.Fatal("invalid width")
		}
		rows := RenderHistogram(1, 2, []HistogramBar{{Row: -1, Value: 100}, {Row: 0, Value: 1000}, {Row: 1, Value: 0}}, HistogramOptions{Reference: 10, Size: 1.25})
		if len(rows) != 2 || len(rows[0]) != 1 || rows[0][0].Glyph == ' ' || rows[1][0].Glyph != ' ' {
			t.Fatalf("unexpected rows: %v", rows)
		}
	})
}
