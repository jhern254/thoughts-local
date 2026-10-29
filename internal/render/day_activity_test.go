package render

import (
	"math"
	"testing"
)

func TestDayActivity(t *testing.T) {
	t.Run("weights actual counts on one shared scale", func(t *testing.T) {
		levels := DayActivityLevels([]DayActivity{{Events: 10, Thoughts: 100}, {Events: 5}}, 1, 0)
		for i, want := range []DayActivityLevel{{Total: 1, Events: .75 * 7.0 / 37, Thoughts: .75 * 30.0 / 37}, {Total: 3.5 / 37, Events: .75 * 3.5 / 37}} {
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
	t.Run("filled mountains share the baseline while outlines omit it", func(t *testing.T) {
		for _, outline := range []bool{false, true} {
			cells := RenderDayActivity(1, 3, []DayActivity{{Events: 1}}, DayActivityOptions{Total: true, Outline: outline, Baseline: true})
			want := []rune{'⣸', '⣿', '⣿'}
			if outline {
				want = []rune{'⣸', '⠇', ' '}
			}
			for y, glyph := range want {
				if got := cells[y][0].Glyph; got != glyph {
					t.Fatalf("outline=%v row %d: got %q, want %q", outline, y, got, glyph)
				}
			}
		}
		for _, values := range [][]DayActivity{nil, {{}, {}}} {
			cells := RenderDayActivity(2, 2, values, DayActivityOptions{Baseline: true})
			for x := range 2 {
				if cells[0][x].Glyph != ' ' || cells[1][x].Glyph != '⣀' || cells[1][x].Series != DayTotal {
					t.Fatalf("empty activity: got %+v, want a grey baseline without invented peaks", cells)
				}
			}
		}
	})
	t.Run("equal counts give Events greater per-record weight on the shared scale", func(t *testing.T) {
		values := make([]DayActivity, 22)
		values[5], values[16] = DayActivity{Events: 1}, DayActivity{Thoughts: 1}
		cells := RenderDayActivity(44, 12, values, DayActivityOptions{Total: true, Exponent: 1, Size: 1})
		firstRow := func(column int) int {
			for y, row := range cells {
				if row[column].Glyph != ' ' {
					return y
				}
			}
			return len(cells)
		}
		if got, want := firstRow(11), firstRow(33); got >= want {
			t.Fatalf("Event summit row=%d, want higher than Thought summit row=%d", got, want)
		}
	})
	t.Run("outline follows the summit without filling beneath its stroke", func(t *testing.T) {
		cells := RenderDayActivity(1, 3, []DayActivity{{Events: 1, Thoughts: 1}}, DayActivityOptions{Total: true, Outline: true, Exponent: 1, Size: 1})
		for row, want := range []rune{'⣸', '⠇', ' '} {
			if got := cells[row][0].Glyph; got != want {
				t.Fatalf("row %d: got %q, want %q", row, got, want)
			}
		}
	})
}

func TestDayActivity_MountainScaling(t *testing.T) {
	t.Run("normal boost retains the prior mountain slopes", func(t *testing.T) {
		levels := DayActivityLevels([]DayActivity{{Events: 10}, {Events: 5}, {}}, 0, 0)
		if math.Abs(levels[1].Events-.75*math.Pow(.5, .8)) > 1e-12 || levels[2].Total != 0 {
			t.Fatalf("default valley: got %+v, want the normal half-count slope and a zero final day", levels)
		}
	})
	t.Run("size can grow beyond the old ceiling without clipping a ridge", func(t *testing.T) {
		values := []DayActivity{{Events: 1, Thoughts: 1}}
		top := func(size float64) int {
			for y, row := range RenderDayActivity(1, 12, values, DayActivityOptions{Total: true, Size: size, Reference: 2}) {
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
	t.Run("Thought-heavy days leave a quarter-height Total ridge without changing component ratios", func(t *testing.T) {
		level := DayActivityLevels([]DayActivity{{Events: 1, Thoughts: 200}}, 1, 0)[0]
		if level.Thoughts > .75*level.Total || level.Events > .75*level.Total {
			t.Fatalf("component peaks: got %+v, want at least 25%% Total headroom", level)
		}
		if math.Abs(level.Thoughts/level.Events-60/.7) > 1e-10 {
			t.Fatalf("component ratio: got %v, want 60/0.7", level.Thoughts/level.Events)
		}
	})
	t.Run("Thought-rich days use the same count scale as Events", func(t *testing.T) {
		levels := DayActivityLevels([]DayActivity{{Events: 10, Thoughts: 100}, {Events: 5}}, 1, 0)
		if got := levels[0]; got.Thoughts <= got.Events || got.Total < got.Thoughts || got.Total < got.Events {
			t.Fatalf("shared peak: got %+v, want Total >= Thoughts > Events", got)
		}
		if got := levels[1].Events; got < .070 || got > .071 {
			t.Fatalf("Event contribution: got %v, want 0.75 × 3.5/37", got)
		}
	})
	t.Run("grey Total stays above the component mountains when their crests occupy separate rows", func(t *testing.T) {
		cells := RenderDayActivity(24, 12, []DayActivity{{Events: 10, Thoughts: 100}}, DayActivityOptions{Total: true, Events: true, Thoughts: true, Size: 3})
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

func TestDayActivity_SolidLayers(t *testing.T) {
	t.Run("a one-row graph retains component colors when Total is hidden", func(t *testing.T) {
		cells := RenderDayActivity(20, 1, func() []DayActivity {
			values := make([]DayActivity, 24)
			values[5].Thoughts = 100
			values[18].Events = 100
			return values
		}(), DayActivityOptions{Events: true, Thoughts: true, Size: 3})
		thoughts, events := false, false
		for _, cell := range cells[0] {
			thoughts = thoughts || cell.Glyph != ' ' && cell.Series == DayThoughts
			events = events || cell.Glyph != ' ' && cell.Series == DayEvents
		}
		if !thoughts || !events {
			t.Fatalf("compact component colors: got Thoughts=%v Events=%v, want both", thoughts, events)
		}
	})
	t.Run("outline keeps separate connected contours without filling beneath them", func(t *testing.T) {
		cells := RenderDayActivity(8, 12, []DayActivity{{Events: 10, Thoughts: 100}}, DayActivityOptions{Total: true, Events: true, Thoughts: true, Outline: true, Size: 3})
		for x, cell := range cells[11] {
			if cell.Glyph != ' ' {
				t.Fatalf("column %d: outline filled the baseline", x)
			}
		}
	})
	t.Run("foreground mountains fill to the baseline without background stripes", func(t *testing.T) {
		cells := RenderDayActivity(40, 12, []DayActivity{{}, {Events: 10, Thoughts: 100}, {}}, DayActivityOptions{Total: true, Events: true, Thoughts: true})
		// Total is behind Thoughts, which is behind Events. Once a foreground
		// mountain is visible, a background color must not reappear below it.
		rank := func(series DayActivitySeries) int {
			switch series {
			case DayThoughts:
				return 1
			case DayEvents:
				return 2
			default:
				return 0
			}
		}
		foundThoughts, foundEvents := false, false
		for x := range 40 {
			previous := DayTotal
			for y := range cells {
				cell := cells[y][x]
				if cell.Glyph == ' ' {
					continue
				}

				if rank(cell.Series) < rank(previous) {
					t.Fatalf("column %d row %d: background series %d reappeared below %d", x, y, cell.Series, previous)
				}
				previous = cell.Series
				foundThoughts = foundThoughts || cell.Series == DayThoughts
				foundEvents = foundEvents || cell.Series == DayEvents
			}
		}
		if !foundThoughts || !foundEvents {
			t.Fatal("missing visible component mountain")
		}
	})
}

func TestDayActivity_PanShape(t *testing.T) {
	for _, outline := range []bool{false, true} {
		t.Run(map[bool]string{false: "filled", true: "outline"}[outline], func(t *testing.T) {
			a := make([]DayActivity, 31)
			a[23] = DayActivity{Events: 10, Thoughts: 200}
			b := append(append([]DayActivity{}, a[1:]...), DayActivity{})
			options := DayActivityOptions{
				FirstDay: 20000, Days: 31,
				Total: true, Events: true, Thoughts: true,
				Outline: outline, Exponent: 2.4, Size: 9,
			}
			before := RenderDayActivity(80, 12, a, options)
			options.FirstDay++
			after := RenderDayActivity(80, 12, b, options)
			left := func(cells [][]DayActivityCell) int {
				for x := range 80 {
					for y := range cells {
						if cells[y][x].Glyph != ' ' {
							return x
						}
					}
				}
				return -1
			}
			shift := left(before) - left(after)
			if shift <= 0 {
				t.Fatalf("pan shift: got %d columns, want leftward movement", shift)
			}
			for y := range before {
				for x := 0; x+shift < 80; x++ {
					if before[y][x+shift] != after[y][x] {
						t.Fatalf("row %d column %d: panning changed the mountain profile rather than translating it", y, x)
					}
				}
			}
		})
	}
}

func TestDayActivityDayColumn(t *testing.T) {
	t.Run("aligns the selected date with the calendar-anchored peak", func(t *testing.T) {
		for _, scenario := range []struct{ width, days, first, selected, column int }{
			{31, 31, 20000, 20000, 0}, {31, 31, 20000, 20030, 30},
			{31, 31, -100, -90, 10}, {80, 31, 20000, 20023, 61},
			{80, 31, 20001, 20023, 58}, {31, 31, 20000, 19999, 0},
			{31, 31, 20000, 20031, 30},
		} {
			got := DayActivityDayColumn(scenario.width, scenario.days, scenario.first, scenario.selected)
			if got != scenario.column {
				t.Fatalf("date %d from %d: got column %d, want %d", scenario.selected, scenario.first, got, scenario.column)
			}
		}
		values := make([]DayActivity, 31)
		values[23] = DayActivity{Events: 10, Thoughts: 200}
		cells := RenderDayActivity(80, 12, values, DayActivityOptions{
			FirstDay: 20000, Days: 31,
			Total: true, Exponent: 2.4, Size: 9,
		})
		if cells[0][61].Glyph == ' ' {
			t.Fatal("the selected date marker missed its full-height summit")
		}
	})
}

func TestDayActivity_PanBoundaryShape(t *testing.T) {
	for _, outline := range []bool{false, true} {
		t.Run(map[bool]string{false: "filled", true: "outline"}[outline], func(t *testing.T) {
			for _, incoming := range []bool{false, true} {
				t.Run(map[bool]string{false: "visible peak", true: "incoming peak"}[incoming], func(t *testing.T) {
					a := make([]DayActivity, 31)
					peak, empty := DayActivity{Events: 10, Thoughts: 200}, DayActivity{}
					next := empty
					if incoming {
						next = peak
					} else {
						a[30] = peak
					}
					b := append(append([]DayActivity{}, a[1:]...), next)
					options := DayActivityOptions{
						FirstDay: 20001, Days: 31,
						Before: &empty, After: &next,
						Total: true, Events: true, Thoughts: true,
						Outline: outline, Exponent: 2.4, Size: 9,
					}
					before := RenderDayActivity(80, 12, a, options)
					options.FirstDay++
					options.After = &empty
					after := RenderDayActivity(80, 12, b, options)
					// This calendar step translates the grid three terminal columns left.
					for y := range before {
						for x := 0; x+3 < 80; x++ {
							if before[y][x+3] != after[y][x] {
								t.Fatalf("row %d column %d: the right-edge peak changed shape as it entered the view", y, x)
							}
						}
					}
				})
			}
		})
	}
}

func TestDayActivity_FixedReference(t *testing.T) {
	t.Run("the same counts keep their height when a larger peak enters", func(t *testing.T) {
		before := DayActivityLevels([]DayActivity{{Events: 20, Thoughts: 100}}, 2.4, 75)
		after := DayActivityLevels([]DayActivity{{Events: 20, Thoughts: 100}, {Events: 30, Thoughts: 180}}, 2.4, 75)
		if before[0] != after[0] {
			t.Fatalf("unchanged mountain: got %+v, want %+v", after[0], before[0])
		}
		if before[0].Total >= 1 || before[0].Total <= 0 {
			t.Fatal("a smaller historical mountain must remain below the reference summit")
		}
	})
	t.Run("a concurrent larger count cannot overflow the fixed canvas", func(t *testing.T) {
		cells := RenderDayActivity(80, 5, []DayActivity{{Thoughts: 1000}}, DayActivityOptions{Total: true, Reference: 1, Size: 9})
		if len(cells) != 5 || len(cells[0]) != 80 {
			t.Fatal("fixed reference overflowed canvas")
		}
	})
}

func TestDayActivity_AlignedSamples(t *testing.T) {
	for _, outline := range []bool{false, true} {
		t.Run(map[bool]string{false: "filled", true: "outline"}[outline], func(t *testing.T) {
			values := make([]DayActivity, 29)
			for i := range values {
				values[i] = DayActivity{Events: float64(2 + i%5), Thoughts: float64(5 + i%11)}
			}
			options := DayActivityOptions{FirstDay: 20000, Days: 80, SampleFirstDay: 19998, SampleDays: 3, Reference: 10, Total: true, Events: true, Thoughts: true, Outline: outline, Exponent: 2.4, Size: 9}
			before := RenderDayActivity(160, 5, values, options)
			options.FirstDay++
			after := RenderDayActivity(160, 5, values, options)
			for y := range before {
				for x := 1; x+2 < 160; x++ {
					if before[y][x+2] != after[y][x] {
						t.Fatalf("row %d column %d: panning changed the profile of a calendar-aligned bucket", y, x)
					}
				}
			}
		})
	}
}

func TestDayActivity_RelativeSize(t *testing.T) {
	t.Run("normal reaches the old full summit while smaller sizes shrink it", func(t *testing.T) {
		values := []DayActivity{{Events: 1}}
		normal := RenderDayActivity(1, 3, values, DayActivityOptions{Total: true, Outline: true, Size: 1})
		if normal[0][0].Glyph != '⣸' {
			t.Fatalf("normal summit: got %q, want a pointed contour at the top of the compact canvas", normal[0][0].Glyph)
		}
		smaller := RenderDayActivity(1, 3, values, DayActivityOptions{Total: true, Outline: true, Size: .5})
		if smaller[0][0].Glyph != ' ' {
			t.Fatal("half size must lower the normal summit")
		}
	})
	t.Run("above normal raises lower mountains without flattening the reference peak", func(t *testing.T) {
		values := make([]DayActivity, 31)
		values[5].Thoughts, values[22].Thoughts = 10, 100
		normal := RenderDayActivity(31, 3, values, DayActivityOptions{Total: true, Outline: true, Size: 1, Exponent: 1})
		larger := RenderDayActivity(31, 3, values, DayActivityOptions{Total: true, Outline: true, Size: 3, Exponent: 1})
		if normal[0][5].Glyph == larger[0][5].Glyph && normal[1][5].Glyph == larger[1][5].Glyph && normal[2][5].Glyph == larger[2][5].Glyph {
			t.Fatal("increasing size above the baseline did not change the lower mountain")
		}
		if larger[0][22].Glyph == ' ' || larger[0][22].Glyph != normal[0][22].Glyph {
			t.Fatal("size adjustment moved or flattened the reference summit")
		}
	})
}
