package daynav

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
)

type readerStub struct {
	reads int
	start time.Time
}

func (s *readerStub) ActivityStart(context.Context, string, time.Time) (time.Time, error) {
	return s.start, nil
}
func (s *readerStub) ActivityCounts(_ context.Context, _ string, b []time.Time, _ time.Time) ([]data.ActivityCount, error) {
	s.reads++
	out := make([]data.ActivityCount, len(b)-1)
	for i := range out {
		out[i] = data.ActivityCount{EventDays: int64(i % 5), Thoughts: int64(i % 7)}
	}
	return out, nil
}
func fixture(t *testing.T) (Model, *readerStub) {
	t.Helper()
	day := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	s := &readerStub{start: day.AddDate(-1, 0, 0)}
	m := New(t.Context(), "u", s)
	m.now = func() time.Time { return day.Add(12 * time.Hour) }
	m.Resize(80, 30, 1)
	cmd := m.Sync(day, true, false)
	m = m.Update(cmd())
	return m, s
}
func TestModel_DayNavigation(t *testing.T) {
	t.Run("moving within loaded history only updates selection", func(t *testing.T) {
		m, s := fixture(t)
		defer m.Close()
		m = m.Update(m.Key("0")())
		reads := s.reads
		for i := 1; i < 20; i++ {
			if cmd := m.Sync(m.selected.AddDate(0, 0, -1), true, false); cmd != nil {
				t.Fatal("day navigation reread history")
			}
		}
		if s.reads != reads {
			t.Fatal("unexpected reads")
		}
	})
	t.Run("focus toggles preserve geometry and choices", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		height := len(strings.Split(m.View(), "\n"))
		m.SetFocused(true)
		m.Key("2")
		m.SetFocused(false)
		m.SetFocused(true)
		if m.series[1] {
			t.Fatal("blur reset hidden Events")
		}
		if len(strings.Split(m.View(), "\n")) != height {
			t.Fatal("focus shifted cards")
		}
	})
	t.Run("zoom rejects stale data and cancellation retires ownership", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		m.SetFocused(true)
		cmd := m.Key("+")
		old := cmd()
		newer := m.Key("+")
		m = m.Update(newer())
		from, until := m.from, m.until
		m = m.Update(old)
		if m.from != from || m.until != until {
			t.Fatal("stale zoom replaced range")
		}
		cmd = m.Key("-")
		msg := cmd()
		m.Close()
		m = m.Update(msg)
		if m.from != from || m.until != until {
			t.Fatal("closed read applied")
		}
	})
	t.Run("width and short heights stay bounded", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		for _, width := range []int{20, 40, 80} {
			m.Resize(width, 20, 1)
			m.SetFocused(true)
			if len(strings.Split(m.View(), "\n")) != 1 {
				t.Fatal("short plot too tall")
			}
		}
	})
}

func TestModel_ReadFailure(t *testing.T) {
	m, _ := fixture(t)
	defer m.Close()
	cmd := m.Key("r")
	reply := cmd().(loadedMsg)
	reply.err = errors.New("read failed")
	m.from = time.Time{}
	m = m.Update(reply)
	if cmd := m.Sync(m.selected, true, false); cmd != nil {
		t.Fatal("failure started an automatic retry")
	}
	if cmd := m.Key("r"); cmd == nil {
		t.Fatal("explicit retry unavailable")
	}
}

