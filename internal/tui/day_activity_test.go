package tui

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/jhern254/go-thoughts/internal/timeline"
)

func TestModel_DayActivityNavigation(t *testing.T) {
	t.Run("day focus moves one date and returns to Events without opening a card", func(t *testing.T) {
		m := newMindsetHome(t, 0)
		m, cmd := rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 32})
		m = runHomeData(t, m, cmd)
		day := m.events.Day()
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyHome}))
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
		if m.homeFocus() != homePanelDays {
			t.Fatal("Up did not focus Days")
		}
		if !strings.Contains(ansi.Strip(m.View().Content), "1 Total · 2 Events · 3 Thoughts") {
			t.Fatal("missing series legend")
		}
		m, cmd = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
		m = runHomeData(t, m, cmd)
		if !m.events.Day().Equal(day.AddDate(0, 0, -1)) || m.homeFocus() != homePanelDays {
			t.Fatal("Left did not move one day while retaining focus")
		}
		m, _ = rootUpdate(m, escapeKey())
		if m.homeFocus() != homePanelEvents || m.exitPromptOpen {
			t.Fatal("Escape did not return to Events")
		}
	})
	t.Run("style keys and toggles preserve accepted day and timeline", func(t *testing.T) {
		m := newMindsetHome(t, 0)
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyHome}))
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
		before := m.events.View()
		day := m.events.Day()
		for _, key := range []rune{'2', 'd', tea.KeyRight, tea.KeyUp, 'f', '0', tea.KeyEsc} {
			var cmd tea.Cmd
			m, cmd = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: key}))
			if cmd != nil {
				t.Fatalf("style key %q started work", key)
			}
		}
		if m.events.View() != before || !m.events.Day().Equal(day) || m.homeFocus() != homePanelDays {
			t.Fatal("tuning changed Events or focus")
		}
	})
}

func TestModel_DayMountainControls(t *testing.T) {
	t.Run("dramatic sizing preserves a usable timeline and reset restores its position", func(t *testing.T) {
		for _, height := range []int{24, 30, 40} {
			m := newMindsetHome(t, 0)
			m, cmd := rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: height})
			m = runHomeData(t, m, cmd)
			m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyHome}))
			m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
			before, day := m.events.View(), m.events.Day()
			m, _ = rootUpdate(m, runeKey('d'))
			for range 40 {
				m, cmd = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
				if cmd != nil {
					t.Fatal("sizing started a data read")
				}
				if m.homeEventsHeight < 14 || len(strings.Split(ansi.Strip(m.View().Content), "\n")) > height {
					t.Fatalf("height %d: sizing starved or overflowed the timeline", height)
				}
			}
			m, _ = rootUpdate(m, runeKey('0'))
			m, _ = rootUpdate(m, escapeKey())
			if !m.events.Day().Equal(day) || m.events.View() != before || m.homeFocus() != homePanelDays {
				t.Fatalf("height %d: reset lost the day, timeline position or graph focus", height)
			}
		}
	})
}

func TestModel_MetricsDayCycle(t *testing.T) {
	t.Run("root day keys retain the metrics card including empty days", func(t *testing.T) {
		m := newMindsetHome(t, 0)
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnd}))
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
		day := m.events.Day()
		for _, step := range []struct {
			key   rune
			delta int
		}{{tea.KeyLeft, -1}, {tea.KeyLeft, -2}, {tea.KeyRight, -1}, {tea.KeyRight, 0}} {
			var cmd tea.Cmd
			m, cmd = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: step.key}))
			if m.homeFocus() != homePanelEvents || m.events.AtTopLevel() {
				t.Fatal("pending navigation left metrics")
			}
			m = runHomeData(t, m, cmd)
			if m.homeFocus() != homePanelEvents || m.events.AtTopLevel() || !m.events.Day().Equal(day.AddDate(0, 0, step.delta)) {
				t.Fatal("completed navigation left the selected day or metrics")
			}
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
		if !m.events.AtTopLevel() {
			t.Fatal("Up did not return from metrics")
		}
	})
}

