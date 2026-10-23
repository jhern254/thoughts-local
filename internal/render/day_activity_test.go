package render

import "testing"

func TestDayActivity(t *testing.T) {
	t.Run("weights normalized series without magnifying hidden curves", func(t *testing.T) {
		values := DayActivityLevels([]DayActivity{{Events: 10, Thoughts: 1000}, {Events: 5}}, 1)
		if got := values[0]; got.Total != 1 || got.Events != .7 || got.Thoughts != .3 {
			t.Fatalf("peak: got %+v, want total 1, events .7, thoughts .3", got)
		}
		if got := values[1]; got.Total != .35 || got.Events != .35 || got.Thoughts != 0 {
			t.Fatalf("second day: got %+v, want .35 event-only activity", got)
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
	t.Run("Event-only peak has greater visible weight than Thought-only peak", func(t *testing.T) {
		for _, scenario := range []struct {
			value DayActivity
			want  []rune
		}{
			{DayActivity{Events: 1}, []rune{' ', '⣶', '⣿'}},
			{DayActivity{Thoughts: 1}, []rune{' ', ' ', '⣿'}},
		} {
			cells := RenderDayActivity(1, 3, []DayActivity{scenario.value}, DayActivityOptions{Total: true, Exponent: 1, Size: 1})
			for row, want := range scenario.want {
				if got := cells[row][0].Glyph; got != want {
					t.Fatalf("row %d: got %q, want %q", row, got, want)
				}
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
		if levels[1].Events >= .35 || levels[1].Events <= 0 || levels[2].Total != 0 {
			t.Fatalf("default valley: got %+v, want a positive value below .35 and a zero final day", levels)
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
