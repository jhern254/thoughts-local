package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestModel_DayActivityNavigation(t *testing.T) {
	t.Run("day focus moves one date and returns to Events without opening a card", func(t *testing.T) {
		m := newMindsetHome(t, 0)
		m, cmd := rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 32})
		m = runHomeData(t, m, cmd)
		day := m.events.Day()
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
		if m.homeFocus() != homePanelDays {
			t.Fatal("Shift+Tab did not focus Days")
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
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
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
			m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
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
