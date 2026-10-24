package daynav

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

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

// Read completion is independent of the one-shot feedback timer. Applying its
// reply cancels the timer, so data tests and benchmarks never wait for feedback.
func activityReply(cmd tea.Cmd) tea.Msg {
	return cmd().(tea.BatchMsg)[0]()
}

func fixture(t *testing.T) (Model, *readerStub) {
	t.Helper()
	day := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	s := &readerStub{start: day.AddDate(-1, 0, 0)}
	m := New(t.Context(), "u", s)
	m.now = func() time.Time { return day.Add(12 * time.Hour) }
	m.Resize(80, 30)
	cmd := m.Sync(day, true, false)
	m = m.Update(activityReply(cmd))
	return m, s
}
func TestModel_DayNavigation(t *testing.T) {
	t.Run("moving within loaded history only updates selection", func(t *testing.T) {
		m, s := fixture(t)
		defer m.Close()
		reads := s.reads
		plot := m.plot
		for i := 1; i < 20; i++ {
			if cmd := m.Sync(m.selected.AddDate(0, 0, -1), true, false); cmd != nil {
				t.Fatal("day navigation reread history")
			}
		}
		for i := 1; i < 20; i++ {
			if cmd := m.Sync(m.selected.AddDate(0, 0, 1), true, false); cmd != nil {
				t.Fatal("right navigation reread loaded history")
			}
			if m.plot != plot {
				t.Fatal("moving right changed mountain heights inside the loaded range")
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
		old := activityReply(cmd)
		newer := m.Key("+")
		m = m.Update(activityReply(newer))
		from, until := m.from, m.until
		m = m.Update(old)
		if m.from != from || m.until != until {
			t.Fatal("stale zoom replaced range")
		}
		cmd = m.Key("-")
		msg := activityReply(cmd)
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
			if len(strings.Split(m.View(), "\n")) != 2 {
				t.Fatal("short plot too tall")
			}
		}
	})
}

func TestModel_ReadFailure(t *testing.T) {
	m, _ := fixture(t)
	defer m.Close()
	cmd := m.Key("r")
	reply := activityReply(cmd).(loadedMsg)
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

func TestModel_LoadingFeedback(t *testing.T) {
	t.Run("fast reads keep the accepted plot without flashing a loading label", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		before := m.plot
		cmd := m.Key("r")
		if got := m.Status(); got != "" {
			t.Fatalf("immediate status: got %q, want no loading feedback before the delay", got)
		}
		m = m.Update(activityReply(cmd))
		if m.Status() != "" || m.plot != before {
			t.Fatal("fast completion left loading feedback or changed the accepted plot")
		}
	})
	t.Run("slow reads show feedback once and completion retires it", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		before := m.plot
		batch := m.Key("r")().(tea.BatchMsg)
		delayed := batch[1]() // Exercise the real, cancellable one-shot timer.
		if !Owns(delayed) {
			t.Fatal("feedback did not retain component ownership")
		}
		m = m.Update(delayed)
		if m.Status() != "Loading activity…" || m.plot != before {
			t.Fatal("slow feedback replaced the accepted plot or failed to appear")
		}
		m = m.Update(batch[0]())
		m = m.Update(delayed)
		if m.pending || m.showStatus || m.Status() != "" || batch[1]() != nil {
			t.Fatal("completion left feedback active or allowed a late timer to restore it")
		}
	})
	t.Run("superseded and foreign feedback cannot label the current read", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		old := m.Key("r")().(tea.BatchMsg)
		delayed := loadDelayedMsg{owner: m.owner, request: m.request}
		m = m.Update(delayed)
		current := m.Key("+")
		m = m.Update(delayed)
		m = m.Update(loadDelayedMsg{owner: new(int), request: m.request})
		if !m.pending || m.Status() != "" || old[1]() != nil {
			t.Fatal("retired feedback labeled a newer read or kept its timer alive")
		}
		m = m.Update(activityReply(current))
		if m.pending || m.Status() != "" {
			t.Fatal("the current read did not finish cleanly")
		}
	})
	t.Run("closing and failures clear feedback without discarding accepted data", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		before := m.plot
		batch := m.Key("r")().(tea.BatchMsg)
		delayed := loadDelayedMsg{owner: m.owner, request: m.request}
		m = m.Update(delayed)
		m.Sync(m.selected, false, false)
		m = m.Update(delayed)
		if m.pending || m.showStatus || m.Status() != "" || batch[1]() != nil || m.plot != before {
			t.Fatal("closing left feedback/work active or discarded accepted data")
		}
		read := m.Sync(m.selected, true, false)
		delayed = loadDelayedMsg{owner: m.owner, request: m.request}
		m = m.Update(delayed)
		result := activityReply(read).(loadedMsg)
		result.err = errors.New("private failure details")
		m = m.Update(result)
		m = m.Update(delayed)
		if m.pending || m.showStatus || m.Status() != "Activity unavailable · r retry" || m.plot != before {
			t.Fatal("failure kept loading feedback, exposed diagnostics, or lost accepted data")
		}
	})
	t.Run("a queued viewport pan starts fresh feedback and settles after movement", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		firstDay := m.from.AddDate(0, 0, -1)
		read := m.Sync(firstDay, true, false)
		delayed := loadDelayedMsg{owner: m.owner, request: m.request}
		m = m.Update(delayed)
		if m.Sync(firstDay.AddDate(0, 0, -5), true, false) != nil {
			t.Fatal("pending navigation started a competing read")
		}
		m = m.Update(activityReply(read))
		next := m.Sync(m.selected, true, false)
		if next == nil || m.Status() != "" {
			t.Fatal("queued pan inherited the previous read's loading label")
		}
		m = m.Update(delayed)
		m = m.Update(activityReply(next))
		if m.pending || m.Status() != "" || m.selected.Before(m.from) || !m.selected.Before(m.until) {
			t.Fatal("settled navigation retained loading feedback or the wrong viewport")
		}
	})
}

