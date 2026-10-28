package tui

import (
	"github.com/jhern254/go-thoughts/internal/data"
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
		if m.OptionsState().BorderWeight != data.BorderHeavy || m.OptionsState().HeavierText {
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
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyDown})
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyDown})
		m, _ = rootUpdate(m, enterKey())
		save := m.OptionsState()
		if save.Request != "applyAccessibility" {
			t.Fatalf("got request %q, want save", save.Request)
		}
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "saved", ID: first.ID, BorderWeight: data.BorderHeavy})
		if !m.OptionsState().Open {
			t.Fatal("stale save closed current page")
		}
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "saved", ID: save.ID, BorderWeight: data.BorderHeavy})
		if m.options.open || !strings.Contains(ansi.Strip(m.events.View()), "┏") {
			t.Fatal("saved borders did not apply to Events")
		}
	})
	t.Run("startup restores browser appearance while native presentation stays unchanged", func(t *testing.T) {
		m := newMindsetHome(t, 0)
		before := m.events.View()
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "appearance", BorderWeight: data.BorderHeavy})
		if m.events.View() != before {
			t.Fatal("native renderer accepted browser preferences")
		}
		m, _ = rootUpdate(m, BrowserOptionsEnabledMsg{})
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "appearance", BorderWeight: data.BorderHeavy})
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
			if !strings.ContainsAny(strings.Join(after, "\n"), "┗╰█") {
				t.Fatal("sample card is missing its bottom border")
			}
			if len(before) != len(after) {
				t.Fatal("border weight changed preview height")
			}
			for i, row := range after {
				if ansi.StringWidth(row) > width || (strings.ContainsAny(row, "┏┓┗┛┃━╭╮╰╯│─█") && ansi.StringWidth(row) != ansi.StringWidth(before[i])) {
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

func TestModel_StructuralBorders(t *testing.T) {
	for _, scenario := range []struct {
		weight       data.BorderWeight
		corner, line string
	}{
		{data.BorderNormal, "╭", "─"}, {data.BorderHeavy, "┏", "━"},
	} {
		t.Run(scenario.weight.Label(), func(t *testing.T) {
			m := newMindsetHome(t, 0)
			m, _ = rootUpdate(m, BrowserOptionsEnabledMsg{})
			m, _ = rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 32})
			m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "appearance", BorderWeight: scenario.weight})
			if got, want := ansi.Strip(m.homeDivider(false)), strings.Repeat(scenario.line, 80); got != want {
				t.Fatalf("divider got %q, want %q", got, want)
			}
			if !strings.Contains(ansi.Strip(m.events.View()), scenario.corner) {
				t.Fatal("cards did not use selected border")
			}
			m, _ = openVisualOptions(m)
			m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "loaded", ID: m.OptionsState().ID, Darkness: 42})
			if !strings.Contains(ansi.Strip(m.View().Content), scenario.corner) {
				t.Fatal("media preview did not use selected border")
			}
			m.options.focus = optionsFocusAdjust
			m, _ = rootUpdate(m, enterKey())
			if !strings.Contains(ansi.Strip(m.View().Content), scenario.corner) {
				t.Fatal("framing preview did not use selected border")
			}
		})
	}
}

func TestModel_AccessibilityScrolling(t *testing.T) {
	m := newRootTestModel()
	m, _ = rootUpdate(m, BrowserOptionsEnabledMsg{})
	m, _ = rootUpdate(m, tea.WindowSizeMsg{Width: 28, Height: 8})
	m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "open"})
	m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = rootUpdate(m, enterKey())
	m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "loaded", ID: m.OptionsState().ID, BorderWeight: data.BorderNormal, TextSize: data.TextStandard})
	for range 5 {
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Apply") || !strings.Contains(view, "Back") {
		t.Fatalf("actions unreachable: %q", view)
	}
	if len(strings.Split(view, "\n")) > 8 {
		t.Fatal("controls exceeded viewport")
	}
	for _, row := range strings.Split(view, "\n") {
		if ansi.StringWidth(row) > 28 {
			t.Fatalf("row overflow: %q", row)
		}
	}
	m, _ = rootUpdate(m, enterKey())
	if !m.options.menu {
		t.Fatal("Back did not return to Options")
	}
}

func TestModel_AccessibilityMouseAfterResize(t *testing.T) {
	m := newRootTestModel()
	m.options.open, m.options.accessibility = true, true
	m.options.focus = 5
	m.options.accessibilityOffset = 0
	m.width, m.height = 28, 8
	if !strings.Contains(ansi.Strip(m.View().Content), "Back") {
		t.Fatal("selected Back is not visible")
	}
	m, _ = rootUpdate(m, tea.MouseClickMsg{Button: tea.MouseLeft, X: 2, Y: 3})
	if !m.options.menu {
		t.Fatal("clicking visible Back after resize did not close Accessibility")
	}
}

func TestModel_QuarterWindowTimeline(t *testing.T) {
	m := newMindsetHome(t, 0)
	m, _ = rootUpdate(m, tea.WindowSizeMsg{Width: 98, Height: 20})
	view := ansi.Strip(m.View().Content)
	if strings.Contains(view, "Resize terminal") || !strings.Contains(view, "Now ·") || !strings.Contains(view, "Thoughts") {
		t.Fatalf("quarter-window timeline is not usable: %q", view)
	}
	if got := len(strings.Split(view, "\n")); got > 20 {
		t.Fatalf("got %d rows, want at most 20", got)
	}
	m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyUp})
	if strings.Contains(m.View().Content, "Resize terminal") {
		t.Fatal("selection required resizing")
	}
}