func TestModel_NowDayCycle(t *testing.T) {
	t.Run("root day keys retain the boundary view and today cannot open a card", func(t *testing.T) {
		m := newMindsetHome(t, 0)
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnd}))
		day := m.events.Day()
		for _, step := range []struct {
			key   rune
			delta int
		}{{tea.KeyLeft, -1}, {tea.KeyRight, 0}, {tea.KeyRight, 0}} {
			moved, cmd := rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: step.key}))
			m = runHomeData(t, moved, cmd)
			if m.homeFocus() != homePanelEvents || !m.events.Day().Equal(day.AddDate(0, 0, step.delta)) || !strings.Contains(m.events.View(), "↑: latest card") || strings.Contains(m.events.View(), "←: collapse") {
				t.Fatalf("key %q lost the boundary view or day: %q", step.key, ansi.Strip(m.events.View()))
			}
		}
	})
}

func TestModel_DayActivityArrowEntry(t *testing.T) {
	t.Run("Up enters the graph and Down returns to Events on the selected day", func(t *testing.T) {
		m := newMindsetHome(t, 0)
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyHome}))
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
		if m.homeFocus() != homePanelDays {
			t.Fatal("Up did not enter the graph")
		}
		day := m.events.Day()
		m, cmd := rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
		m = runHomeData(t, m, cmd)
		if m.homeFocus() != homePanelDays || !m.events.Day().Equal(day.AddDate(0, 0, -1)) {
			t.Fatal("day change lost graph focus")
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
		if m.homeFocus() != homePanelEvents {
			t.Fatal("Down did not return to Events")
		}
	})
}

func TestModel_DistributionColorsOutsideTimeline(t *testing.T) {
	t.Run("Mindset and entity focus retain the graph's highlighted timeline curves", func(t *testing.T) {
		now := time.Now()
		events := &homeEventStub{items: []data.Event{{EventID: 1, StartedAt: now.Add(-time.Hour)}}}
		m := NewModel(
			t.Context(), &data.User{UserID: "u"}, &subjectServiceStub{},
			thought.NewService(testutils.NewFakeThoughtStore()), &metricsStub{}, events,
			timeline.NewService(events, homeThoughtStore{}, homeThoughtStore{}), logging.Nop(),
		)
		m, cmd := rootUpdate(m, m.Init()())
		m = startHomeData(t, m, cmd)
		m, cmd = rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 50})
		m = runHomeData(t, m, cmd)
		m.focusHome(homePanelDays)
		highlighted := m.events.View()
		if !strings.Contains(highlighted, "38;5;62") {
			t.Fatal("fixture has no highlighted distribution")
		}
		for _, panel := range []homePanel{homePanelMindset, homePanelEntities} {
			m.focusHome(panel)
			if got := m.events.View(); got != highlighted {
				t.Fatalf("panel %d muted the timeline distributions", panel)
			}
		}
	})
}

func TestModel_DayMountainHeightDuringMindsetLoad(t *testing.T) {
	t.Run("keeps the graph canvas steady while longer Mindset text arrives", func(t *testing.T) {
		m := newMindsetHomeTexts(t, strings.Repeat("Keep a deliberately long reminder readable. ", 80))
		m, cmd := rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 40})
		m = runHomeData(t, m, cmd)
		reload := m.mindsets.Load(m.events.Day())
		m.resizeHome()
		loadingHeight := m.days.Height()
		m, _ = rootUpdate(m, reload())
		if got := m.days.Height(); got != loadingHeight {
			t.Fatalf("graph height after Mindset reply: got %d, want unchanged %d", got, loadingHeight)
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyHome}))
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
		mountain := ansi.Strip(m.days.View())
		for _, key := range []rune{tea.KeyLeft, tea.KeyRight} {
			m, cmd = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: key}))
			m = runHomeData(t, m, cmd)
			if got := ansi.Strip(m.days.View()); got != mountain {
				t.Fatal("day navigation or a Mindset reply changed the cached mountain geometry")
			}
		}
	})
}