func TestModel_ActivityOwnership(t *testing.T) {
	t.Run("refresh arriving during a read reloads bounds once", func(t *testing.T) {
		m, s := fixture(t)
		defer m.Close()
		cmd := m.Key("r")
		s.start = s.start.AddDate(-1, 0, 0)
		m.Invalidate()
		m = m.Update(activityReply(cmd))
		next := m.Sync(m.selected, true, false)
		if next == nil {
			t.Fatal("lost pending invalidation")
		}
		m = m.Update(activityReply(next))
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
		if reply := activityReply(cmd); reply != nil {
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
		m = m.Update(activityReply(m.Key("+")))
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
		m = m.Update(activityReply(cmd))
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
		m = m.Update(activityReply(m.Key("+")))
		if got, want := m.first, displaytime.Day(s.start); !got.Equal(want) {
			t.Fatalf("history start: got %v, want %v", got, want)
		}
	})
}

func TestModel_UnlabeledPlot(t *testing.T) {
	t.Run("reserves the baseline before the first reply at the default terminal size", func(t *testing.T) {
		m := New(t.Context(), "u", &readerStub{})
		m.Resize(80, 24)
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		if len(rows) != 2 || rows[0] != strings.Repeat("⣀", 80) || rows[1] != strings.Repeat(" ", 80) {
			t.Fatalf("initial graph: got %q, want two reserved rows with a shared baseline", rows)
		}
	})
	t.Run("moves a highlight block in the gap without changing mountain dots", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		before, oldMarker := m.plot, m.Marker()
		if cmd := m.Sync(m.selected.AddDate(0, 0, -1), true, false); cmd != nil {
			t.Fatal("cached marker movement queried activity")
		}
		rows := strings.Split(m.View(), "\n")
		if m.Marker() == oldMarker || strings.Join(rows[:len(rows)-1], "\n") != before {
			t.Fatal("marker did not move independently of the mountain geometry")
		}
		gap := rows[len(rows)-1]
		if ansi.Strip(gap) != strings.Repeat(" ", 80) {
			t.Fatalf("gap: got %q, want a blank row", ansi.Strip(gap))
		}
		prefix, styled, found := strings.Cut(gap, "\x1b[48;5;62m")
		block, _, reset := strings.Cut(styled, "\x1b[")
		if !found || !reset || ansi.StringWidth(prefix) != m.Marker() || block != " " || strings.Count(gap, "48;5;62") != 1 {
			t.Fatalf("indicator: got %q, want one highlight cell at column %d", gap, m.Marker())
		}
	})
	t.Run("keeps the full mountain canvas free of status text", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		for _, focused := range []bool{false, true} {
			m.SetFocused(focused)
			rows := strings.Split(m.View(), "\n")
			if ansi.Strip(strings.Join(rows[:len(rows)-1], "\n")) != ansi.Strip(m.plot) {
				t.Fatal("date marker changed mountain geometry")
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
		m = m.Update(loadDelayedMsg{owner: m.owner, request: m.request})
		if m.Status() != "Loading activity…" || m.plot != before {
			t.Fatal("refresh lost feedback or accepted curves")
		}
		m = m.Update(activityReply(cmd))
		if m.Status() != "" {
			t.Fatal("completed refresh kept status text")
		}
	})
	t.Run("explicit style controls and read failures remain understandable", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		m.SetFocused(true)
		m.Key("d")
		if !strings.Contains(m.Status(), "normal boost · 100% · filled") {
			t.Fatal("missing style values")
		}
		m.Key("esc")
		m.err = errors.New("private details")
		if got := m.Status(); !strings.Contains(got, "Activity unavailable · r retry") || strings.Contains(got, "private details") {
			t.Fatalf("unsafe or missing error feedback: %q", got)
		}
	})
}

