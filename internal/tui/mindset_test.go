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
		if rows[4] != "" {
			t.Fatalf("row after separator: got %q, want blank", rows[4])
		}
		if !strings.HasPrefix(rows[5], " Events ") {
			t.Fatalf("row after gap: got %q, want Events header", rows[5])
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
		if got, want := row(expanded, "Events")-row(before, "Events"), 4; got != want {
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
		m, cmd = rootUpdate(m, runeKey('q'))
		if cmd != nil || m.View().Content != selected || m.homeFocus() != 0 {
			t.Fatal("Q must leave the focused picker and cursor unchanged")
		}
		m, _ = rootUpdate(m, escapeKey())
		if !m.exitPromptOpen || m.homeFocus() != 0 || !m.mindsets.Expanded() || !strings.Contains(m.View().Content, "Exit app?") {
			t.Fatal("Escape must open the app exit prompt without leaving Mindset")
		}
		m, _ = rootUpdate(m, escapeKey())
		if m.exitPromptOpen || m.View().Content != selected {
			t.Fatal("canceling exit must preserve picker focus, cursor, and scroll")
		}
	})
	t.Run("Left preserves the focused picker and its cursor", func(t *testing.T) {
		for _, count := range []int{0, 3} {
			m := newMindsetHome(t, count)
			m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
			m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnd}))
			day := m.events.Day()
			before := m.View().Content
			for range 2 {
				var cmd tea.Cmd
				m, cmd = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
				if cmd != nil {
					t.Fatal("Left returned a command, want no action")
				}
				if m.homeFocus() != 0 || !m.mindsets.Expanded() || m.View().Content != before || !m.events.Day().Equal(day) {
					t.Fatalf("Left with %d thoughts: got focus %d expanded=%v day=%v, want unchanged expanded Mindset on %v", count, m.homeFocus(), m.mindsets.Expanded(), m.events.Day(), day)
				}
			}
		}
	})
	t.Run("Down from the last thought wraps to the first and opens it", func(t *testing.T) {
		m := newMindsetHome(t, 6)
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnd}))
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
		selected := m.View().Content
		m, cmd := rootUpdate(m, enterKey())
		m = runHomeData(t, m, cmd)
		if m.screen != screenMindsetThought || !strings.Contains(m.View().Content, "Practice 1 deliberately") {
			t.Fatalf("wrapped detail: got %q, want Practice 1", m.View().Content)
		}
		m, _ = rootUpdate(m, escapeKey())
		if m.View().Content != selected {
			t.Fatal("detail return changed wrapped selection or scroll")
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