func TestModel_DayGraphWindowHeight(t *testing.T) {
	t.Run("short terminals keep the gap and fourteen Events rows", func(t *testing.T) {
		m := newMindsetHome(t, 0)
		for _, scenario := range []struct{ height, eventRow, band int }{{24, 6, 2}, {25, 7, 3}, {26, 8, 4}, {27, 9, 5}, {29, 9, 5}, {60, 9, 5}} {
			resized, cmd := rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: scenario.height})
			m = runHomeData(t, resized, cmd)
			rows := strings.Split(ansi.Strip(m.View().Content), "\n")
			if !strings.HasPrefix(rows[scenario.eventRow], " Events ") || rows[scenario.eventRow-1] != strings.Repeat(" ", 80) {
				t.Fatalf("terminal %d rows: missing gap or Events on row %d: %q", scenario.height, scenario.eventRow, rows)
			}
			if m.days.Height() != scenario.band || m.homeEventsHeight < 14 || len(rows) > scenario.height {
				t.Fatalf("terminal %d rows: got band=%d Events height=%d screen height=%d", scenario.height, m.days.Height(), m.homeEventsHeight, len(rows))
			}
		}
	})
	t.Run("loading feedback uses the Events heading without replacing accepted mountains", func(t *testing.T) {
		m := newMindsetHome(t, 0)
		m, cmd := rootUpdate(m, tea.WindowSizeMsg{Width: 100, Height: 32})
		m = runHomeData(t, m, cmd)
		before := strings.Split(m.View().Content, "\n")
		load := m.days.Key("r")
		if strings.Contains(m.View().Content, "Loading activity") {
			t.Fatal("a fast refresh flashed feedback before the delay")
		}
		batch := load().(tea.BatchMsg)
		delayed := batch[1]()
		m, _ = rootUpdate(m, delayed)
		rows := strings.Split(m.View().Content, "\n")
		if !strings.HasSuffix(ansi.Strip(rows[9]), "Loading activity…") {
			t.Fatalf("loading heading: got %q", ansi.Strip(rows[9]))
		}
		if strings.Join(rows[4:9], "\n") != strings.Join(before[4:9], "\n") {
			t.Fatal("loading feedback replaced accepted mountain cells")
		}
		m, _ = rootUpdate(m, batch[0]())
		m, _ = rootUpdate(m, delayed)
		if strings.Contains(m.View().Content, "Loading activity") {
			t.Fatal("completed refresh retained loading feedback")
		}
	})
	t.Run("new zoom clears old activity feedback through root dispatch", func(t *testing.T) {
		m := newMindsetHome(t, 0)
		batch := m.days.Key("r")().(tea.BatchMsg)
		delayed := batch[1]()
		m, _ = rootUpdate(m, delayed)
		if !strings.Contains(m.View().Content, "Loading activity") {
			t.Fatal("slow activity feedback did not reach the home layout")
		}
		m, cmd := rootUpdate(m, runeKey('+'))
		m, _ = rootUpdate(m, delayed)
		if strings.Contains(m.View().Content, "Loading activity") {
			t.Fatal("a newer zoom inherited retired loading feedback")
		}
		m = runHomeData(t, m, cmd)
		m, _ = rootUpdate(m, delayed)
		if m.days.Status() != "" || strings.Contains(m.View().Content, "Loading activity") {
			t.Fatal("settled zoom retained or restored activity feedback")
		}
	})
	t.Run("style values sit on the right of Events without occupying mountain rows", func(t *testing.T) {
		m := newMindsetHome(t, 0)
		m, cmd := rootUpdate(m, tea.WindowSizeMsg{Width: 100, Height: 32})
		m = runHomeData(t, m, cmd)
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyHome}))
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
		m, _ = rootUpdate(m, runeKey('d'))
		rows := strings.Split(ansi.Strip(m.View().Content), "\n")
		if !strings.HasPrefix(rows[9], " Events ") || !strings.HasSuffix(rows[9], "normal boost · 100% · filled") || ansi.StringWidth(rows[9]) != 100 {
			t.Fatalf("Events heading: got %q, want date and right-aligned settings on row 9", rows[9])
		}
		for i := 4; i <= 7; i++ {
			if strings.Contains(rows[i], "boost") || strings.Contains(rows[i], "Loading") {
				t.Fatalf("mountain row %d contains status: %q", i, rows[i])
			}
		}
		if rows[8] != strings.Repeat(" ", 100) {
			t.Fatalf("indicator gap: got %q, want blank text with one highlight block", rows[8])
		}
		for _, glyph := range rows[7] {
			if glyph < 0x2800 || glyph > 0x28ff || (glyph-0x2800)&0xc0 != 0xc0 {
				t.Fatalf("shared baseline: got %q, want both bottom Braille dots in every cell", rows[9])
			}
		}
		m, cmd = rootUpdate(m, tea.WindowSizeMsg{Width: 40, Height: 24})
		m = runHomeData(t, m, cmd)
		rows = strings.Split(ansi.Strip(m.View().Content), "\n")
		if !strings.HasPrefix(rows[6], " Events ") || strings.Contains(rows[6], "boost") {
			t.Fatalf("narrow heading: got %q, want date on unchanged row 6", rows[6])
		}
		if !strings.Contains(strings.Join(rows[7:], "\n"), "normal boost · 100% · filled") || len(rows) > 24 {
			t.Fatalf("narrow status must use existing footer: %q", rows)
		}
	})
	t.Run("medium and fullscreen windows keep the same compact graph band", func(t *testing.T) {
		m := newMindsetHome(t, 0)
		for _, height := range []int{29, 32, 40, 60, 29} {
			var cmd tea.Cmd
			m, cmd = rootUpdate(m, tea.WindowSizeMsg{Width: 120, Height: height})
			m = runHomeData(t, m, cmd)
			rows := strings.Split(ansi.Strip(m.View().Content), "\n")
			if !strings.HasPrefix(rows[9], " Events ") {
				t.Fatalf("terminal %d rows: got row 9 %q, want Events below the same four-row graph band", height, rows[9])
			}
			if m.days.Height() != 5 {
				t.Fatalf("terminal %d rows: got graph band %d, want four mountain rows and a marker gap", height, m.days.Height())
			}
		}
	})
}