func TestModel_MountainSizing(t *testing.T) {
	t.Run("keeps the earlier compact height even in a fullscreen terminal", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		for _, scenario := range []struct{ terminalRows, plotHeight int }{{24, 2}, {25, 3}, {26, 4}, {27, 5}, {30, 5}, {32, 5}, {40, 5}, {80, 5}} {
			m.Resize(80, scenario.terminalRows)
			if got := m.Height(); got != scenario.plotHeight {
				t.Fatalf("terminal %d rows: got plot height %d, want %d including the date-marker gap", scenario.terminalRows, got, scenario.plotHeight)
			}
		}
	})
	t.Run("uses normal boost and 100 percent as the baseline in the same four-row canvas in small and fullscreen windows", func(t *testing.T) {
		m, s := fixture(t)
		defer m.Close()
		m.Resize(80, 40)
		m.SetFocused(true)
		if got, want := m.Height(), 5; got != want {
			t.Fatalf("default height: got %d, want %d", got, want)
		}
		_, exponent := m.boostPreset()
		if m.size != 100 || exponent != .8 {
			t.Fatalf("default size/exponent: got %d/%g, want 100/0.8", m.size, exponent)
		}
		initial := m.plot
		reads := s.reads
		m.Key("d")
		for range 40 {
			m.Key("down")
		}
		if m.size != 50 || m.Height() != 5 {
			t.Fatal("reducing fill changed the compact canvas")
		}
		for range 60 {
			if m.Key("up") != nil {
				t.Fatal("size adjustment requested data")
			}
		}
		if m.size != 300 || m.Height() != 5 {
			t.Fatalf("dramatic size/height: got %d/%d, want 300/5", m.size, m.Height())
		}
		if s.reads != reads {
			t.Fatal("style adjustment queried history")
		}
		m.Resize(80, 24)
		if m.Height() != 2 {
			t.Fatalf("short-window height: got %d, want 2", m.Height())
		}
		m.Resize(80, 40)
		m.Key("0")
		if m.plot != initial {
			t.Fatal("reset changed the baseline mountain shape")
		}
		if m.size != 100 || m.Height() != 5 || m.outline {
			t.Fatal("reset did not restore the filled mountain default")
		}
	})
	t.Run("uses both horizontal Braille dots for bounded detail", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		m.span = 0 // Set up the coarse all-history sampling case.
		m = m.Update(activityReply(m.load(false)))
		for _, scenario := range []struct{ width, buckets int }{{80, 162}, {160, 256}} {
			m.Resize(scenario.width, 40)
			cmd := m.Sync(m.selected, true, false)
			if cmd == nil {
				cmd = m.Key("r")
			}
			reply := activityReply(cmd).(loadedMsg)
			if got := len(reply.counts); got > scenario.buckets || got-2 <= scenario.width {
				t.Fatalf("width %d: got %d buckets, want more than one sample per cell and at most %d including context", scenario.width, got, scenario.buckets)
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
		m = m.Update(activityReply(m.Key("0")))
		if got := data.CalendarDays(m.from, m.until); got != 31 {
			t.Fatalf("reset range: got %d days, want the default month", got)
		}
	})
}

