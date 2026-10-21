package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
)

func newMindsetHome(t *testing.T, count int) Model {
	t.Helper()
	service := thought.NewService(testutils.NewFakeThoughtStore())
	for i := range count {
		item, err := service.Create(t.Context(), "u", fmt.Sprintf("Practice %d deliberately 界", i+1), nil, time.Unix(int64(i), 0))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.SetMindset(t.Context(), "u", item.ThoughtID, true, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	m := NewModel(t.Context(), &data.User{UserID: "u"}, &subjectServiceStub{}, service, &metricsStub{}, &homeEventStub{}, &homeTimelineStub{}, logging.Nop())
	// Focus the panel before delivery; timer behavior is tested with explicit ticks
	// in the panel package, while this test executes real data command batches.
	m.focusHome(0)
	m, cmd := rootUpdate(m, m.Init()())
	m = startHomeData(t, m, cmd)
	m.focusHome(1)
	return m
}

func TestModel_MindsetHome(t *testing.T) {
	t.Run("separates the inline mindset header from Events", func(t *testing.T) {
		m := newMindsetHome(t, 0)
		m, _ = rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 32})
		rows := strings.Split(ansi.Strip(m.View().Content), "\n")
		if got, want := rows[2], " Mindset  ..."; got != want {
			t.Fatalf("mindset row: got %q, want %q", got, want)
		}
		if got, want := rows[3], strings.Repeat("─", 80); got != want {
			t.Fatalf("panel separator: got %q, want %q", got, want)
		}
		if !strings.HasPrefix(rows[4], " Events ") {
			t.Fatalf("row below separator: got %q, want Events header", rows[4])
		}
	})
	t.Run("expansion pushes Events down and returning from detail restores selection", func(t *testing.T) {
		m := newMindsetHome(t, 6)
		m, _ = rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 32})
		before := ansi.Strip(m.View().Content)
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
		expanded := ansi.Strip(m.View().Content)
		row := func(text, label string) int {
			for i, line := range strings.Split(text, "\n") {
				if strings.Contains(line, label) {
					return i
				}
			}
			return -1
		}
		if got, want := row(expanded, "Events")-row(before, "Events"), 5; got != want {
			t.Fatalf("Events displacement: got %d, want %d", got, want)
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnd}))
		selected := m.View().Content
		m, cmd := rootUpdate(m, enterKey())
		m = runHomeData(t, m, cmd)
		if m.screen != screenMindsetThought || !strings.Contains(m.View().Content, "Practice 6") {
			t.Fatalf("detail: got screen %v, %q", m.screen, m.View().Content)
		}
		m, _ = rootUpdate(m, escapeKey())
		if m.screen != screenEvents || m.View().Content != selected {
			t.Fatal("return lost panel selection, scroll, or timeline")
		}
		m, _ = rootUpdate(m, escapeKey())
		if m.mindsets.Expanded() || m.exitPromptOpen {
			t.Fatal("Escape must collapse before exiting")
		}
		m, cmd = rootUpdate(m, escapeKey())
		m, _ = rootUpdate(m, cmd()) // Leaving focus schedules rotation; do not run the timer.
		if m.homeFocus() != 1 || m.exitPromptOpen {
			t.Fatal("second Escape must return to Events")
		}
	})
	t.Run("Tab cycles all three panels and reverse Tab returns", func(t *testing.T) {
		m := newMindsetHome(t, 0)
		for _, want := range []int{2, 0, 1} {
			m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
			if got := m.homeFocus(); got != want {
				t.Fatalf("focus: got %d, want %d", got, want)
			}
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
		if m.homeFocus() != 0 {
			t.Fatal("reverse Tab did not focus Mindset")
		}
	})
	t.Run("narrow expansion preserves usable Events height and clips Unicode rows", func(t *testing.T) {
		m := newMindsetHome(t, 6)
		m, _ = rootUpdate(m, tea.WindowSizeMsg{Width: 40, Height: 24})
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
		if got, want := m.homeEventsHeight, 14; got != want {
			t.Fatalf("Events height: got %d, want %d", got, want)
		}
		if strings.Contains(m.events.View(), "Resize terminal") {
			t.Fatal("expanded panel made a usable timeline unusable")
		}
		for _, line := range strings.Split(m.View().Content, "\n") {
			if got := ansi.StringWidth(line); got > 40 {
				t.Fatalf("line width: got %d, want <=40: %q", got, line)
			}
		}
	})
	t.Run("m from panel detail updates status without leaving the thought", func(t *testing.T) {
		m := newMindsetHome(t, 1)
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
		m, cmd := rootUpdate(m, enterKey())
		m = runHomeData(t, m, cmd)
		m, cmd = rootUpdate(m, runeKey('m'))
		blocked, _ := rootUpdate(m, escapeKey())
		if blocked.screen != screenMindsetThought {
			t.Fatal("pending designation allowed back navigation")
		}
		m = runHomeData(t, m, cmd)
		if m.screen != screenMindsetThought || strings.HasPrefix(ansi.Strip(m.thoughts.View()), " M ") {
			t.Fatalf("unmarked detail: got %q", m.thoughts.View())
		}
	})
}
