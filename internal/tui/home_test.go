package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/jhern254/go-thoughts/internal/timeline"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
)

// Existing screen tests begin with Subjects selected in the entity strip.
// Home-specific tests use NewModel directly and exercise its real startup.
func newScreenTestModel(ctx context.Context, user *data.User, subjects SubjectService, thoughtService ThoughtService, metrics MetricsService, logger logging.Logger) Model {
	m := NewModel(ctx, user, subjects, thoughtService, metrics, &homeEventStub{}, &homeTimelineStub{}, logger)
	m.entityFocused = true
	m.events.SetFocused(false)
	return m
}

type homeEventStub struct {
	items []data.Event
	user  string
	lists int
}

func (s *homeEventStub) List(_ context.Context, user string, _, _ time.Time) ([]data.Event, error) {
	s.user = user
	s.lists++
	return s.items, nil
}

func (s *homeEventStub) Get(context.Context, string, int64) (*data.Event, error) {
	return &s.items[0], nil
}
func (*homeEventStub) Create(context.Context, string, string, time.Time, *int64) (*data.Event, error) {
	panic("unexpected event create")
}
func (*homeEventStub) End(context.Context, string, int64, int64, time.Time) (*data.Event, error) {
	panic("unexpected event end")
}

type homeTimelineStub struct{}

func (*homeTimelineStub) OpenThoughtsView(context.Context, string, int64) (timeline.ThoughtScope, error) {
	panic("unexpected event expansion")
}
func (*homeTimelineStub) BrowseThoughtsView(context.Context, string, timeline.ThoughtScope, data.ThoughtSummaryViewRequest) (data.ThoughtSummaryViewResult, error) {
	panic("unexpected event thoughts")
}
func (*homeTimelineStub) CountThoughts(context.Context, string, timeline.ThoughtScope) (int64, error) {
	panic("unexpected event count")
}
func (*homeTimelineStub) LatestThought(context.Context, string, int64) (*data.ThoughtSummaryView, error) {
	panic("unexpected event preview")
}
func (*homeTimelineStub) ThoughtCounts(context.Context, string, time.Time, time.Time) ([]data.EventThoughtCountView, error) {
	return nil, nil
}

type homeThoughtStore struct {
	summary      data.ThoughtSummaryView
	subjectTimes []data.SubjectTimeView
}

func (s homeThoughtStore) ListThoughtsInRange(context.Context, string, time.Time, time.Time) ([]data.Thought, error) {
	panic("unexpected full body range")
}
func (s homeThoughtStore) BrowseThoughtsViewInRange(context.Context, string, time.Time, time.Time, data.ThoughtSummaryViewRequest) (data.ThoughtSummaryViewResult, error) {
	return data.ThoughtSummaryViewResult{Items: []data.ThoughtSummaryView{s.summary}}, nil
}
func (s homeThoughtStore) LatestThoughtInRange(context.Context, string, time.Time, time.Time) (*data.ThoughtSummaryView, error) {
	return &s.summary, nil
}
func (s homeThoughtStore) ThoughtCountsByEvent(context.Context, string, time.Time, time.Time, time.Time) ([]data.EventThoughtCountView, error) {
	return []data.EventThoughtCountView{{EventID: 1, Count: 1}}, nil
}

// Home startup contains one data batch and one minute timer. Only the data
// batch is executed here; timer lifecycle is covered with explicit clock ticks.
func startHomeData(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	return runHomeData(t, m, cmd().(tea.BatchMsg)[0])
}
func runHomeData(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, child := range batch {
			m = runHomeData(t, m, child)
		}
		return m
	}
	m, next := rootUpdate(m, msg)
	return runHomeData(t, m, next)
}

