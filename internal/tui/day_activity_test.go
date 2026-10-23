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
