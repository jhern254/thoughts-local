package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func preview(t *testing.T, args ...string) string {
	t.Helper()
	var output bytes.Buffer
	if err := run(args, &output); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestPreview(t *testing.T) {
	t.Run("distribution comparison shows separated overlapping and merged profiles", func(t *testing.T) {
		for _, mode := range []string{"filled", "outline"} {
			view := preview(t, "-plain", "-scenario", "distributions", "-mode", mode)
			for _, label := range []string{"Separated", "Shared valleys", "Merged peak"} {
				if !strings.Contains(view, label) {
					t.Fatalf("missing comparison %q", label)
				}
			}
			if strings.Contains(view, "Events") || strings.Contains(view, "Now") {
				t.Fatal("renderer comparison should not pretend to be Events")
			}
			rows := strings.Split(strings.TrimSuffix(view, "\n"), "\n")
			if len(rows) != 36 {
				t.Fatalf("got %d rows, want 36", len(rows))
			}
			for _, row := range rows {
				if ansi.StringWidth(row) != 100 {
					t.Fatal("comparison exceeds its width")
				}
			}
			colored := preview(t, "-scenario", "distributions", "-mode", mode)
			if ansi.Strip(colored) != view {
				t.Fatal("comparison colors changed geometry")
			}
		}
	})
	t.Run("filled is default and all modes preserve card and timestamp positions", func(t *testing.T) {
		filled := preview(t, "-plain")
		if explicit := preview(t, "-plain", "-mode", "filled"); filled != explicit {
			t.Fatal("default is not filled")
		}
		outline := preview(t, "-plain", "-mode", "outline")
		if filled == outline {
			t.Fatal("mode did not change the distribution")
		}
		f, o := strings.Split(strings.TrimSuffix(filled, "\n"), "\n"), strings.Split(strings.TrimSuffix(outline, "\n"), "\n")
		if len(f) != 36 {
			t.Fatalf("got %d rows, want 36", len(f))
		}
		for i := range f {
			if ansi.StringWidth(f[i]) != 100 {
				t.Fatalf("row %d has width %d, want 100", i, ansi.StringWidth(f[i]))
			}
			if ansi.Cut(f[i], 0, 12) != ansi.Cut(o[i], 0, 12) || ansi.Cut(f[i], 22, 100) != ansi.Cut(o[i], 22, 100) {
				t.Fatalf("mode moved timeline content on row %d", i)
			}
		}
		for row, text := range map[int]string{10: "09:00 AM", 15: "10:00 AM", 18: "11:00 AM", 21: "11:30 AM", 27: "── Now · 12:30 PM ──"} {
			if !strings.Contains(f[row], text) {
				t.Fatalf("row %d: want %q, got %q", row, text, f[row])
			}
		}
		if strings.Contains(f[13], "10:00 AM") {
			t.Fatal("adjacent events duplicated the shared boundary")
		}
		if !strings.Contains(f[26], "╰") || !strings.Contains(f[27], "Now") {
			t.Fatal("Now must immediately follow the ongoing card")
		}
	})

	t.Run("selection only changes color and leaves the baseline subdued", func(t *testing.T) {
		for _, mode := range []string{"filled", "outline"} {
			selected := preview(t, "-mode", mode, "-selected", "3")
			unselected := preview(t, "-mode", mode, "-selected", "0")
			if ansi.Strip(selected) != ansi.Strip(unselected) {
				t.Fatal("selection changed geometry")
			}
			if !strings.Contains(selected, "\x1b[38;5;62m") || strings.Contains(unselected, "\x1b[38;5;62m") {
				t.Fatal("selected purple not applied correctly")
			}
			if !strings.Contains(selected, "\x1b[38;5;245m") || !strings.Contains(selected, "\x1b[38;5;240m⢸") {
				t.Fatal("gray curves or connecting baseline missing")
			}
			row := strings.Split(selected, "\n")[23]
			if !strings.Contains(ansi.Cut(row, 12, 22), "\x1b[38;5;62m") {
				t.Fatal("selected curve is not purple")
			}
			if plain := preview(t, "-plain", "-mode", mode); strings.Contains(plain, "\x1b") {
				t.Fatal("plain output includes ANSI styling")
			}
		}
	})

	t.Run("scenarios remain bounded at normal and narrow sizes", func(t *testing.T) {
		for _, scenario := range []string{"main", "adjacent", "gapped", "crowded", "empty"} {
			for _, size := range [][2]string{{"100", "36"}, {"60", "28"}} {
				view := preview(t, "-plain", "-scenario", scenario, "-width", size[0], "-height", size[1])
				width, _ := strconv.Atoi(size[0])
				height, _ := strconv.Atoi(size[1])
				rows := strings.Split(strings.TrimSuffix(view, "\n"), "\n")
				if len(rows) != height {
					t.Fatalf("%s: got %d rows, want %d", scenario, len(rows), height)
				}
				for i, row := range rows {
					if got := ansi.StringWidth(row); got != width {
						t.Fatalf("%s row %d: got width %d, want %d", scenario, i, got, width)
					}
				}
				if !strings.Contains(view, "Events") || !strings.Contains(view, "Thoughts   Subjects") {
					t.Fatal("missing main UI shell")
				}
				if scenario == "main" && !strings.Contains(view, "Now · 12:30 PM") {
					t.Fatal("narrow preview lost Now")
				}
				if scenario == "empty" && strings.ContainsAny(view, "╭╰") {
					t.Fatal("empty day has an invented event")
				}
				if scenario == "adjacent" && !strings.Contains(view, "...") {
					t.Fatal("historical ending missing")
				}
				if scenario == "adjacent" && !strings.Contains(view, "←/→: day") {
					t.Fatal("historical help should describe day navigation")
				}
			}
		}
	})

	t.Run("invalid options report errors without producing a partial preview", func(t *testing.T) {
		for _, args := range [][]string{{"-mode", "bad"}, {"-scenario", "bad"}, {"-width", "0"}, {"-height", "0"}, {"-selected", "-1"}, {"unexpected"}} {
			var output bytes.Buffer
			if err := run(args, &output); err == nil || output.Len() != 0 {
				t.Fatalf("args %v: want error and no output", args)
			}
		}
	})

	t.Run("reviewed full UI snapshots", func(t *testing.T) {
		for _, sample := range []struct {
			name string
			args []string
		}{
			{"filled-100x36", []string{"-plain"}},
			{"outline-100x36", []string{"-plain", "-mode", "outline"}},
			{"filled-60x28", []string{"-plain", "-width", "60", "-height", "28"}},
			{"distributions-filled-100x36", []string{"-plain", "-scenario", "distributions", "-selected", "2"}},
			{"distributions-outline-100x36", []string{"-plain", "-scenario", "distributions", "-selected", "2", "-mode", "outline"}},
		} {
			want, err := os.ReadFile(filepath.Join("testdata", sample.name+".txt"))
			if err != nil {
				t.Fatal(err)
			}
			if got := preview(t, sample.args...); got != string(want) {
				t.Fatalf("%s differs from its reviewed snapshot", sample.name)
			}
		}
	})
}