type dayZoomMetrics struct {
	metricsStub
	first    time.Time
	requests [][]time.Time
}

func (s *dayZoomMetrics) ActivityStart(context.Context, string, time.Time) (time.Time, error) {
	return s.first, nil
}
func (s *dayZoomMetrics) ActivityCounts(_ context.Context, _ string, bounds []time.Time, _ time.Time) ([]data.ActivityCount, error) {
	s.requests = append(s.requests, append([]time.Time(nil), bounds...))
	return make([]data.ActivityCount, len(bounds)-1), nil
}

func TestModel_EventsActivityZoom(t *testing.T) {
	newHome := func(t *testing.T) (Model, *dayZoomMetrics) {
		t.Helper()
		metrics := &dayZoomMetrics{first: time.Now().AddDate(-1, 0, 0)}
		m := NewModel(
			t.Context(), &data.User{UserID: "u"}, &subjectServiceStub{},
			thought.NewService(testutils.NewFakeThoughtStore()), metrics,
			&homeEventStub{}, &homeTimelineStub{}, logging.Nop(),
		)
		m.focusHome(homePanelMindset)
		m, cmd := rootUpdate(m, m.Init()())
		m = startHomeData(t, m, cmd)
		m.focusHome(homePanelEvents)
		return m, metrics
	}
	t.Run("zoom keys work from Events and preserve the timeline and focus", func(t *testing.T) {
		m, metrics := newHome(t)
		day, view := m.events.Day(), m.events.View()
		initial := append([]time.Time(nil), metrics.requests[len(metrics.requests)-1]...)
		for _, key := range []rune{'+', '-', '0'} {
			before := len(metrics.requests)
			var cmd tea.Cmd
			m, cmd = rootUpdate(m, runeKey(key))
			if cmd == nil {
				t.Fatalf("Events zoom key %q returned no command", key)
			}
			m = runHomeData(t, m, cmd)
			if len(metrics.requests) == before {
				t.Fatalf("Events zoom key %q did not load its range", key)
			}
			bounds := metrics.requests[len(metrics.requests)-1]
			if key == '+' && len(bounds) >= len(initial) {
				t.Fatal("zoom in did not narrow the graph")
			}
			if key == '0' && !slices.Equal(bounds, initial) {
				t.Fatalf("reset range: got %v to %v, want original month", bounds[0], bounds[len(bounds)-1])
			}
			if m.homeFocus() != homePanelEvents || !m.events.Day().Equal(day) || m.events.View() != view {
				t.Fatalf("zoom key %q changed Events selection or focus", key)
			}
		}
	})
	t.Run("zero inside Event distribution controls still resets style", func(t *testing.T) {
		m, metrics := newHome(t)
		for _, key := range []tea.KeyPressMsg{runeKey('d'), {Code: tea.KeyRight}, {Code: tea.KeyUp}} {
			m, _ = rootUpdate(m, key)
		}
		before := len(metrics.requests)
		var cmd tea.Cmd
		m, cmd = rootUpdate(m, runeKey('0'))
		if cmd != nil || len(metrics.requests) != before {
			t.Fatal("Event style reset was intercepted as graph zoom")
		}
		if !strings.Contains(ansi.Strip(m.events.View()), "normal boost") || !strings.Contains(ansi.Strip(m.events.View()), "100%") {
			t.Fatal("Event style reset did not restore normal/100%")
		}
	})
	t.Run("zoom keeps the metrics card focused", func(t *testing.T) {
		m, _ := newHome(t)
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyEnd})
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyDown})
		before := m.events.View()
		m, cmd := rootUpdate(m, runeKey('+'))
		if cmd == nil {
			t.Fatal("metrics focus did not forward zoom")
		}
		m = runHomeData(t, m, cmd)
		if m.events.AtTopLevel() || m.homeFocus() != homePanelEvents || m.events.View() != before {
			t.Fatal("zoom changed the metrics selection or focus")
		}
	})
	t.Run("event form input is not consumed by graph zoom", func(t *testing.T) {
		m, metrics := newHome(t)
		m, _ = rootUpdate(m, runeKey('n'))
		before := len(metrics.requests)
		for _, key := range []rune{'0', '+', '-'} {
			m, _ = rootUpdate(m, runeKey(key))
		}
		if !strings.Contains(ansi.Strip(m.events.View()), "0+-") {
			t.Fatal("zoom keys were not delivered as Event form text")
		}
		if !m.events.FormOpen() || len(metrics.requests) != before {
			t.Fatal("editing an Event triggered day-graph zoom")
		}
	})
}

