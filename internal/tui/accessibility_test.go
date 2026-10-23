package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestModel_Accessibility(t *testing.T) {
	t.Run("independent drafts preview cancel and save with stale reply protection", func(t *testing.T) {
		m := newMindsetHome(t, 0)
		m, _ = rootUpdate(m, BrowserOptionsEnabledMsg{})
		m, _ = rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 32})
		original := m.events.View()
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "open"})
		if !strings.Contains(m.View().Content, "Accessibility") {
			t.Fatal("missing Accessibility in Options")
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyDown})
		m, _ = rootUpdate(m, enterKey())
		first := m.OptionsState()
		if !first.Accessibility || first.Request != "load" {
			t.Fatalf("got state %+v, want accessibility load", first)
		}
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "loaded", ID: first.ID})
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyRight})
		if !m.OptionsState().ThickerCardBorders || m.OptionsState().HeavierText {
			t.Fatal("border toggle changed text weight")
		}
		if !strings.Contains(ansi.Strip(m.View().Content), "┏") {
			t.Fatal("preview did not use heavy border")
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyDown})
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyRight})
		if !m.OptionsState().HeavierText {
			t.Fatal("text toggle did not change")
		}
		m, _ = rootUpdate(m, escapeKey())
		if !m.options.menu || m.events.View() != original {
			t.Fatal("cancel changed saved event rendering")
		}
		m, _ = rootUpdate(m, enterKey())
		current := m.OptionsState()
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "loaded", ID: current.ID})
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyRight})
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyDown})
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyDown})
		m, _ = rootUpdate(m, enterKey())
		save := m.OptionsState()
		if save.Request != "applyAccessibility" {
			t.Fatalf("got request %q, want save", save.Request)
		}
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "saved", ID: first.ID, ThickerCardBorders: true})
		if !m.OptionsState().Open {
			t.Fatal("stale save closed current page")
		}
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "saved", ID: save.ID, ThickerCardBorders: true})
		if m.options.open || !strings.Contains(ansi.Strip(m.events.View()), "┏") {
			t.Fatal("saved borders did not apply to Events")
		}
	})
	t.Run("startup restores browser appearance while native presentation stays unchanged", func(t *testing.T) {
		m := newMindsetHome(t, 0)
		before := m.events.View()
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "appearance", ThickerCardBorders: true})
		if m.events.View() != before {
			t.Fatal("native renderer accepted browser preferences")
		}
		m, _ = rootUpdate(m, BrowserOptionsEnabledMsg{})
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "appearance", ThickerCardBorders: true})
		if !strings.Contains(ansi.Strip(m.events.View()), "┏") {
			t.Fatal("startup did not restore thick borders")
		}
		if len(strings.Split(m.events.View(), "\n")) != len(strings.Split(before, "\n")) {
			t.Fatal("border weight changed view height")
		}
	})
	t.Run("narrow controls and preview retain their columns", func(t *testing.T) {
		m := newRootTestModel()
		m, _ = rootUpdate(m, BrowserOptionsEnabledMsg{})
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "open"})
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyDown})
		m, _ = rootUpdate(m, enterKey())
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "loaded", ID: m.OptionsState().ID})
		for _, width := range []int{80, 40} {
			m, _ = rootUpdate(m, tea.WindowSizeMsg{Width: width, Height: 24})
			before := strings.Split(ansi.Strip(m.View().Content), "\n")
			m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyRight})
			after := strings.Split(ansi.Strip(m.View().Content), "\n")
			if !strings.ContainsAny(strings.Join(after, "\n"), "┗╰") {
				t.Fatal("sample card is missing its bottom border")
			}
			if len(before) != len(after) {
				t.Fatal("border weight changed preview height")
			}
			for i, row := range after {
				if ansi.StringWidth(row) > width || (strings.ContainsAny(row, "┏┓┗┛┃━╭╮╰╯│─") && ansi.StringWidth(row) != ansi.StringWidth(before[i])) {
					t.Fatalf("row %d changed geometry: %q -> %q", i, before[i], row)
				}
			}
		}
	})
}

// Visual-specific tests enter through the real browser Options menu.
func openVisualOptions(m Model) (Model, tea.Cmd) {
	m, cmd := rootUpdate(m, BrowserOptionsMsg{Action: "open"})
	if m.options.menu {
		return rootUpdate(m, enterKey())
	}
	return m, cmd
}
