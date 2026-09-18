package events

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
)

func TestModel_TimelinePosition(t *testing.T) {
	t.Run("losing focus pauses follow-now and returning focus preserves selection", func(t *testing.T) {
		m, _, _ := fixture(t)
		selected, top := m.items[m.position.eventIndex].EventID, m.position.topLine
		m.SetFocused(false)
		m, _ = m.Update(tickMsg{m.owner, m.session, m.clock.AddDate(0, 0, 1)})
		if m.load.eventsPending || m.position.followNow || m.position.topLine != top {
			t.Fatal("blurred timeline followed the clock into another day")
		}
		m.SetFocused(true)
		if m.blurred || m.position.followNow || m.items[m.position.eventIndex].EventID != selected {
			t.Fatal("returning focus must preserve manual position and selected event")
		}
		m, _ = m.Update(eventKey("n"))
		if !m.FormOpen() {
			t.Fatal("focused Events did not accept keyboard input")
		}
	})
	t.Run("last event can stay at the top with unused rows below", func(t *testing.T) {
		m, _, _ := fixture(t)
		for _, height := range []int{27, 35} {
			m.Resize(80, height)
			m, _ = m.Update(eventKey("home"))
			lines := strings.Split(ansi.Strip(m.timelineBody()), "\n")
			if !strings.HasPrefix(lines[0], "10:37 AM  ╭") || lines[len(lines)-1] != "" {
				t.Fatalf("got body %q, want selected final event at top with blank rows below", lines)
			}
		}
	})
	t.Run("paging moves rendered lines independently of selection and Home restores it", func(t *testing.T) {
		m, _, _ := fixture(t)
		m, _ = m.Update(eventKey("home"))
		before := m.View()
		selected := m.items[m.position.eventIndex].EventID
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp}))
		if m.View() == before || m.items[m.position.eventIndex].EventID != selected {
			t.Fatal("PageUp must move the timeline without selecting another event")
		}
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
		if m.View() != before {
			t.Fatal("PageDown must restore the same rendered-line position")
		}
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
		if got := strings.Split(ansi.Strip(m.timelineBody()), "\n")[0]; !strings.Contains(got, "Now") {
			t.Fatalf("got first row %q, want final Now row at the top", got)
		}
		m, _ = m.Update(eventKey("home"))
		if m.View() != before {
			t.Fatal("Home must reveal the first selected event again")
		}
	})
	t.Run("Up and Down select adjacent events and stop at the ends", func(t *testing.T) {
		m, _, _ := fixture(t)
		end := m.clock.Add(-10 * time.Minute)
		m.items[0].EndedAt = &end
		label := "Second event"
		m.items = append(m.items, data.Event{EventID: 2, StartedAt: end, ActivityType: &label})
		m, _ = m.Update(eventKey("home"))
		before := m.View()
		for range 2 {
			m, _ = m.Update(eventKey("down"))
			if m.items[m.position.eventIndex].EventID != 2 || !strings.Contains(m.View(), label) {
				t.Fatal("Down must select and reveal the second event, stopping at the last event")
			}
		}
		for range 2 {
			m, _ = m.Update(eventKey("up"))
			if m.items[m.position.eventIndex].EventID != 1 || m.View() != before {
				t.Fatal("Up must restore the first event and stop there")
			}
		}
	})
}