func TestModel_MountainFocusPalette(t *testing.T) {
	t.Run("Events uses grey while other home focus keeps colors even on one row", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		for _, height := range []int{40, 20} {
			m.Resize(80, height)
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

func TestModel_ViewportContext(t *testing.T) {
	t.Run("refreshes the history scale and reads neighbors without widening the viewport", func(t *testing.T) {
		m, reader := fixture(t)
		defer m.Close()
		from, until, reads := m.from, m.until, reader.reads
		reply := activityReply(m.Key("r")).(loadedMsg)
		bounds := reply.boundaries
		if !bounds[0].Equal(from.AddDate(0, 0, -1)) || !bounds[1].Equal(from) || !bounds[len(bounds)-2].Equal(until) || !bounds[len(bounds)-1].Equal(until.AddDate(0, 0, 1)) {
			t.Fatal("request did not include exactly one neighboring daily bucket per edge")
		}
		m = m.Update(reply)
		if m.from != from || m.until != until || len(m.values) != 31 {
			t.Fatal("neighboring context stretched the visible month")
		}
		if m.before == nil || m.after == nil || m.before.Thoughts != 0 || m.after.Thoughts != 4 {
			t.Fatalf("neighboring samples: got before=%v after=%v, want supplied edge counts", m.before, m.after)
		}
		// This yearly fixture needs two bounded daily history batches on refresh,
		// followed by one viewport read. Panning does not repeat those batches.
		if reader.reads != reads+3 {
			t.Fatalf("refresh count reads: got %d, want %d", reader.reads, reads+3)
		}
	})
}

// Counts are tied to dates, so scrolling cannot change the synthetic data itself.
type datedActivityReader struct {
	first    time.Time
	counts   map[time.Time]data.ActivityCount
	requests [][]time.Time
}

func (s *datedActivityReader) ActivityStart(context.Context, string, time.Time) (time.Time, error) {
	return s.first, nil
}
func (s *datedActivityReader) ActivityCounts(ctx context.Context, _ string, boundaries []time.Time, _ time.Time) ([]data.ActivityCount, error) {
	s.requests = append(s.requests, append([]time.Time(nil), boundaries...))
	counts := make([]data.ActivityCount, len(boundaries)-1)
	for i := range counts {
		for day := boundaries[i]; day.Before(boundaries[i+1]); day = day.AddDate(0, 0, 1) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			counts[i].EventDays += s.counts[day].EventDays
			counts[i].Thoughts += s.counts[day].Thoughts
		}
	}
	return counts, nil
}

func TestModel_ScrollingLandscape(t *testing.T) {
	t.Run("a larger peak entering the window does not rescale shared mountains", func(t *testing.T) {
		for _, outline := range []bool{false, true} {
			t.Run(map[bool]string{false: "filled", true: "outline"}[outline], func(t *testing.T) {
				today := displaytime.Day(time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC))
				reader := &datedActivityReader{first: today.AddDate(0, 0, -80), counts: map[time.Time]data.ActivityCount{}}
				for i := 0; i <= 80; i++ {
					reader.counts[today.AddDate(0, 0, -i)] = data.ActivityCount{EventDays: int64(20 + i%7), Thoughts: int64(140 + i%13)}
				}
				// It is just outside the original viewport and its neighboring sample.
				reader.counts[today.AddDate(0, 0, -32)] = data.ActivityCount{EventDays: 30, Thoughts: 180}
				m := New(t.Context(), "u", reader)
				defer m.Close()
				m.now = func() time.Time { return today.Add(12 * time.Hour) }
				m.Resize(93, 40) // 31 days: exactly three terminal columns per day.
				m = m.Update(activityReply(m.Sync(today, true, false)))
				m.boost, m.outline = 4, outline
				m.SetHighlighted(true)
				before := strings.Split(m.plot, "\n")
				if strings.TrimSpace(ansi.Strip(before[len(before)-2])) == "" {
					t.Fatal("fixture must have mountain detail above the baseline")
				}
				requests := len(reader.requests)
				m = m.Update(activityReply(m.Sync(m.from.AddDate(0, 0, -1), true, false)))
				after := strings.Split(m.plot, "\n")
				for y := range before {
					a, b := []rune(ansi.Strip(before[y])), []rune(ansi.Strip(after[y]))
					// Exclude only the newly exposed three columns and edge interpolation.
					for x := 6; x+3 < len(b); x++ {
						if a[x-3] != b[x] {
							t.Fatalf("row %d column %d: scrolling rescaled an unchanged dated mountain", y, x)
						}
					}
				}
				if got := len(reader.requests) - requests; got != 1 {
					t.Fatalf("pan count reads: got %d, want one viewport read", got)
				}
			})
		}
	})
	t.Run("wide-range buckets keep the same dates when scrolling one day", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		m.Resize(80, 40)
		m.span = 400
		before := activityReply(m.Key("r")).(loadedMsg)
		m = m.Update(before)
		after := activityReply(m.Sync(m.from.AddDate(0, 0, -1), true, false)).(loadedMsg)
		// Compare interior bucket starts, away from either clipped boundary.
		shared := 0
		for _, a := range before.boundaries[2 : len(before.boundaries)-2] {
			for _, b := range after.boundaries[2 : len(after.boundaries)-2] {
				if a.Equal(b) {
					shared++
					break
				}
			}
		}
		if shared < len(before.boundaries)-6 {
			t.Fatalf("shared bucket boundaries: got %d of %d, want only edge changes", shared, len(before.boundaries)-4)
		}
	})
}

