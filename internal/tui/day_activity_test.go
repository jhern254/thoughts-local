package tui

import (
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/jhern254/go-thoughts/internal/timeline"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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
		mountain := m.days.View()
		for _, key := range []rune{tea.KeyLeft, tea.KeyRight} {
			m, cmd = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: key}))
			m = runHomeData(t, m, cmd)
			if got := m.days.View(); got != mountain {
				t.Fatal("day navigation or a Mindset reply changed the cached mountain geometry")
			}
		}
	})
}

func TestModel_DayGraphWindowHeight(t *testing.T) {
	t.Run("medium and fullscreen windows keep the same compact graph band", func(t *testing.T) {
		m := newMindsetHome(t, 0)
		for _, height := range []int{26, 32, 40, 60, 26} {
			var cmd tea.Cmd
			m, cmd = rootUpdate(m, tea.WindowSizeMsg{Width: 120, Height: height})
			m = runHomeData(t, m, cmd)
			rows := strings.Split(ansi.Strip(m.View().Content), "\n")
			if !strings.HasPrefix(rows[8], " Events ") {
				t.Fatalf("terminal %d rows: got row 8 %q, want Events below the same two-row graph", height, rows[8])
			}
			if m.days.Height() != 3 {
				t.Fatalf("terminal %d rows: got graph band %d, want two plot rows plus quiet space", height, m.days.Height())
			}
		}
	})
}