func TestModel_EventsHome(t *testing.T) {
	t.Run("Q does nothing before asynchronous home initialization", func(t *testing.T) {
		m := newScreenTestModel(t.Context(), &data.User{UserID: "u"}, &subjectServiceStub{}, thought.NewService(testutils.NewFakeThoughtStore()), &metricsStub{}, logging.Nop())
		m.entityFocused = false
		_, cmd := m.Update(runeKey('q'))
		if cmd != nil {
			t.Fatal("Q should not quit before initialization")
		}
	})
	t.Run("delayed startup does not steal navigation", func(t *testing.T) {
		m := newRootTestModel()
		startup := m.Init()()
		m, _ = rootUpdate(m, enterKey())
		m, cmd := rootUpdate(m, startup)
		if m.screen != screenSubjectList || cmd != nil {
			t.Fatal("startup took over another screen")
		}
	})
	newHome := func(t *testing.T) (Model, *homeEventStub) {
		service := thought.NewService(testutils.NewFakeThoughtStore())
		item, err := service.Create(t.Context(), "home-user", "complete thought detail", nil, time.Now().Add(-time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		events := &homeEventStub{items: []data.Event{{EventID: 1, StartedAt: time.Now().Add(-time.Hour)}}}
		store := homeThoughtStore{summary: data.ThoughtSummaryView{ThoughtID: item.ThoughtID, Preview: "bounded preview", CharacterCount: int64(len([]rune(item.Thought))), ObservedAt: item.ObservedAt}}
		m := NewModel(t.Context(), &data.User{UserID: "home-user"}, &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) { return nil, nil }}, service, &metricsStub{}, events, timeline.NewService(events, store, store), logging.Nop())
		m, cmd := rootUpdate(m, m.Init()())
		m = startHomeData(t, m, cmd)
		return m, events
	}

	t.Run("Up enters the graph from the first actual card and Down restores selection", func(t *testing.T) {
		m, _ := newHome(t)
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyHome}))
		before, day := m.events.View(), m.events.Day()
		m, cmd := rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
		if cmd != nil || m.homeFocus() != homePanelDays || !m.events.Day().Equal(day) || ansi.Strip(m.events.View()) != ansi.Strip(before) {
			t.Fatal("Up changed the selected card instead of entering the graph")
		}
		m, cmd = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
		if cmd != nil || m.homeFocus() != homePanelEvents || m.events.View() != before {
			t.Fatal("Down did not restore the same selected card")
		}
	})
	t.Run("Now buffers navigation to metrics and Up returns in Events", func(t *testing.T) {
		m, events := newHome(t)
		{
			resized, resizeCmd := rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 30})
			m = runHomeData(t, resized, resizeCmd)
		}
		reads := events.lists
		press := func(code rune) {
			t.Helper()
			var cmd tea.Cmd
			m, cmd = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: code}))
			if cmd != nil || m.entityFocused || m.screen != screenEvents || events.lists != reads {
				t.Fatal("navigation started work or changed panel focus")
			}
		}
		preview := m.events.View()
		press(tea.KeyUp)
		if m.events.View() == preview {
			t.Fatal("Up did not reveal the latest card")
		}
		press(tea.KeyDown)
		if m.events.View() != preview {
			t.Fatal("Down did not stop at the original Now preview")
		}
		press(tea.KeyDown)
		if m.events.AtTopLevel() || !strings.Contains(m.events.View(), "↑: back/scroll") {
			t.Fatal("second Down did not open metrics")
		}
		press(tea.KeyUp)
		if !m.events.AtTopLevel() || m.events.View() != preview {
			t.Fatal("Up did not return to Now inside Events")
		}
		press(tea.KeyDown)
		press(tea.KeyEnd)
		if m.events.View() != preview {
			t.Fatal("End did not return to Now")
		}
	})

	t.Run("assigned event badge survives thought detail and disappears on collapse", func(t *testing.T) {
		m, events := newHome(t)
		id := int64(7)
		events.items[0].SubjectID = &id
		m.subjects.service.(*subjectServiceStub).get = func(ctx context.Context, userID string, subjectID int64) (*data.Subject, error) {
			if userID != "home-user" || subjectID != id || ctx.Err() != nil {
				t.Fatalf("got subject lookup (%q, %d, %v), want active home-user lookup for %d", userID, subjectID, ctx.Err(), id)
			}
			return &data.Subject{SubjectID: id, SubjectName: "Event subject"}, nil
		}
		{
			resized, resizeCmd := rootUpdate(m, tea.WindowSizeMsg{Width: 100, Height: 30})
			m = runHomeData(t, resized, resizeCmd)
		}
		m, cmd := rootUpdate(m, enterKey())
		m = runHomeData(t, m, cmd)
		before := m.View().Content
		if !strings.Contains(before, "Event subject") || !strings.Contains(before, "bounded preview") || m.entityFocused {
			t.Fatalf("got expanded root view %q, want event subject and thought preview with Events focus", before)
		}
		m, cmd = rootUpdate(m, enterKey())
		m = runHomeData(t, m, cmd)
		m, cmd = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
		m = runHomeData(t, m, cmd)
		if m.View().Content != before {
			t.Fatal("detail return changed the expanded card or its subject badge")
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
		if strings.Contains(m.View().Content, "Event subject") || !m.events.AtTopLevel() || m.entityFocused {
			t.Fatal("collapse retained the badge or changed Events focus")
		}
	})

	t.Run("marks event picker thoughts without moving the home layout", func(t *testing.T) {
		m, _ := newHome(t)
		m, cmd := rootUpdate(m, enterKey())
		m = runHomeData(t, m, cmd)
		height := m.homeEventsHeight
		m, cmd = rootUpdate(m, runeKey('m'))
		if m.homeEventsHeight != height || !strings.Contains(m.View().Content, "mindset") {
			t.Fatal("pending marking moved or hid the panel")
		}
		m = runHomeData(t, m, cmd)
		if !strings.Contains(ansi.Strip(m.events.View()), " M ") {
			t.Fatalf("event thought badge: got %q", m.events.View())
		}
		m, cmd = rootUpdate(m, enterKey())
		m = runHomeData(t, m, cmd)
		if !strings.Contains(ansi.Strip(m.events.View()), " M ") {
			t.Fatal("event thought detail lost mindset status")
		}
	})

	t.Run("Left from event thought detail restores the expanded event", func(t *testing.T) {
		m, _ := newHome(t)
		m, cmd := rootUpdate(m, enterKey())
		m = runHomeData(t, m, cmd)
		before := m.View().Content
		m, cmd = rootUpdate(m, enterKey())
		m = runHomeData(t, m, cmd)
		if m.events.CanLeave() {
			t.Fatal("thought detail did not open")
		}
		m, cmd = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
		m = runHomeData(t, m, cmd)
		if m.screen != screenEvents || m.entityFocused || m.events.AtTopLevel() || m.View().Content != before {
			t.Fatal("Left lost the originating expanded event, focus, selection or scroll")
		}
	})
	t.Run("curve controls route keys without changing panel or day", func(t *testing.T) {
		m, _ := newHome(t)
		{
			resized, resizeCmd := rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 30})
			m = runHomeData(t, resized, resizeCmd)
		}
		before := m.events.View()
		press := func(code rune) {
			t.Helper()
			var cmd tea.Cmd
			m, cmd = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: code}))
			if cmd != nil {
				t.Fatalf("settings key %q issued a command", code)
			}
		}
		press('d')
		for _, code := range []rune{tea.KeyRight, tea.KeyDown, 'f'} {
			press(code)
		}
		view := ansi.Strip(m.events.View())
		if !strings.Contains(view, "medium boost · 95% size · outline") || !strings.Contains(view, "[0 Reset to default]") || m.entityFocused {
			t.Fatal("root did not route tuning controls")
		}
		press(tea.KeyTab)
		if !m.entityFocused || strings.Contains(m.events.View(), "Distributions:") {
			t.Fatal("panel switch did not close controls")
		}
		press('d')
		if strings.Contains(m.events.View(), "Distributions:") {
			t.Fatal("entity panel key opened Events controls")
		}
		press(tea.KeyTab) // Mindset
		press(tea.KeyTab) // Events
		press('d')
		if !strings.Contains(ansi.Strip(m.events.View()), "medium boost · 95% size · outline") {
			t.Fatal("panel switch lost settings")
		}
		for _, width := range []int{60, 59, 80} {
			{
				resized, resizeCmd := rootUpdate(m, tea.WindowSizeMsg{Width: width, Height: 30})
				m = runHomeData(t, resized, resizeCmd)
			}
		}
		press('0')
		press(tea.KeyEnter)
		if m.events.View() != before || m.entityFocused {
			t.Fatal("reset and close did not restore the original Events view")
		}
		press('d')
		press('f')
		press(tea.KeyEsc)
		m, cmd := rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
		m = runHomeData(t, m, cmd)
		press('d')
		if !strings.Contains(m.events.View(), "outline") {
			t.Fatal("day navigation lost settings")
		}
		press(tea.KeyEsc)
		press(tea.KeyTab)
		m, cmd = rootUpdate(m, enterKey())
		m = runHomeData(t, m, cmd)
		m, cmd = rootUpdate(m, escapeKey())
		m = startHomeData(t, m, cmd)
		if m.entityFocused {
			press(tea.KeyTab)
			press(tea.KeyTab)
		}
		press('d')
		if !strings.Contains(m.events.View(), "outline") {
			t.Fatal("leaving and returning to Events lost session settings")
		}
	})

	t.Run("distribution survives focus resize and Thought detail return", func(t *testing.T) {
		m, _ := newHome(t)
		{
			resized, resizeCmd := rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 30})
			m = runHomeData(t, resized, resizeCmd)
		}
		hasCurve := func(view string) bool {
			return strings.ContainsFunc(view, func(r rune) bool { return unicode.Is(unicode.Braille, r) })
		}
		before := m.events.View()
		if !hasCurve(before) {
			t.Fatal("live Events did not draw accepted counts")
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
		if !m.entityFocused || ansi.Strip(m.events.View()) != ansi.Strip(before) {
			t.Fatal("focus switch moved the timeline")
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
		if m.events.View() != before {
			t.Fatal("returning focus did not restore curve styling")
		}
		for _, width := range []int{59, 60, 40, 80} {
			{
				resized, resizeCmd := rootUpdate(m, tea.WindowSizeMsg{Width: width, Height: 30})
				m = runHomeData(t, resized, resizeCmd)
			}
			if hasCurve(m.events.View()) != (width >= 60) {
				t.Fatalf("width %d: wrong lane visibility through root dispatch", width)
			}
		}
		m, cmd := rootUpdate(m, enterKey())
		m = runHomeData(t, m, cmd)
		expanded := m.events.View()
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
		if !m.entityFocused {
			t.Fatal("Tab did not focus entity panel")
		}
		focusedRows, blurredRows := strings.Split(expanded, "\n"), strings.Split(m.events.View(), "\n")
		if len(focusedRows) != len(blurredRows) {
			t.Fatal("panel switch moved expanded rows")
		}
		for i, row := range focusedRows {
			if ansi.Cut(ansi.Strip(row), 12, 22) != ansi.Cut(ansi.Strip(blurredRows[i]), 12, 22) {
				t.Fatal("panel switch changed thought plot geometry")
			}
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
		if m.events.View() != expanded {
			t.Fatal("focus return changed thought plot highlight")
		}
		if !hasCurve(expanded) || !strings.Contains(expanded, "Newest first") {
			t.Fatal("expansion failed to show its thought length plot")
		}
		m, cmd = rootUpdate(m, enterKey())
		m = runHomeData(t, m, cmd)
		if !strings.Contains(m.events.View(), "complete thought detail") {
			t.Fatal("Thought detail did not open")
		}
		m, cmd = rootUpdate(m, escapeKey())
		m = runHomeData(t, m, cmd)
		if m.events.View() != expanded {
			t.Fatal("Thought detail return changed expanded layout or anchor")
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
		if !hasCurve(m.events.View()) || m.entityFocused || m.screen != screenEvents {
			t.Fatal("collapse lost curve or Events focus")
		}
	})
	t.Run("boots into owned Events data and wraps horizontal navigation both ways", func(t *testing.T) {
		m, events := newHome(t)
		if events.user != "home-user" || !strings.Contains(m.View().Content, "Events") || !strings.HasPrefix(m.View().Content, "Local user: home-user\n\n") {
			t.Fatal("home did not use or display bootstrapped user")
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
		for _, key := range []rune{tea.KeyRight, tea.KeyLeft, 'h', 'l'} {
			before := m.entityStrip("")
			m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: key}))
			if m.entityStrip("") == before {
				t.Fatalf("key %q did not move entity selection", key)
			}
			m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: key}))
			m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: key}))
			if m.entityStrip("") != before {
				t.Fatal("entity navigation did not wrap")
			}
		}
		{
			resized, resizeCmd := rootUpdate(m, tea.WindowSizeMsg{Width: 24, Height: 16})
			m = runHomeData(t, resized, resizeCmd)
		}
		if !strings.Contains(m.View().Content, "Subjects") {
			t.Fatal("selected entity clipped away")
		}
	})
	t.Run("rapid day changes discard obsolete batches and late replies", func(t *testing.T) {
		m, service := newHome(t)
		m, cmd := rootUpdate(m, runeKey('r'))
		batch := cmd().(tea.BatchMsg)
		lateList, lateCounts := batch[0](), batch[1]()
		_, lateLatest := rootUpdate(m, lateList)
		var pending []tea.Cmd
		calls := service.lists
		for i := 0; i < 40; i++ {
			key := tea.KeyLeft
			if i%2 != 0 {
				key = tea.KeyRight
			}
			m, cmd = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: key}))
			pending = append(pending, cmd)
		}
		m, cmd = rootUpdate(m, runeKey('r'))
		m = runHomeData(t, m, cmd)
		before := m.View().Content
		for i := len(pending) - 1; i >= 0; i-- {
			m = runHomeData(t, m, pending[i])
		}
		m, _ = rootUpdate(m, lateList)
		m, _ = rootUpdate(m, lateCounts)
		m = runHomeData(t, m, lateLatest)
		if m.View().Content != before || strings.Contains(before, "Loading timeline") || service.lists != calls+1 {
			t.Fatalf("obsolete batches affected current screen or reads: got %d new reads, want 1", service.lists-calls)
		}
	})
	t.Run("count feedback survives list completion but cannot outlive its request", func(t *testing.T) {
		m, _ := newHome(t)
		m, cmd := rootUpdate(m, runeKey('r'))
		batch := cmd().(tea.BatchMsg)
		m = runHomeData(t, m, batch[0])
		if !strings.Contains(m.View().Content, "1 thought") || strings.Contains(m.View().Content, "Counting thoughts") {
			t.Fatal("refresh flashed count feedback or discarded its known count")
		}
		delayed := batch[2]() // Actual timer; the independent count is deliberately held.
		m, _ = rootUpdate(m, delayed)
		if !strings.Contains(m.View().Content, "1 thought · refreshing…") || strings.Contains(m.View().Content, "Loading timeline") {
			t.Fatal("slow count did not get independent feedback through root dispatch")
		}
		lateCount := batch[1]()
		m, cmd = rootUpdate(m, runeKey('r'))
		m = runHomeData(t, m, cmd)
		before := m.View().Content
		m, _ = rootUpdate(m, delayed)
		m, _ = rootUpdate(m, lateCount)
		if m.View().Content != before || strings.Contains(before, "refreshing") {
			t.Fatal("superseded count or timer changed the completed refresh")
		}
	})
	t.Run("delayed feedback preserves the displayed day and cannot outlive its request or screen", func(t *testing.T) {
		m, service := newHome(t)
		{
			resized, cmd := rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 30})
			m = runHomeData(t, resized, cmd)
		}
		before := m.View().Content
		m, cmd := rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
		if m.View().Content != before {
			t.Fatal("root dispatch blanked the timeline before the list arrived")
		}
		// Obtain the actual one-shot timer result; do not sleep or run the
		// recurring minute timer. Other ownership tests deliver data replies.
		delayed := cmd().(tea.BatchMsg)[2]()
		m, _ = rootUpdate(m, delayed)
		view := m.View().Content
		if !strings.Contains(view, "Loading timeline for") || !strings.Contains(view, "bounded preview") || !strings.Contains(view, displaytime.Format(time.Now(), "January 2, 2006")) {
			t.Fatal("slow read lost the displayed date/cards or failed to show feedback")
		}
		start := displaytime.Day(time.Now()).AddDate(0, 0, -2).Add(9 * time.Hour)
		end, label := start.Add(time.Hour), "Two days earlier"
		service.items = []data.Event{{EventID: 2, StartedAt: start, EndedAt: &end, ActivityType: &label}}
		m, cmd = rootUpdate(m, runeKey('h'))
		m, _ = rootUpdate(m, delayed)
		if strings.Contains(m.View().Content, "Loading timeline") {
			t.Fatal("old timer enabled feedback on a newer request")
		}
		batch := cmd().(tea.BatchMsg)
		m = runHomeData(t, m, batch[0])
		view = m.View().Content
		if !strings.Contains(view, label) || !strings.Contains(view, displaytime.Format(start, "January 2, 2006")) || strings.Contains(view, "Loading timeline") || m.entityFocused {
			t.Fatal("ready list waited for counts/timer or lost its date or calendar focus")
		}
		m = runHomeData(t, m, batch[1])
		m, _ = rootUpdate(m, delayed)
		if strings.Contains(m.View().Content, "Loading timeline") {
			t.Fatal("old timer restored feedback after completion")
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
		m, cmd = rootUpdate(m, enterKey())
		m = runHomeData(t, m, cmd)
		m, cmd = rootUpdate(m, escapeKey())
		m = startHomeData(t, m, cmd)
		before = m.View().Content
		m, _ = rootUpdate(m, delayed)
		if m.View().Content != before {
			t.Fatal("old timer changed a reopened home session")
		}
	})
	t.Run("previous day arrives at its first event in calendar navigation", func(t *testing.T) {
		m, service := newHome(t)
		start := displaytime.Day(time.Now()).AddDate(0, 0, -1).Add(21 * time.Hour)
		end := start.Add(time.Hour)
		service.items = []data.Event{{EventID: 2, StartedAt: start, EndedAt: &end}}
		m, cmd := rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
		m = runHomeData(t, m, cmd)
		view := ansi.Strip(m.View().Content)
		if m.entityFocused || !strings.Contains(view, "09:00 PM                ╭") || strings.Contains(view, "←: collapse") {
			t.Fatalf("got %q, want first event visible in timeline navigation", view)
		}
	})
	t.Run("horizontal keys open thoughts and collapse events without changing days", func(t *testing.T) {
		for _, keys := range [][2]rune{{tea.KeyRight, tea.KeyLeft}, {'l', 'h'}} {
			m, _ := newHome(t)
			m, cmd := rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: keys[0]}))
			m = runHomeData(t, m, cmd)
			if !strings.Contains(m.View().Content, "←: collapse") {
				t.Fatalf("key %q did not expand today's selected event", keys[0])
			}
			expanded := m.View().Content
			collapsed, _ := rootUpdate(m, escapeKey())
			m, cmd = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: keys[0]}))
			m = runHomeData(t, m, cmd)
			if !strings.Contains(m.View().Content, "complete thought detail") {
				t.Fatalf("key %q did not open the selected thought", keys[0])
			}
			// Detail navigation is unchanged: Escape returns to the event list.
			m, _ = rootUpdate(m, escapeKey())
			if m.View().Content != expanded {
				t.Fatal("detail return changed the expanded event or selection")
			}
			m, cmd = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: keys[1]}))
			if cmd != nil || m.View().Content != collapsed.View().Content {
				t.Fatalf("key %q should collapse the event without navigating days", keys[1])
			}
		}
	})
	t.Run("Tab transfers highlighting without losing event or thought selection", func(t *testing.T) {
		m, _ := newHome(t)
		{
			resized, resizeCmd := rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 30})
			m = runHomeData(t, resized, resizeCmd)
		}
		for _, expanded := range []bool{false, true} {
			if expanded {
				var cmd tea.Cmd
				m, cmd = rootUpdate(m, enterKey())
				m = runHomeData(t, m, cmd)
			}
			focused := m.events.View()
			expected := m.events
			expected.SetFocused(true)
			if m.entityFocused || focused != expected.View() {
				t.Fatal("timeline must own focus before rendering home")
			}
			if !strings.Contains(m.View().Content, focused) {
				t.Fatal("home did not render the focused timeline")
			}
			m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
			// Deliver another update without letting root View synchronize a copy.
			{
				resized, resizeCmd := rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 30})
				m = runHomeData(t, resized, resizeCmd)
			}
			if !m.entityFocused || m.events.View() == focused {
				t.Fatal("Tab must blur Events during Update, before root View")
			}
			blurred := m.events.View()
			beforeText := strings.Join(strings.Fields(strings.ReplaceAll(ansi.Strip(focused), "│", " ")), " ")
			blurredText := strings.Join(strings.Fields(strings.ReplaceAll(ansi.Strip(blurred), "│", " ")), " ")
			if focused == blurred || beforeText != blurredText || !strings.Contains(m.View().Content, blurred) {
				t.Fatal("Tab should change calendar highlighting without changing its contents")
			}
			m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
			if m.entityFocused || m.events.View() != focused {
				t.Fatal("Shift+Tab back must restore Events focus before root View")
			}
			if !strings.Contains(m.View().Content, focused) {
				t.Fatal("returning focus did not restore selected event and thought")
			}
		}
	})
	t.Run("returning from entities preserves strip focus until Tab resumes the timeline", func(t *testing.T) {
		for _, entity := range []entityKind{entitySubjects, entityThoughts} {
			t.Run(entity.title(), func(t *testing.T) {
				m, _ := newHome(t)
				m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
				if entity == entityThoughts {
					m, _ = rootUpdate(m, runeKey('h'))
				}
				m, cmd := rootUpdate(m, enterKey())
				m = runHomeData(t, m, cmd)
				if m.screen == screenEvents {
					t.Fatal("Enter did not open the selected entity")
				}
				m, cmd = rootUpdate(m, escapeKey())
				m = startHomeData(t, m, cmd)
				expected := m.events
				expected.SetFocused(false)
				if m.screen != screenEvents || !m.entityFocused || m.events.View() != expected.View() {
					t.Fatal("return must leave Events blurred with the entity strip focused")
				}
				m, cmd = rootUpdate(m, runeKey('l'))
				if cmd != nil || m.selectedEntity == entity {
					t.Fatal("entity strip did not retain horizontal keyboard navigation")
				}
				m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
				m, cmd = rootUpdate(m, runeKey('l'))
				m = runHomeData(t, m, cmd)
				if m.entityFocused || !strings.Contains(m.events.View(), "←: collapse") {
					t.Fatal("Shift+Tab back did not restore timeline keyboard navigation")
				}
			})
		}
	})
	t.Run("restores event detail and rejects another thought component's reply", func(t *testing.T) {
		m, _ := newHome(t)
		m, cmd := rootUpdate(m, enterKey())
		m = runHomeData(t, m, cmd)
		before := m.View().Content
		other := m.thoughts.OpenBrowseThoughtsView(&metricsStub{total: 999})
		reply := other().(tea.BatchMsg)[1]()
		m, _ = rootUpdate(m, reply)
		if m.View().Content != before {
			t.Fatal("foreign thought result changed event picker")
		}
		m, cmd = rootUpdate(m, enterKey())
		m = runHomeData(t, m, cmd)
		if !strings.Contains(m.View().Content, "complete thought detail") {
			t.Fatal("did not open shared detail")
		}
		if !m.View().AltScreen {
			t.Fatal("detail must remain in the alternate screen")
		}
		m, _ = rootUpdate(m, escapeKey())
		if m.View().Content != before {
			t.Fatal("return changed event anchors")
		}
		m, _ = rootUpdate(m, escapeKey())
		if strings.Contains(m.View().Content, "r: refresh event") || !strings.Contains(m.View().Content, "Home/End: first/now") {
			t.Fatal("one Escape from expanded event should return to timeline navigation")
		}
	})
	t.Run("delayed expansion cannot reopen after leaving and returning home", func(t *testing.T) {
		m, _ := newHome(t)
		m, cmd := rootUpdate(m, enterKey())
		old := cmd()
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
		m, cmd = rootUpdate(m, enterKey())
		m = runHomeData(t, m, cmd)
		m, cmd = rootUpdate(m, escapeKey())
		m = startHomeData(t, m, cmd)
		before := m.View().Content
		m, _ = rootUpdate(m, old)
		if m.View().Content != before {
			t.Fatal("old expansion changed reopened home")
		}
	})
}

