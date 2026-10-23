package daynav

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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
	m.Resize(80, 30)
	cmd := m.Sync(day, true, false)
	m = m.Update(cmd())
	return m, s
}
func TestModel_DayNavigation(t *testing.T) {
	t.Run("moving within loaded history only updates selection", func(t *testing.T) {
		m, s := fixture(t)
		defer m.Close()
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
			m.Resize(width, 20)
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
