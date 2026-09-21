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
	t.Run("fitting collapsed overview stays at the top through selection resize and collapse", func(t *testing.T) {
		m, service, store := fixture(t)
		m.day = m.day.AddDate(0, 0, -1)
		firstEnd, secondEnd := m.day.Add(12*time.Hour), m.day.Add(18*time.Hour)
		service.items = []data.Event{
			{EventID: 1, StartedAt: m.day.Add(6 * time.Hour), EndedAt: &firstEnd},
			{EventID: 2, StartedAt: firstEnd, EndedAt: &secondEnd},
		}
		m.position.followNow = false
		m.items = nil
		m.Resize(80, 26) // Exactly 22 body rows for the collapsed day.
		m = execute(t, m, m.loadDay(m.day))
		assertOverview := func() {
			t.Helper()
			if m.position.topLine != 0 || !strings.HasPrefix(ansi.Strip(m.timelineBody()), "12:00 AM") || !strings.Contains(m.timelineBody(), "...") {
				t.Fatalf("got offset %d and body %q, want complete overview at top", m.position.topLine, m.timelineBody())
			}
		}
		assertOverview()
		lists, reads, counts := service.lists, store.reads, store.counts
		for _, key := range []string{"down", "up", "home", "down"} {
			m, _ = m.Update(eventKey(key))
			assertOverview()
		}
		m.Resize(40, 27)
		assertOverview()
		if service.lists != lists || store.reads != reads || store.counts != counts {
			t.Fatal("selection or resize read data")
		}
		m, cmd := m.Update(eventKey("enter"))
		m = execute(t, m, cmd)
		if m.position.topLine == 0 || !strings.HasPrefix(ansi.Strip(m.timelineBody()), "12:00 PM  ╭") {
			t.Fatal("expansion did not anchor the selected card at its start")
		}
		m, _ = m.Update(eventKey("left"))
		assertOverview()
		if m.position.eventIndex != 1 {
			t.Fatal("collapse changed the selected event")
		}
		m.Resize(40, 14)
		m, _ = m.Update(eventKey("down"))
		if m.position.topLine == 0 {
			t.Fatal("overflowing layout did not reveal selection")
		}
		m.Resize(80, 26)
		assertOverview()
	})
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
	t.Run("overflowing day keeps the last event at the top with unused rows below", func(t *testing.T) {
		m, _, _ := fixture(t)
		for _, height := range []int{14, 16} {
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
		m.Resize(80, 14)
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