func (s homeThoughtStore) ThoughtStatsInRange(ctx context.Context, u string, from, until time.Time) (data.ThoughtIntervalStats, error) {
	return data.ThoughtIntervalStats{Count: 1, MaxCharacters: s.summary.CharacterCount}, nil
}

func (*homeTimelineStub) ThoughtStats(context.Context, string, timeline.ThoughtScope) (data.ThoughtIntervalStats, error) {
	panic("unexpected event stats")
}

func (*homeTimelineStub) ListEntriesInRange(context.Context, string, time.Time, time.Time) ([]timeline.Entry, error) {
	return nil, nil
}
func (*homeTimelineStub) OpenThoughtsOutsideEventsView(context.Context, string, time.Time) (timeline.ThoughtScope, error) {
	panic("unexpected thoughts outside events expansion")
}
func (s homeThoughtStore) ListThoughtGroupsOutsideEvents(context.Context, string, time.Time, time.Time, time.Time) ([]data.ThoughtGroupOutsideEvents, error) {
	return nil, nil
}

func (s homeThoughtStore) SubjectTimeInRange(context.Context, string, time.Time, time.Time, time.Time) ([]data.SubjectTimeView, error) {
	return s.subjectTimes, nil
}

func (*homeTimelineStub) SubjectTimeInRange(context.Context, string, time.Time, time.Time, time.Time) ([]data.SubjectTimeView, error) {
	return nil, nil
}