func TestModel_HistoryScale(t *testing.T) {
	t.Run("finds a remote peak through bounded batches and keeps it while holding either direction", func(t *testing.T) {
		today := displaytime.Day(time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC))
		reader := &datedActivityReader{first: today.AddDate(0, 0, -700), counts: map[time.Time]data.ActivityCount{}}
		reader.counts[reader.first.AddDate(0, 0, 10)] = data.ActivityCount{EventDays: 30, Thoughts: 180}
		for i := 0; i < 100; i++ {
			reader.counts[today.AddDate(0, 0, -i)] = data.ActivityCount{EventDays: 20, Thoughts: 100}
		}
		m := New(t.Context(), "u", reader)
		defer m.Close()
		m.now = func() time.Time { return today.Add(12 * time.Hour) }
		m.Resize(93, 40)
		m = m.Update(activityReply(m.Sync(today, true, false)))
		if got, want := m.reference, 75.0; math.Abs(got-want) > 1e-12 {
			t.Fatalf("history reference: got %v, want %v", got, want)
		}
		if got := len(reader.requests); got != 4 {
			t.Fatalf("initial count reads: got %d, want three bounded history batches and one viewport", got)
		}
		for _, bounds := range reader.requests {
			if len(bounds) > 257 {
				t.Fatalf("request has %d boundaries, want at most 257", len(bounds))
			}
		}
		scale := m.reference
		for _, direction := range []int{-1, 1} {
			for range 40 {
				selected := m.from.AddDate(0, 0, -1)
				if direction > 0 {
					selected = m.until
				}
				reads := len(reader.requests)
				m = m.Update(activityReply(m.Sync(selected, true, false)))
				if m.reference != scale || len(reader.requests) != reads+1 {
					t.Fatal("holding day navigation recomputed the scale or repeated the history read")
				}
			}
		}
		m.Invalidate()
		reader.counts[reader.first] = data.ActivityCount{Thoughts: 1000}
		m = m.Update(activityReply(m.Sync(m.selected, true, false)))
		if m.reference != 300 {
			t.Fatalf("refreshed reference: got %v, want 300", m.reference)
		}
	})
	t.Run("a failed scale refresh keeps accepted mountains and scale together", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		plot, scale := m.plot, m.reference
		msg := activityReply(m.Key("r")).(loadedMsg)
		msg.reference = 999
		msg.err = errors.New("synthetic failure")
		m = m.Update(msg)
		if m.plot != plot || m.reference != scale {
			t.Fatal("failed refresh replaced part of the accepted landscape")
		}
	})
	t.Run("cancellation stops scale batches before more reads", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		reader := &datedActivityReader{}
		_, err := readHistoryReference(ctx, reader, "u", time.Now().AddDate(-10, 0, 0), time.Now(), 1, time.Now())
		if !errors.Is(err, context.Canceled) || len(reader.requests) != 0 {
			t.Fatalf("canceled scale: got error %v and %d reads, want cancellation without reads", err, len(reader.requests))
		}
	})
	t.Run("bucket anchoring keeps civil spacing through DST and before the epoch", func(t *testing.T) {
		for _, date := range []string{"1969-12-30", "2026-03-07", "2026-11-01"} {
			day, err := time.Parse("2006-01-02", date)
			if err != nil {
				t.Fatal(err)
			}
			day = displaytime.Day(day.Add(12 * time.Hour))
			start := alignedBucketStart(day, 3)
			if calendarDayIndex(start)%3 != 0 || start.After(day) || !day.Before(start.AddDate(0, 0, 3)) {
				t.Fatalf("%s aligned to %v, want enclosing epoch-anchored three-day bucket", date, start)
			}
		}
	})
}

