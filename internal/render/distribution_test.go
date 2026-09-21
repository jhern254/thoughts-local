package render

import (
	"math"
	"reflect"
	"strconv"
	"testing"
)

func pixels(cells [][]Cell) map[[2]int]bool {
	result := make(map[[2]int]bool)
	bits := [2][4]rune{{1, 2, 4, 64}, {8, 16, 32, 128}}
	for y, row := range cells {
		for x, cell := range row {
			if cell.Glyph == ' ' {
				continue
			}
			for dx := range 2 {
				for dy := range 4 {
					if (cell.Glyph-0x2800)&bits[dx][dy] != 0 {
						result[[2]int{x*2 + dx, y*4 + dy}] = true
					}
				}
			}
		}
	}
	return result
}

func BenchmarkRenderDistributions(b *testing.B) {
	for _, count := range []int{3, 24} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			curves := make([]Distribution, count)
			for i := range curves {
				curves[i] = Distribution{CenterY: float64(i*20) + 15.5, Count: 20}
			}
			b.ReportAllocs()
			for b.Loop() {
				RenderDistributions(10, count*5+4, curves, Options{})
			}
		})
	}
}

func leftEdge(points map[[2]int]bool, y int) int {
	left := 20
	for p := range points {
		if p[1] == y {
			left = min(left, p[0])
		}
	}
	return left
}