func TestModel_ActivityOwnership(t *testing.T) {
	t.Run("refresh arriving during a read reloads bounds once", func(t *testing.T) {
		m, s := fixture(t)
		defer m.Close()
		cmd := m.Key("r")
		s.start = s.start.AddDate(-1, 0, 0)
		m.Invalidate()
		m = m.Update(cmd())
		next := m.Sync(m.selected, true, false)
		if next == nil {
			t.Fatal("lost pending invalidation")
		}
		m = m.Update(next())
		if next := m.Sync(m.selected, true, false); next != nil {
			t.Fatal("refresh repeated after completion")
		}
	})
	t.Run("closing cancels pending reads without replacing visible data", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		before := m.plot
		cmd := m.Key("r")
		m.Sync(m.selected, false, false)
		if reply := cmd(); reply != nil {
			t.Fatal("canceled read published a result")
		}
		if m.plot != before {
			t.Fatal("closing discarded accepted plot")
		}
	})
	t.Run("style and series controls do not query history", func(t *testing.T) {
		m, s := fixture(t)
		defer m.Close()
		m.SetFocused(true)
		before := s.reads
		for _, key := range []string{"1", "2", "3", "d", "right", "up", "f", "0", "esc"} {
			if cmd := m.Key(key); cmd != nil {
				t.Fatalf("%s returned a read", key)
			}
		}
		if s.reads != before {
			t.Fatal("style reread history")
		}
	})
	t.Run("zoom preserves range until selection leaves it", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		m = m.Update(m.Key("+")())
		from, until := m.from, m.until
		for range 3 {
			if m.Sync(m.selected.AddDate(0, 0, -1), true, false) != nil {
				t.Fatal("in-range selection queried")
			}
		}
		if m.from != from || m.until != until {
			t.Fatal("selection recentered zoom")
		}
		cmd := m.Sync(from.AddDate(0, 0, -1), true, false)
		if cmd == nil {
			t.Fatal("outside selection did not shift range")
		}
		m = m.Update(cmd())
		if !m.from.Equal(from.AddDate(0, 0, -1)) || !m.until.Equal(until.AddDate(0, 0, -1)) {
			t.Fatal("range did not shift by one day")
		}
	})
}

func TestModel_ZoomAfterInvalidation(t *testing.T) {
	t.Run("zoom retains a pending request to refresh the history start", func(t *testing.T) {
		m, s := fixture(t)
		defer m.Close()
		m.Key("r")
		s.start = s.start.AddDate(-1, 0, 0)
		m.Invalidate()
		m = m.Update(m.Key("+")())
		if got, want := m.first, displaytime.Day(s.start); !got.Equal(want) {
			t.Fatalf("history start: got %v, want %v", got, want)
		}
	})
}

func TestModel_UnlabeledPlot(t *testing.T) {
	t.Run("uses the old heading row as space without moving curves or Events", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		for _, focused := range []bool{false, true} {
			m.SetFocused(focused)
			rows := strings.Split(m.View(), "\n")
			if got, want := rows[0], ""; got != want {
				t.Fatalf("row above plot: got %q, want blank", got)
			}
			if got, want := strings.Join(rows[1:], "\n"), m.plot; got != want {
				t.Fatal("removing heading changed curve rows")
			}
			if len(rows) != m.Height() {
				t.Fatalf("view height: got %d, want %d", len(rows), m.Height())
			}
		}
	})
	t.Run("pending refresh retains the plot and shows read feedback", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		before := m.plot
		cmd := m.Key("r")
		rows := strings.Split(m.View(), "\n")
		if rows[0] != "Loading activity…" || strings.Join(rows[1:], "\n") != before {
			t.Fatal("refresh lost feedback or accepted curves")
		}
		m = m.Update(cmd())
		if strings.Split(m.View(), "\n")[0] != "" {
			t.Fatal("completed refresh kept status text")
		}
	})
	t.Run("explicit style controls and read failures remain understandable", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		m.SetFocused(true)
		m.Key("d")
		if !strings.Contains(m.View(), "low boost · 100% · filled") {
			t.Fatal("missing style values")
		}
		m.Key("esc")
		m.err = errors.New("private details")
		if got := m.View(); !strings.Contains(got, "Activity unavailable · r retry") || strings.Contains(got, "private details") {
			t.Fatalf("unsafe or missing error feedback: %q", got)
		}
	})
}

