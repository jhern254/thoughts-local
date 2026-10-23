package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestModel_HistoricalDayBurst(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		keys   []rune
		days   int
		repeat bool
	}{
		{"individual keys while replies are pending", []rune{tea.KeyLeft, tea.KeyLeft, tea.KeyLeft}, -3, false},
		{"alternating keys while replies are pending", []rune{tea.KeyLeft, tea.KeyRight, tea.KeyLeft, tea.KeyLeft}, -2, false},
		{"held keys while replies are pending", []rune{tea.KeyRight, tea.KeyRight, tea.KeyRight}, 3, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			m := newMindsetHome(t, 0)
			defer func() { m.events.Close() }()
			m, _ = rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 30})
			// Start far enough in history that Right always means next day.
			for range 10 {
				var cmd tea.Cmd
				m, cmd = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
				m = runHomeData(t, m, cmd)
			}
			day := m.events.Day()
			var pending []tea.Cmd
			var stale tea.Msg
			for i, code := range scenario.keys {
				var cmd tea.Cmd
				m, cmd = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: code, IsRepeat: scenario.repeat}))
				if cmd == nil {
					t.Fatal("day key produced no load")
				}
				pending = append(pending, cmd)
				if i == 0 {
					// Capture an actual successful read while it still owns
					// its request, then deliver it after the newer day wins.
					stale = cmd().(tea.BatchMsg)[0]()
				}
				if got := m.events.Day(); !got.Equal(day) {
					t.Fatalf("pending day: got %v, want accepted day %v", got, day)
				}
			}
			m = runHomeData(t, m, pending[len(pending)-1])
			if got, want := m.events.Day(), day.AddDate(0, 0, scenario.days); !got.Equal(want) {
				t.Fatalf("final day: got %v, want %v", got, want)
			}
			accepted := m.View().Content
			m, cmd := rootUpdate(m, stale)
			m = runHomeData(t, m, cmd)
			for _, obsolete := range pending[:len(pending)-1] {
				m = runHomeData(t, m, obsolete)
			}
			if got := m.View().Content; got != accepted {
				t.Fatal("obsolete replies replaced the accepted day")
			}
			if !m.events.DayReady() || m.homeFocus() != homePanelEvents || m.screen != screenEvents {
				t.Fatal("day burst changed focus or left the timeline pending")
			}
		})
	}
}