func TestModel_HistoryRefreshReplacement(t *testing.T) {
	t.Run("zoom replacing a pending refresh still reads the changed history scale", func(t *testing.T) {
		today := displaytime.Day(time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC))
		reader := &datedActivityReader{first: today.AddDate(0, 0, -80), counts: map[time.Time]data.ActivityCount{today: {Thoughts: 10}}}
		m := New(t.Context(), "u", reader)
		defer m.Close()
		m.now = func() time.Time { return today.Add(12 * time.Hour) }
		m.Resize(93, 40)
		m = m.Update(activityReply(m.Sync(today, true, false)))
		reader.counts[reader.first] = data.ActivityCount{Thoughts: 1000}
		obsolete := m.Key("r")
		m = m.Update(activityReply(m.Key("+"))) // Both spans still use daily buckets.
		if m.reference != 300 {
			t.Fatalf("replacement reference: got %v, want 300 from refreshed history", m.reference)
		}
		m = m.Update(obsolete())
		if m.reference != 300 {
			t.Fatal("retired history read replaced the accepted scale")
		}
	})
}

func TestModel_ResetZoom(t *testing.T) {
	t.Run("zero restores the default month without resetting style or the selected date", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		from, until, selected := m.from, m.until, m.selected
		m.SetFocused(true)
		m.Key("d")
		m.Key("right")
		m.Key("up")
		m.Key("f")
		m.Key("esc")
		boost, size, outline := m.boost, m.size, m.outline
		m = m.Update(activityReply(m.Key("+")))
		if m.until.Sub(m.from) >= until.Sub(from) {
			t.Fatal("zoom in did not narrow the range")
		}
		m = m.Update(activityReply(m.Key("0")))
		if m.from != from || m.until != until {
			t.Fatalf("reset range: got %v to %v, want %v to %v", m.from, m.until, from, until)
		}
		if m.selected != selected || m.boost != boost || m.size != size || m.outline != outline {
			t.Fatal("zoom reset changed the date or distribution style")
		}
		if strings.Contains(m.Help(), "0 all") || !strings.Contains(m.Help(), "0 reset") {
			t.Fatalf("zoom reset help: got %q", m.Help())
		}
	})
	t.Run("reset uses the selected historical date and clamps a short month", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		selected := displaytime.Day(time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC))
		m = m.Update(activityReply(m.Sync(selected, true, false)))
		m = m.Update(activityReply(m.Key("0")))
		if m.from.Format("2006-01-02") != "2026-02-28" || m.until.Format("2006-01-02") != "2026-04-01" || !m.selected.Equal(selected) {
			t.Fatalf("historical reset: got %v to %v on %v, want February 28 through March 31 inclusive", m.from, m.until, m.selected)
		}
	})
	t.Run("a late zoom result cannot overwrite a reset", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		old := activityReply(m.Key("+"))
		m = m.Update(activityReply(m.Key("0")))
		from, until := m.from, m.until
		m = m.Update(old)
		if m.from != from || m.until != until {
			t.Fatal("retired zoom replaced the reset month")
		}
	})
}

