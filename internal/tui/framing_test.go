package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"strings"
	"testing"
)

func TestModel_BackgroundFraming(t *testing.T) {
	open := func(t *testing.T) Model {
		t.Helper()
		m := newRootTestModel()
		m, _ = rootUpdate(m, BrowserOptionsEnabledMsg{})
		m, _ = rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 32})
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "open"})
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "loaded", ID: m.OptionsState().ID, Background: true, Darkness: 70, Framing: data.DefaultBackgroundFraming()})
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyTab})
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyTab})
		m, _ = rootUpdate(m, enterKey())
		if !m.OptionsState().Editing {
			t.Fatal("framing did not open")
		}
		return m
	}
	t.Run("Done keeps draft framing and Cancel restores entry values", func(t *testing.T) {
		m := open(t)
		m, _ = rootUpdate(m, runeKey('+'))
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyRight})
		m, _ = rootUpdate(m, enterKey())
		if m.OptionsState().Editing || !m.OptionsState().Open || m.OptionsState().Framing.Zoom != 105 || m.OptionsState().Framing.PositionX != 4900 {
			t.Fatalf("done state = %+v", m.OptionsState())
		}
		m, _ = rootUpdate(m, enterKey())
		oldID := m.OptionsState().ID
		m, _ = rootUpdate(m, runeKey('+'))
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyEscape})
		if m.OptionsState().Framing.Zoom != 105 {
			t.Fatal("cancel retained draft changes")
		}
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "reframe", ID: oldID, Framing: data.BackgroundFraming{Fit: "fill", Zoom: 300, PositionX: 0, PositionY: 0}})
		if m.OptionsState().Framing.Zoom != 105 {
			t.Fatal("late drag changed framing")
		}
	})
	t.Run("reset and limits preserve darkness and original image selection", func(t *testing.T) {
		m := open(t)
		for range 50 {
			m, _ = rootUpdate(m, runeKey('+'))
		}
		if m.OptionsState().Framing.Zoom != 300 {
			t.Fatal("zoom exceeds maximum")
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyTab})
		m, _ = rootUpdate(m, enterKey())
		if m.OptionsState().Framing != data.DefaultBackgroundFraming() || m.OptionsState().Darkness != 70 || !m.options.hasImage {
			t.Fatal("reset changed unrelated values")
		}
		for range 5 {
			m, _ = rootUpdate(m, runeKey('-'))
		}
		if m.OptionsState().Framing.Zoom != 100 {
			t.Fatal("zoom below minimum")
		}
		for _, size := range []tea.WindowSizeMsg{{Width: 40, Height: 28}, {Width: 60, Height: 12}} {
			m, _ = rootUpdate(m, size)
			view := m.View().Content
			if !strings.Contains(view, "Done") || !strings.Contains(view, "Cancel") || len(strings.Split(view, "\n")) > size.Height {
				t.Fatalf("framing actions overflow at %+v", size)
			}
		}
	})
}