func TestModel_SubjectTimeHomeFocus(t *testing.T) {
	for _, width := range []int{100, 40} {
		t.Run(fmt.Sprintf("%d columns", width), func(t *testing.T) {
			service := thought.NewService(testutils.NewFakeThoughtStore())
			now := time.Now().Truncate(time.Second)
			for i := range 6 {
				item, err := service.Create(t.Context(), "u", fmt.Sprintf("Mindset %d", i+1), nil, now)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := service.SetMindset(t.Context(), "u", item.ThoughtID, true, now); err != nil {
					t.Fatal(err)
				}
			}
			store := homeThoughtStore{}
			for i := range 20 {
				store.subjectTimes = append(store.subjectTimes, data.SubjectTimeView{SubjectID: int64(i + 1), SubjectName: fmt.Sprintf("Subject %02d", i+1), TimeSpentSeconds: int64(3600 - i*60)})
			}
			events := &homeEventStub{items: []data.Event{{EventID: 1, StartedAt: now.Add(-time.Hour)}}}
			m := NewModel(t.Context(), &data.User{UserID: "u"}, &subjectServiceStub{}, service, &metricsStub{}, events, timeline.NewService(events, store, store), logging.Nop())
			// Keep recurring mindset timers out of the data-command walkthrough.
			m.focusHome(homePanelMindset)
			m, cmd := rootUpdate(m, m.Init()())
			m = startHomeData(t, m, cmd)
			m.focusHome(homePanelEvents)
			{
				resized, resizeCmd := rootUpdate(m, tea.WindowSizeMsg{Width: width, Height: 32})
				m = runHomeData(t, resized, resizeCmd)
			}
			press := func(key tea.Key) { t.Helper(); m, _ = rootUpdate(m, tea.KeyPressMsg(key)) }
			press(tea.Key{Code: tea.KeyEnd})
			nowView := m.events.View()
			press(tea.Key{Code: tea.KeyDown})
			if m.events.AtTopLevel() || m.homeFocus() != homePanelEvents {
				t.Fatal("Down did not focus subject time inside Events")
			}
			press(tea.Key{Code: tea.KeyDown})
			press(tea.Key{Code: tea.KeyDown})
			if text := ansi.Strip(m.events.View()); !strings.Contains(text, "Subject 03") || strings.Contains(text, "Subject 01") {
				t.Fatalf("got chart %q, want scrolled subjects", text)
			}
			for _, step := range []struct {
				key   tea.Key
				focus homePanel
			}{
				{tea.Key{Code: tea.KeyTab}, homePanelEntities},
				{tea.Key{Code: tea.KeyTab}, homePanelMindset},
				{tea.Key{Code: tea.KeyTab}, homePanelEvents},
				{tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}, homePanelMindset},
				{tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}, homePanelEntities},
				{tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}, homePanelEvents},
			} {
				press(step.key)
				if m.homeFocus() != step.focus {
					t.Fatalf("focus: got %v, want %v", m.homeFocus(), step.focus)
				}
				if m.events.AtTopLevel() {
					t.Fatal("focus switch left subject time")
				}
				if text := ansi.Strip(m.events.View()); !strings.Contains(text, "Subject 03") || strings.Contains(text, "Subject 01") || !strings.Contains(text, "Now") {
					t.Fatalf("focus switch lost chart scroll or Now: %q", text)
				}
				var chartBorder string
				for _, row := range strings.Split(m.events.View(), "\n") {
					if strings.Contains(ansi.Strip(row), "╭") {
						chartBorder = row
						break
					}
				}
				if chartBorder == "" {
					t.Fatal("missing subject-time card border")
				}
				if got, want := chartBorder != ansi.Strip(chartBorder), step.focus == homePanelEvents; got != want {
					t.Fatalf("chart border highlight: got %v, want %v", got, want)
				}
				var dividers []string
				for _, row := range strings.Split(m.View().Content, "\n") {
					if strings.ReplaceAll(ansi.Strip(row), "┬", "─") == strings.Repeat("─", width) {
						dividers = append(dividers, row)
					}
				}
				if len(dividers) != 1 {
					t.Fatalf("got %d entity dividers, want 1", len(dividers))
				}
				for i, row := range dividers {
					if got, want := row != ansi.Strip(row), step.focus == homePanelEntities; got != want {
						t.Fatalf("divider %d highlight: got %v, want %v", i, got, want)
					}
				}
			}
			for range 3 {
				press(tea.Key{Code: tea.KeyUp})
			}
			if !m.events.AtTopLevel() || m.events.View() != nowView || m.homeFocus() != homePanelEvents {
				t.Fatal("Up did not restore the latest-event/Now stop with Events focus")
			}
		})
	}
}
