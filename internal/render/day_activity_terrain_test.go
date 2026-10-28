package render

import (
	"fmt"
	"reflect"
	"testing"
)

// Daily counts are deliberately sparse and uneven; terrain may interpret their
// shoulders but must not manufacture unrelated activity.
func mountainFixtures() map[string][]DayActivity {
	fixtures := make(map[string][]DayActivity)
	for _, name := range []string{"dominant", "twins", "sustained", "isolated", "channels", "empty", "irregular"} {
		fixtures[name] = make([]DayActivity, 43) // Visible month plus six context days per side.
	}
	fixtures["dominant"][21] = DayActivity{Events: 18, Thoughts: 160}
	fixtures["dominant"][18] = DayActivity{Events: 5, Thoughts: 36}
	fixtures["dominant"][25] = DayActivity{Events: 3, Thoughts: 50}
	fixtures["twins"][17] = DayActivity{Events: 14, Thoughts: 120}
	fixtures["twins"][25] = DayActivity{Events: 12, Thoughts: 140}
	for i := 17; i <= 25; i++ {
		fixtures["sustained"][i] = DayActivity{Events: float64(12 + i%4), Thoughts: float64(80 + i%3*12)}
	}
	fixtures["isolated"][21] = DayActivity{Events: 18, Thoughts: 160}
	fixtures["channels"][16] = DayActivity{Thoughts: 220}
	fixtures["channels"][27] = DayActivity{Events: 95}
	for i := range fixtures["irregular"] {
		if i%5 != 0 && i%7 != 0 {
			fixtures["irregular"][i] = DayActivity{Events: float64(i * i % 19), Thoughts: float64(i * 37 % 150)}
		}
	}
	return fixtures
}

func mountainOptions() DayActivityOptions {
	return DayActivityOptions{
		FirstDay:       20000,
		Days:           31,
		SampleFirstDay: 19994,
		SampleDays:     1,
		Reference:      75,
		Total:          true,
		Events:         true,
		Thoughts:       true,
		Baseline:       true,
	}
}

func BenchmarkDayMountainRender(b *testing.B) {
	values, options := mountainFixtures()["irregular"], mountainOptions()
	for _, size := range [][2]int{{40, 2}, {80, 4}, {80, 6}, {80, 8}, {120, 6}, {120, 8}} {
		b.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = RenderDayActivity(size[0], size[1], values, options)
			}
		})
	}
}

func TestDayActivity_Terrain(t *testing.T) {
	t.Run("isolated activity has broad asymmetric finite shoulders", func(t *testing.T) {
		cells := RenderDayActivity(124, 8, mountainFixtures()["isolated"], mountainOptions())
		occupied := func(column int) bool {
			for _, row := range cells[:7] {
				if row[column].Glyph != ' ' {
					return true
				}
			}
			return cells[7][column].Glyph != '⣀'
		}
		if !occupied(46) {
			t.Fatal("four days before the summit has no broad shoulder")
		}
		for _, column := range []int{0, 35, 79, 100, 123} {
			if occupied(column) {
				t.Fatalf("column %d invented activity beyond the six/four-day supports", column)
			}
		}
	})
	t.Run("identical calendar data translates exactly including incoming shoulders", func(t *testing.T) {
		for _, outline := range []bool{false, true} {
			values := mountainFixtures()["irregular"]
			options := mountainOptions()
			options.Outline = outline
			before := RenderDayActivity(93, 8, values, options)
			if !reflect.DeepEqual(before, RenderDayActivity(93, 8, values, options)) {
				t.Fatal("terrain is not deterministic")
			}
			options.FirstDay++
			// One day scroll retires a distant context bucket and adds another; all
			// activity capable of reaching the shared viewport remains in the read.
			options.SampleFirstDay++
			shifted := append(append([]DayActivity{}, values[1:]...), DayActivity{})
			after := RenderDayActivity(93, 8, shifted, options)
			for y := range before {
				for x := 0; x+3 < 93; x++ {
					if before[y][x+3] != after[y][x] {
						t.Fatalf("outline=%v row=%d column=%d changed during pan", outline, y, x)
					}
				}
			}
		}
	})
	t.Run("all requested canvases remain bounded", func(t *testing.T) {
		for _, size := range [][2]int{{40, 2}, {80, 4}, {80, 6}, {80, 8}, {120, 6}, {120, 8}} {
			cells := RenderDayActivity(size[0], size[1], mountainFixtures()["twins"], mountainOptions())
			if len(cells) != size[1] {
				t.Fatal("wrong row count")
			}
			for _, row := range cells {
				if len(row) != size[0] {
					t.Fatal("wrong column count")
				}
			}
		}
	})
	t.Run("neighboring activity retains two summits and a distinct valley", func(t *testing.T) {
		cells := RenderDayActivity(124, 8, mountainFixtures()["twins"], mountainOptions())
		skyline := func(column int) int {
			for y, row := range cells {
				if row[column].Glyph != ' ' {
					return y
				}
			}
			return len(cells)
		}
		left, right, valley := skyline(46), skyline(78), skyline(62)
		if valley < max(left, right)+2 {
			t.Fatalf("summits at rows %d/%d and valley at %d, want two-row separation", left, right, valley)
		}
	})
	t.Run("each channel can be shown alone without leaking another channel's color", func(t *testing.T) {
		for _, series := range []DayActivitySeries{DayTotal, DayEvents, DayThoughts} {
			options := mountainOptions()
			options.Total, options.Events, options.Thoughts = series == DayTotal, series == DayEvents, series == DayThoughts
			options.Baseline = false
			visible := false
			for _, row := range RenderDayActivity(80, 8, mountainFixtures()["channels"], options) {
				for _, cell := range row {
					if cell.Glyph != ' ' {
						visible = true
						if cell.Series != series {
							t.Fatalf("only channel %d enabled, got color %d", series, cell.Series)
						}
					}
				}
			}
			if !visible {
				t.Fatalf("enabled channel %d vanished", series)
			}
		}
	})
}
