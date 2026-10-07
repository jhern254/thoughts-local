package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestModel_Options(t *testing.T) {
	t.Run("plain actions use the entity highlight and follow vertical focus", func(t *testing.T) {
		m := newRootTestModel()
		m, _ = rootUpdate(m, BrowserOptionsEnabledMsg{})
		m, _ = rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 32})
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "open"})
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "loaded", ID: m.OptionsState().ID, Darkness: 70})
		for _, label := range []string{"Choose image", "Background darkness: 70%", "Apply", "Remove background", "Back"} {
			if !strings.Contains(m.View().Content, m.subjects.list.Styles.Title.Render(label)) {
				t.Fatalf("%q does not use the entity highlight", label)
			}
			m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyDown})
		}
		rows := strings.Split(ansi.Strip(m.View().Content), "\n")
		for i, label := range []string{"Apply", "Remove background", "Back"} {
			if got := strings.TrimSpace(rows[20+i]); got != label {
				t.Fatalf("row %d = %q, want %q", 20+i, got, label)
			}
		}
		if strings.Contains(ansi.Strip(m.View().Content), "[") {
			t.Fatal("Options should use plain labels, not bracketed buttons")
		}
	})
	t.Run("browser controls preserve the underlying timeline and reject stale replies", func(t *testing.T) {
		m := newRootTestModel()
		m, _ = rootUpdate(m, BrowserOptionsEnabledMsg{})
		before := m.View().Content
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "open"})
		id := m.OptionsState().ID
		if !strings.Contains(m.View().Content, "Background darkness") {
			t.Fatal("missing TUI appearance controls")
		}
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "loaded", ID: id, Darkness: 70})
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyTab})
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyLeft})
		if got := m.OptionsState().Darkness; got != 65 {
			t.Fatalf("darkness = %d, want 65", got)
		}
		m, _ = rootUpdate(m, runeKey('q'))
		if m.View().Content != before || m.exitPromptOpen {
			t.Fatal("Options changed underlying screen")
		}
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "open"})
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "saved", ID: id, Darkness: 10})
		if !m.OptionsState().Open || m.OptionsState().Request != "load" {
			t.Fatal("stale save affected new page")
		}
	})
	t.Run("preview reserves blank cells inside a literal border", func(t *testing.T) {
		m := newRootTestModel()
		m, _ = rootUpdate(m, BrowserOptionsEnabledMsg{})
		m, _ = rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 32})
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "open"})
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "loaded", ID: m.OptionsState().ID, Darkness: 70})
		rows := strings.Split(ansi.Strip(m.View().Content), "\n")
		if !strings.Contains(rows[7], "╭"+strings.Repeat("─", 58)+"╮") {
			t.Fatalf("preview border: %q", rows[7])
		}
		if !strings.Contains(rows[8], "│"+strings.Repeat(" ", 58)+"│") {
			t.Fatalf("preview interior: %q", rows[8])
		}
	})
	t.Run("busy save blocks duplicate actions and escape until its result", func(t *testing.T) {
		m := newRootTestModel()
		m, _ = rootUpdate(m, BrowserOptionsEnabledMsg{})
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "open"})
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "loaded", ID: m.OptionsState().ID, Darkness: 70})
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyTab})
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyTab})
		m, _ = rootUpdate(m, enterKey())
		id := m.OptionsState().ID
		m, _ = rootUpdate(m, enterKey())
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyEscape})
		if !m.OptionsState().Open || m.OptionsState().ID != id || m.OptionsState().Request != "apply" {
			t.Fatal("save was duplicated or abandoned")
		}
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "failed", ID: id})
		if !strings.Contains(m.View().Content, "Could not load or apply") {
			t.Fatal("missing safe error")
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyEscape})
		if m.OptionsState().Open || m.exitPromptOpen {
			t.Fatal("failed save did not allow safe return")
		}
	})
	t.Run("narrow layout retains every action and short layout omits preview", func(t *testing.T) {
		m := newRootTestModel()
		m, _ = rootUpdate(m, BrowserOptionsEnabledMsg{})
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "open"})
		for _, size := range []tea.WindowSizeMsg{{Width: 40, Height: 30}, {Width: 60, Height: 18}} {
			m, _ = rootUpdate(m, size)
			content := ansi.Strip(m.View().Content)
			for _, label := range []string{"Apply", "Remove background", "Back", "Q/Esc: back"} {
				if !strings.Contains(content, label) {
					t.Fatalf("missing %q at %v", label, size)
				}
			}
			if len(strings.Split(content, "\n")) > size.Height {
				t.Fatalf("Options overflows %v", size)
			}
		}
		if m.OptionsState().Preview.Height != 0 {
			t.Fatal("short terminal should omit preview")
		}
	})
	t.Run("native options explains browser presentation and returns", func(t *testing.T) {
		m := newRootTestModel()
		m, _ = rootUpdate(m, runeKey('l'))
		m, _ = rootUpdate(m, enterKey())
		if !strings.Contains(m.View().Content, "browser mode") {
			t.Fatal("missing native explanation")
		}
		m, cmd := rootUpdate(m, runeKey('q'))
		if cmd != nil || m.screen != screenEvents || m.selectedEntity != entityOptions {
			t.Fatal("native return changed")
		}
	})
}
