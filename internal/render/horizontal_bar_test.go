package render

import "testing"

func TestRenderHorizontalBars(t *testing.T) {
	t.Run("lengths stay ordered and scrolling preserves the event reference", func(t *testing.T) {
		bars := []HorizontalBar{{Row: 0, Value: 1}, {Row: 2, Value: 160}, {Row: 4, Value: 320}}
		options := HorizontalBarOptions{Reference: 320}
		rows := RenderHorizontalBars(10, 5, bars, options)
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
		cropped := RenderHorizontalBars(10, 1, []HorizontalBar{{Row: 0, Value: 160}}, options)
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
		bars := []HorizontalBar{{Row: 0, Value: 80}, {Row: 3, Value: 90}, {Row: 6, Value: 100}, {Row: 9, Value: 90}}
		rows := RenderHorizontalBars(10, 10, bars, HorizontalBarOptions{Reference: 100})
		previous := RenderHorizontalBars(10, 10, bars, HorizontalBarOptions{Reference: 100, Exponent: 0.8})
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
		bars := []HorizontalBar{{Row: 1, Value: 80}, {Row: 5, Value: 100}}
		rows := RenderHorizontalBars(10, 7, bars, HorizontalBarOptions{Reference: 100})
		for y := range rows {
			if (rows[y][9].Glyph != ' ') != (y >= 1 && y <= 5) {
				t.Fatalf("unexpected stem extent at row %d", y)
			}
		}
		crop := RenderHorizontalBars(10, 3, []HorizontalBar{{Row: -1, Value: 80}, {Row: 3, Value: 100}}, HorizontalBarOptions{Reference: 100})
		for y := range crop {
			for x := range crop[y] {
				if crop[y][x] != rows[y+2][x] {
					t.Fatal("cropping changed stem")
				}
			}
		}
	})

	t.Run("outline preserves bounds and removes interior dots", func(t *testing.T) {
		bars := []HorizontalBar{{Row: 0, Value: 100}}
		filled := RenderHorizontalBars(10, 1, bars, HorizontalBarOptions{Reference: 100})
		outline := RenderHorizontalBars(10, 1, bars, HorizontalBarOptions{Reference: 100, Mode: Outline})
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
		if RenderHorizontalBars(0, 1, nil, HorizontalBarOptions{}) != nil {
			t.Fatal("invalid width")
		}
		rows := RenderHorizontalBars(1, 2, []HorizontalBar{{Row: -1, Value: 100}, {Row: 0, Value: 1000}, {Row: 1, Value: 0}}, HorizontalBarOptions{Reference: 10, Size: 1.25})
		if len(rows) != 2 || len(rows[0]) != 1 || rows[0][0].Glyph == ' ' || rows[1][0].Glyph != ' ' {
			t.Fatalf("unexpected rows: %v", rows)
		}
	})
}