func TestModel_ZoomOutHistory(t *testing.T) {
	t.Run("zooming out can fit history while zero returns to the default month", func(t *testing.T) {
		m, _ := fixture(t)
		defer m.Close()
		from, until := m.from, m.until
		for range 20 {
			m = m.Update(activityReply(m.Key("-")))
			if m.span == 0 {
				break
			}
		}
		if m.span != 0 || data.CalendarDays(m.from, m.until) < 365 {
			t.Fatal("zooming out did not fit the available history")
		}
		m = m.Update(activityReply(m.Key("0")))
		if m.from != from || m.until != until {
			t.Fatal("zero did not reset the fitted history view to the original month")
		}
	})
}

func TestModel_ZoomImmediatelyAfterReset(t *testing.T) {
	m, _ := fixture(t)
	defer m.Close()
	m.span = 0
	m = m.Update(activityReply(m.load(false)))
	reset := m.Key("0")
	m = m.Update(activityReply(m.Key("+")))
	if got := data.CalendarDays(m.from, m.until); got != 24 {
		t.Fatalf("zoom after pending month reset: got %d days, want 24", got)
	}
	from, until := m.from, m.until
	m = m.Update(reset())
	if m.from != from || m.until != until {
		t.Fatal("late reset replaced the newer zoom")
	}
}

func TestModel_ZoomOutShortHistory(t *testing.T) {
	m, reader := fixture(t)
	defer m.Close()
	reader.start = m.selected
	m = m.Update(activityReply(m.Sync(m.selected, true, true)))
	m = m.Update(activityReply(m.Key("+")))
	previous := data.CalendarDays(m.from, m.until)
	for range 3 {
		m = m.Update(activityReply(m.Key("-")))
		days := data.CalendarDays(m.from, m.until)
		if days < previous || days > 31 {
			t.Fatalf("short-history zoom out: got %d days after %d, want a growing view capped at the default month", days, previous)
		}
		previous = days
	}
	if previous != 31 {
		t.Fatalf("zoom-out limit: got %d days, want 31", previous)
	}
}