func TestModel_DayMountainCompactLayout(t *testing.T) {
	m := newMindsetHome(t, 0)
	resized, cmd := rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	m = runHomeData(t, resized, cmd)
	assertLayout := func(m Model, mountainRows, terminalRows int) {
		t.Helper()
		rows := strings.Split(m.View().Content, "\n")
		eventRow := 5 + mountainRows // Mindset ends at row 2, separator at 3; marker is separate.
		if m.days.Height() != mountainRows+1 || m.homeEventsHeight < 14 || len(rows) > terminalRows {
			t.Fatalf("band=%d Events=%d rendered=%d, want %d/at least 14/at most %d", m.days.Height(), m.homeEventsHeight, len(rows), mountainRows+1, terminalRows)
		}
		if !strings.HasPrefix(ansi.Strip(rows[eventRow]), " Events ") || !strings.Contains(rows[eventRow-1], "48;5;62") {
			t.Fatalf("Events or marker missing at the separate rows %d/%d", eventRow, eventRow-1)
		}
		if ansi.Strip(rows[eventRow-2]) != strings.Repeat("⣀", m.width) {
			t.Fatal("empty mountain baseline did not stop above marker and Events")
		}
	}
	assertLayout(m, 4, 40)
	for range 16 {
		moved, command := rootUpdate(m, runeKey('+'))
		m = runHomeData(t, moved, command)
	}
	assertLayout(m, 4, 40)
	for _, size := range [][2]int{{40, 24}, {80, 28}, {120, 50}, {80, 40}} {
		resized, command := rootUpdate(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = runHomeData(t, resized, command)
		want := 4
		if size[1] == 24 {
			want = 1
		}
		assertLayout(m, want, size[1])
	}
	moved, command := rootUpdate(m, runeKey('-'))
	m = runHomeData(t, moved, command)
	assertLayout(m, 4, 40)
}
