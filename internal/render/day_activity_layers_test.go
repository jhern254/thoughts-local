package render

import (
	"math"
	"testing"
)

func TestDayActivity_SharedLayers(t *testing.T) {
	t.Run("inner boundaries follow Total and retain each weighted band", func(t *testing.T) {
		options := mountainOptions()
		values := mountainFixtures()["irregular"]
		levels := DayActivityLevels(values, options.Exponent, options.Reference)
		terrain := dayActivityTerrain(80, options.Days, 0, len(values), levels, options)
		for column, level := range terrain {
			if level.Total == 0 {
				continue
			}
			if math.Abs(level.Thoughts-.75*level.Total) > 1e-12 {
				t.Fatalf("dot %d: upper Thoughts boundary=%v, want 75%% of Total=%v", column, level.Thoughts, level.Total)
			}
			if level.Events < 0 || level.Events > level.Thoughts {
				t.Fatalf("dot %d: Events=%v, want within Thoughts boundary=%v", column, level.Events, level.Thoughts)
			}
		}
		options.Events = false
		withoutEvents := dayActivityTerrain(80, options.Days, 0, len(values), levels, options)
		for column, level := range terrain {
			remaining := withoutEvents[column]
			if remaining.Total != level.Total || math.Abs(remaining.Thoughts+level.Events-level.Thoughts) > 1e-12 {
				t.Fatalf("dot %d: hiding Events changed Total or failed to remove its offset: before=%+v after=%+v", column, level, remaining)
			}
		}
	})
	t.Run("balanced activity exposes all three solid layers at compact height", func(t *testing.T) {
		options := mountainOptions()
		values := make([]DayActivity, 43)
		values[21] = DayActivity{Events: 10, Thoughts: 20}
		options.Reference = 13
		for _, height := range []int{3, 4} {
			seen := [3]bool{}
			for _, row := range RenderDayActivity(80, height, values, options) {
				for _, cell := range row {
					if cell.Glyph != ' ' {
						seen[cell.Series] = true
					}
				}
			}
			if seen != [3]bool{true, true, true} {
				t.Fatalf("height %d: visible layers=%v, want Total, Events and Thoughts", height, seen)
			}
		}
	})
	t.Run("filled columns have no cut-out gaps", func(t *testing.T) {
		for name, values := range mountainFixtures() {
			for _, size := range [][2]int{{40, 2}, {80, 4}, {120, 4}} {
				options := mountainOptions()
				options.Baseline = false
				rows := RenderDayActivity(size[0], size[1], values, options)
				for x := 0; x < size[0]*brailleColumns; x++ {
					started := false
					for y := 0; y < size[1]*brailleRows; y++ {
						glyph := rows[y/brailleRows][x/brailleColumns].Glyph
						present := glyph >= brailleBlank && glyph&brailleBit(x%brailleColumns, y%brailleRows) != 0
						if started && !present {
							t.Fatalf("%s %dx%d: gap at dot %d,%d", name, size[0], size[1], x, y)
						}
						started = started || present
					}
				}
			}
		}
	})
	t.Run("an outline cell never recolors dots from another contour", func(t *testing.T) {
		options := mountainOptions()
		options.Outline, options.Baseline, options.Thoughts = true, false, false
		values := mountainFixtures()["irregular"]
		combined := RenderDayActivity(80, 4, values, options)
		options.Total = false
		events := RenderDayActivity(80, 4, values, options)
		options.Total, options.Events = true, false
		total := RenderDayActivity(80, 4, values, options)
		for y, row := range combined {
			for x, cell := range row {
				if cell.Glyph == ' ' {
					continue
				}
				expected := total[y][x]
				if cell.Series == DayEvents {
					expected = events[y][x]
				}
				if cell.Glyph != expected.Glyph {
					t.Fatalf("cell %d,%d: series %d glyph=%q, want only its own contour %q", x, y, cell.Series, cell.Glyph, expected.Glyph)
				}
			}
		}
	})
	t.Run("outline has no separate baseline or invented empty activity", func(t *testing.T) {
		options := mountainOptions()
		options.Outline = true
		for _, row := range RenderDayActivity(40, 3, mountainFixtures()["empty"], options) {
			for _, cell := range row {
				if cell.Glyph != ' ' {
					t.Fatalf("empty outline: got %q, want blank", cell.Glyph)
				}
			}
		}
	})
}