func TestModel_MountainSizing(t *testing.T) {
	t.Run("defaults to the former enlarged low-boost look and keeps tuning inside a fixed canvas", func(t *testing.T) {
		m, s := fixture(t)
		defer m.Close()
		m.Resize(80, 40, 1)
		m.SetFocused(true)
		if got, want := m.Height(), 13; got != want {
			t.Fatalf("default height: got %d, want %d", got, want)
		}
		reads := s.reads
		m.Key("d")
		for range 40 {
			if m.Key("up") != nil {
				t.Fatal("size adjustment requested data")
			}
		}
		if m.size != 300 || m.Height() != 13 {
			t.Fatalf("dramatic size/height: got %d/%d, want 300/13", m.size, m.Height())
		}
		if s.reads != reads {
			t.Fatal("style adjustment queried history")
		}
		m.Resize(80, 24, 1)
		if m.Height() != 1 {
			t.Fatalf("short-window height: got %d, want 1", m.Height())
		}
		m.Resize(80, 40, 1)
		m.Key("0")
		if m.size != 100 || m.Height() != 13 || m.outline {
			t.Fatal("reset did not restore the filled mountain default")
		}
	})
	t.Run("uses both horizontal Braille dots for bounded detail", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		m = m.Update(m.Key("0")())
		for _, scenario := range []struct{ width, buckets int }{{80, 160}, {160, 256}} {
			m.Resize(scenario.width, 40, 1)
			cmd := m.Sync(m.selected, true, false)
			if cmd == nil {
				cmd = m.Key("r")
			}
			reply := cmd().(loadedMsg)
			if got := len(reply.counts); got != scenario.buckets {
				t.Fatalf("width %d: got %d buckets, want %d", scenario.width, got, scenario.buckets)
			}
			m = m.Update(reply)
		}
	})
}

func TestModel_DefaultMountains(t *testing.T) {
	t.Run("opens one calendar month before today and keeps series colors after blur", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		if got := data.CalendarDays(m.from, m.until); got != 31 {
			t.Fatalf("default range: got %d days, want 31", got)
		}
		m.SetHighlighted(true)
		m.SetFocused(true)
		focused := m.plot
		m.SetFocused(false)
		if m.plot != focused {
			t.Fatal("leaving graph focus removed component colors")
		}
		m = m.Update(m.Key("0")())
		if got := data.CalendarDays(m.from, m.until); got < 365 {
			t.Fatalf("all-history range: got %d days, want at least 365", got)
		}
	})
}

func TestModel_MountainFocusPalette(t *testing.T) {
	t.Run("Events uses grey while other home focus keeps colors even on one row", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		for _, height := range []int{40, 20} {
			m.Resize(80, height, 1)
			m.SetHighlighted(false)
			grey := m.plot
			if strings.Contains(grey, "38;5;37") || strings.Contains(grey, "38;5;179") {
				t.Fatal("Events focus retained component colors")
			}
			m.SetHighlighted(true)
			if !strings.Contains(m.plot, "38;5;37") || !strings.Contains(m.plot, "38;5;179") {
				t.Fatalf("height %d lost component colors", height)
			}
			if ansi.Strip(m.plot) != ansi.Strip(grey) {
				t.Fatalf("height %d: palette changed mountain geometry", height)
			}
		}
	})
}

func TestModel_CalendarMonthWindow(t *testing.T) {
	t.Run("left edge is a calendar month ago including short months and empty history", func(t *testing.T) {
		for _, scenario := range []struct{ today, from string }{{"2026-10-23", "2026-09-23"}, {"2026-03-31", "2026-02-28"}, {"2024-03-31", "2024-02-29"}} {
			today, _ := time.Parse("2006-01-02", scenario.today)
			from, until := window(today, today, today, time.Time{}, time.Time{}, -1)
			if from.Format("2006-01-02") != scenario.from || !until.Equal(today.AddDate(0, 0, 1)) {
				t.Fatalf("%s range: got %s through %s, want %s through tomorrow", scenario.today, from, until, scenario.from)
			}
		}
	})
}