func TestRenderDistributions(t *testing.T) {
	t.Run("counts increase both dimensions and saturate at twenty", func(t *testing.T) {
		previousWidth, previousHeight := 0, 0
		for _, count := range []int64{0, 1, 5, 10, 20} {
			points := pixels(RenderDistributions(10, 16, []Distribution{{CenterY: 31.5, Count: count}}, Options{}))
			left, top, bottom := 20, 64, -1
			for p := range points {
				left, top, bottom = min(left, p[0]), min(top, p[1]), max(bottom, p[1])
				if !points[[2]int{p[0], 63 - p[1]}] {
					t.Fatalf("count %d: asymmetric dot at %v", count, p)
				}
			}
			width, height := 20-left, bottom-top+1
			if width < previousWidth || height <= previousHeight {
				t.Fatalf("count %d: dimensions %dx%d did not grow from %dx%d", count, width, height, previousWidth, previousHeight)
			}
			if count == 0 && (width > 4 || height != 12) {
				t.Fatalf("zero mound is %dx%d dots, want shallow and 12 dots tall", width, height)
			}
			if count == 20 && (width != 17 || height != 32) {
				t.Fatalf("maximum mound is %dx%d dots, want 17x32", width, height)
			}
			if !points[[2]int{19, top}] || !points[[2]int{19, bottom}] {
				t.Fatalf("count %d: tails do not return to baseline", count)
			}
			previousWidth, previousHeight = width, height
		}
		maximum := RenderDistributions(10, 16, []Distribution{{CenterY: 31.5, Count: 20}}, Options{})
		if got := RenderDistributions(10, 16, []Distribution{{CenterY: 31.5, Count: 100}}, Options{}); !reflect.DeepEqual(got, maximum) {
			t.Fatal("counts above twenty must have identical geometry")
		}
	})

	t.Run("filled is the default and shares its outer boundary with outline", func(t *testing.T) {
		curves := []Distribution{{CenterY: 31.5, Count: 20}}
		filled := pixels(RenderDistributions(10, 16, curves, Options{}))
		outline := pixels(RenderDistributions(10, 16, curves, Options{Mode: Outline}))
		if len(filled) <= len(outline) {
			t.Fatal("filled mode must add interior dots")
		}
		for p := range outline {
			if !filled[p] {
				t.Fatalf("outline dot %v missing from filled mode", p)
			}
		}
		for y := 16; y < 48; y++ {
			leftFilled, leftOutline := 20, 20
			for x := range 20 {
				if filled[[2]int{x, y}] {
					leftFilled = min(leftFilled, x)
				}
				if outline[[2]int{x, y}] {
					leftOutline = min(leftOutline, x)
				}
			}
			if leftFilled != leftOutline {
				t.Fatalf("row %d: different outer boundaries", y)
			}
			for x := leftFilled; x < 20; x++ {
				if !filled[[2]int{x, y}] {
					t.Fatalf("hole at %d,%d", x, y)
				}
			}
		}
	})

	t.Run("outline uses a thin connected stroke without changing the profile", func(t *testing.T) {
		for _, count := range []int64{0, 1, 5, 10, 20} {
			curves := []Distribution{{CenterY: 31.5, Count: count}}
			filled := pixels(RenderDistributions(10, 16, curves, Options{}))
			outline := pixels(RenderDistributions(10, 16, curves, Options{Mode: Outline}))
			for y := range 64 {
				left := leftEdge(outline, y)
				if left != leftEdge(filled, y) {
					t.Fatalf("count %d row %d: outline moved the boundary", count, y)
				}
				for x := left; x < 20; x++ {
					p := [2]int{x, y}
					if !outline[p] {
						continue
					}
					if !outline[[2]int{x, 63 - y}] {
						t.Fatalf("count %d: asymmetric outline dot at %v", count, p)
					}
					// Extra horizontal dots are justified only by a steep edge:
					// they bridge to the next dot row with diagonal connectivity.
					if x > left && leftEdge(outline, y-1) <= x && leftEdge(outline, y+1) <= x {
						t.Fatalf("count %d: unnecessary stroke thickness at %v", count, p)
					}
				}
			}
		}
	})

	t.Run("separate mounds connect through an unowned baseline", func(t *testing.T) {
		cells := RenderDistributions(10, 24, []Distribution{{CenterY: 15.5}, {CenterY: 79.5, Count: 10}}, Options{Mode: Outline})
		points := pixels(cells)
		for y := 22; y < 68; y++ {
			if !points[[2]int{19, y}] {
				t.Fatalf("baseline missing at dot row %d", y)
			}
		}
		if cells[10][9].CurveIndex != -1 {
			t.Fatal("connecting baseline must be unowned")
		}
		visited := make(map[[2]int]bool)
		queue := [][2]int{{19, 10}}
		visited[queue[0]] = true
		for len(queue) > 0 {
			p := queue[0]
			queue = queue[1:]
			for dx := -1; dx <= 1; dx++ {
				for dy := -1; dy <= 1; dy++ {
					next := [2]int{p[0] + dx, p[1] + dy}
					if points[next] && !visited[next] {
						visited[next] = true
						queue = append(queue, next)
					}
				}
			}
		}
		if len(visited) != len(points) {
			t.Fatalf("connected component has %d dots, want all %d", len(visited), len(points))
		}
	})

	t.Run("strongest contributor owns overlaps and input order resolves ties", func(t *testing.T) {
		for _, mode := range []Mode{Filled, Outline} {
			cells := RenderDistributions(10, 16, []Distribution{{CenterY: 31.5}, {CenterY: 31.5, Count: 20}}, Options{Mode: mode})
			if cells[7][1].CurveIndex != 1 {
				t.Fatal("larger overlapping curve must own its peak")
			}
			tied := RenderDistributions(10, 16, []Distribution{{CenterY: 31.5, Count: 20}, {CenterY: 31.5, Count: 20}}, Options{Mode: mode})
			if tied[7][1].CurveIndex != 0 {
				t.Fatal("first curve must win exact ties")
			}
		}
	})

	t.Run("overlapping tails form a shared valley between distinct peaks", func(t *testing.T) {
		curves := []Distribution{{CenterY: 23.5, Count: 20}, {CenterY: 43.5, Count: 20}}
		for _, mode := range []Mode{Filled, Outline} {
			combined := pixels(RenderDistributions(10, 20, curves, Options{Mode: mode}))
			first := pixels(RenderDistributions(10, 20, curves[:1], Options{Mode: mode}))
			second := pixels(RenderDistributions(10, 20, curves[1:], Options{Mode: mode}))
			valley := leftEdge(combined, 33)
			separate := min(leftEdge(first, 33), leftEdge(second, 33))
			if valley >= separate {
				t.Fatalf("combined valley starts at dot %d, want left of the separate outlines at %d", valley, separate)
			}
			if valley <= leftEdge(combined, 23) || valley <= leftEdge(combined, 43) {
				t.Fatal("separated contributions should retain two peaks with a shallower valley")
			}
		}
	})

	t.Run("nearby peaks merge smoothly without flat clipping", func(t *testing.T) {
		curves := []Distribution{{CenterY: 27.5, Count: 20}, {CenterY: 35.5, Count: 20}}
		for _, mode := range []Mode{Filled, Outline} {
			points := pixels(RenderDistributions(10, 16, curves, Options{Mode: mode}))
			if leftEdge(points, 31) >= leftEdge(points, 27) || leftEdge(points, 31) >= leftEdge(points, 35) {
				t.Fatal("close contributions should form a rounded peak between their centers")
			}
			for p := range points {
				if p[0] < 3 {
					t.Fatalf("mixture exceeds the maximum display amplitude at %v", p)
				}
			}
			coincident := RenderDistributions(10, 16, []Distribution{{CenterY: 31.5, Count: 20}, {CenterY: 31.5, Count: 20}}, Options{Mode: mode})
			single := RenderDistributions(10, 16, []Distribution{{CenterY: 31.5, Count: 20}}, Options{Mode: mode})
			if !reflect.DeepEqual(pixels(coincident), pixels(single)) {
				t.Fatal("coincident peaks should scale uniformly to the same bell, not clip flat")
			}
		}
	})

	t.Run("mixture scaling does not change when its peak moves outside the viewport", func(t *testing.T) {
		curves := []Distribution{{CenterY: 27.5, Count: 20}, {CenterY: 35.5, Count: 20}, {CenterY: 59.5, Count: 10}}
		for _, mode := range []Mode{Filled, Outline} {
			whole := RenderDistributions(10, 20, curves, Options{Mode: mode})
			for offset := 0; offset <= 16; offset++ {
				shifted := append([]Distribution(nil), curves...)
				for i := range shifted {
					shifted[i].CenterY -= float64(offset * 4)
				}
				cropped := RenderDistributions(10, 4, shifted, Options{Mode: mode})
				if !reflect.DeepEqual(whole[offset:offset+4], cropped) {
					t.Fatalf("mixture changed while scrolling to row %d", offset)
				}
			}
		}
	})

	t.Run("clipping keeps geometry and narrow lanes contain their dots", func(t *testing.T) {
		for _, mode := range []Mode{Filled, Outline} {
			for _, count := range []int64{0, 1, 5, 10, 20} {
				whole := RenderDistributions(10, 16, []Distribution{{CenterY: 31.5, Count: count}}, Options{Mode: mode})
				for offset := 0; offset <= 12; offset++ {
					cropped := RenderDistributions(10, 4, []Distribution{{CenterY: 31.5 - float64(offset*4), Count: count}}, Options{Mode: mode})
					if !reflect.DeepEqual(whole[offset:offset+4], cropped) {
						t.Fatalf("clipping changed curve geometry at count %d, row offset %d", count, offset)
					}
				}
			}
			for _, width := range []int{1, 2} {
				cells := RenderDistributions(width, 8, []Distribution{{CenterY: 15.5, Count: 20}}, Options{Mode: mode})
				if len(cells) != 8 || len(cells[0]) != width || len(pixels(cells)) == 0 {
					t.Fatal("narrow lane dimensions or output incorrect")
				}
			}
		}
	})

	t.Run("empty input and nonpositive dimensions are safe", func(t *testing.T) {
		if len(pixels(RenderDistributions(10, 8, nil, Options{}))) != 0 {
			t.Fatal("empty input drew dots")
		}
		for _, size := range [][2]int{{0, 8}, {10, 0}, {-1, 8}, {10, -1}} {
			if len(RenderDistributions(size[0], size[1], nil, Options{})) != 0 {
				t.Fatal("nonpositive size should return no rows")
			}
		}
		zero := RenderDistributions(10, 8, []Distribution{{CenterY: 15.5}}, Options{})
		negative := RenderDistributions(10, 8, []Distribution{{CenterY: 15.5, Count: -1}}, Options{})
		if !reflect.DeepEqual(zero, negative) {
			t.Fatal("negative counts must use the zero mound")
		}
		for _, center := range []float64{1e100, -1e100, math.Inf(1), math.NaN()} {
			if len(pixels(RenderDistributions(10, 8, []Distribution{{CenterY: center, Count: 20}}, Options{}))) != 0 {
				t.Fatalf("offscreen or nonfinite center %v drew dots", center)
			}
		}
	})
}
