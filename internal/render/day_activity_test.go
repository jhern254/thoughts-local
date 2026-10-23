package render

import (
	"math"
	"testing"
)

func TestDayActivity(t *testing.T) {
	t.Run("weights actual counts on one shared scale", func(t *testing.T) {
		levels := DayActivityLevels([]DayActivity{{Events: 10, Thoughts: 100}, {Events: 5}}, 1)
		for i, want := range []DayActivityLevel{{Total: 1, Events: 7.0 / 37, Thoughts: 30.0 / 37}, {Total: 3.5 / 37, Events: 3.5 / 37}} {
			got := levels[i]
			if math.Abs(got.Total-want.Total) > 1e-12 || math.Abs(got.Events-want.Events) > 1e-12 || math.Abs(got.Thoughts-want.Thoughts) > 1e-12 {
				t.Fatalf("bucket %d: got %+v, want %+v", i, got, want)
			}
		}
	})
	t.Run("empty activity stays empty and dimensions are bounded", func(t *testing.T) {
		for _, width := range []int{1, 17, 80} {
			cells := RenderDayActivity(width, 3, []DayActivity{{}, {}}, DayActivityOptions{Total: true, Events: true, Thoughts: true})
			if len(cells) != 3 || len(cells[0]) != width {
				t.Fatal("wrong plot dimensions")
			}
			for _, row := range cells {
				for _, cell := range row {
					if cell.Glyph != ' ' {
						t.Fatal("zero activity invented a mound")
					}
				}
			}
		}
	})
	t.Run("hiding components leaves total geometry unchanged", func(t *testing.T) {
		values := []DayActivity{{Events: 1}, {Thoughts: 10}, {Events: 3, Thoughts: 5}}
		a := RenderDayActivity(40, 3, values, DayActivityOptions{Total: true})
		b := RenderDayActivity(40, 3, values, DayActivityOptions{Total: true, Events: true, Thoughts: true})
		for y := range a {
			for x := range a[y] {
				if a[y][x].Glyph != ' ' && b[y][x].Glyph == ' ' {
					t.Fatal("toggling components removed total activity")
				}
			}
		}
	})
}

func TestDayActivity_Geometry(t *testing.T) {
	t.Run("equal counts give Events greater per-record weight on the shared scale", func(t *testing.T) {
		cells := RenderDayActivity(4, 3, []DayActivity{{Events: 1}, {Thoughts: 1}}, DayActivityOptions{Total: true, Exponent: 1, Size: 1})
		for y, want := range [][2]rune{{'⣤', ' '}, {'⣿', '⣀'}, {'⣿', '⣿'}} {
			if cells[y][0].Glyph != want[0] || cells[y][3].Glyph != want[1] {
				t.Fatalf("row %d endpoints: got %q/%q, want %q/%q", y, cells[y][0].Glyph, cells[y][3].Glyph, want[0], want[1])
			}
		}
	})
	t.Run("outline leaves the interior blank", func(t *testing.T) {
		cells := RenderDayActivity(1, 3, []DayActivity{{Events: 1, Thoughts: 1}}, DayActivityOptions{Total: true, Outline: true, Exponent: 1, Size: 1})
		for row, want := range []rune{'⠤', ' ', ' '} {
			if got := cells[row][0].Glyph; got != want {
				t.Fatalf("row %d: got %q, want %q", row, got, want)
			}
		}
	})
}

func TestDayActivity_MountainScaling(t *testing.T) {
	t.Run("default contrast keeps the quieter days below a linear half-height", func(t *testing.T) {
		levels := DayActivityLevels([]DayActivity{{Events: 10}, {Events: 5}, {}}, 0)
		if levels[1].Events >= .5 || levels[1].Events <= 0 || levels[2].Total != 0 {
			t.Fatalf("default valley: got %+v, want a positive value below .5 and a zero final day", levels)
		}
	})
	t.Run("size can grow beyond the old ceiling without clipping a ridge", func(t *testing.T) {
		values := []DayActivity{{Events: 1, Thoughts: 1}}
		top := func(size float64) int {
			for y, row := range RenderDayActivity(1, 12, values, DayActivityOptions{Total: true, Size: size}) {
				if row[0].Glyph != ' ' {
					return y
				}
			}
			return 12
		}
		if got, want := top(3), top(1.25); got >= want {
			t.Fatalf("large mountain starts at row %d, want above old ceiling row %d", got, want)
		}
	})
}

func TestDayActivity_SharedScale(t *testing.T) {
	t.Run("Thought-rich days use the same count scale as Events", func(t *testing.T) {
		levels := DayActivityLevels([]DayActivity{{Events: 10, Thoughts: 100}, {Events: 5}}, 1)
		if got := levels[0]; got.Thoughts <= got.Events || got.Total < got.Thoughts || got.Total < got.Events {
			t.Fatalf("shared peak: got %+v, want Total >= Thoughts > Events", got)
		}
		if got := levels[1].Events; got < .094 || got > .095 {
			t.Fatalf("Event contribution: got %v, want 3.5/37", got)
		}
	})
	t.Run("visible grey crest remains on top when a component coincides with Total", func(t *testing.T) {
		cells := RenderDayActivity(24, 8, []DayActivity{{Events: 10}, {Events: 5, Thoughts: 20}, {Thoughts: 100}}, DayActivityOptions{Total: true, Events: true, Thoughts: true})
		for x := range 24 {
			found := false
			for y := range cells {
				cell := cells[y][x]
				if cell.Glyph != ' ' {
					found = true
					if cell.Series != DayTotal {
						t.Fatalf("column %d top row %d: got series %d, want grey Total", x, y, cell.Series)
					}
					break
				}
			}
			if !found {
				t.Fatalf("column %d lost its Total contour", x)
			}
		}
	})
}
