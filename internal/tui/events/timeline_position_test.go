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
	t.Run("shrinking the timeline keeps the selected collapsed card visible", func(t *testing.T) {
		m, _, store := fixture(t)
		m.Resize(120, 40)
		m, _ = m.Update(eventKey("home"))
		selected, reads := m.entries[m.position.entryIndex].Key(), store.reads
		m.SetFocused(false)
		for _, size := range [][2]int{{54, 14}, {120, 40}, {40, 14}} {
			m.Resize(size[0], size[1])
			body := ansi.Strip(m.timelineBody())
			if !strings.Contains(body, "Reading") || !strings.Contains(body, "230 thoughts") {
				t.Fatalf("%dx%d: selected collapsed card left the viewport: %q", size[0], size[1], body)
			}
			if m.entries[m.position.entryIndex].Key() != selected || store.reads != reads || !m.blurred {
				t.Fatal("resize changed selection, queried data, or stole panel focus")
			}
		}
	})

	t.Run("losing focus pauses follow-now and returning focus preserves selection", func(t *testing.T) {
		m, _, _ := fixture(t)
		selected, top := m.entries[m.position.entryIndex].Event.EventID, m.position.topLine
		m.SetFocused(false)
		m, _ = m.Update(tickMsg{m.owner, m.session, m.clock.AddDate(0, 0, 1)})
		if m.load.entriesPending || m.position.followNow || m.position.topLine != top {
			t.Fatal("blurred timeline followed the clock into another day")
		}
		m.SetFocused(true)
		if m.blurred || m.position.followNow || m.entries[m.position.entryIndex].Event.EventID != selected {
			t.Fatal("returning focus must preserve manual position and selected event")
		}
		m, _ = m.Update(eventKey("n"))
		if !m.FormOpen() {
			t.Fatal("focused Events did not accept keyboard input")
		}
	})
	t.Run("last event can stay at the top with the metrics preview below", func(t *testing.T) {
		m, _, _ := fixture(t)
		for _, height := range []int{27, 35} {
			m.Resize(80, height)
			m, _ = m.Update(eventKey("home"))
			lines := strings.Split(ansi.Strip(m.timelineBody()), "\n")
			if !strings.HasPrefix(timelineCardText(lines[0]), "10:37 AM  ╭") || !strings.Contains(strings.Join(lines, "\n"), "Subject time today") {
				t.Fatalf("got body %q, want selected final event at top with the metrics preview below", lines)
			}
		}
	})
	t.Run("paging moves rendered lines independently of selection and Home restores it", func(t *testing.T) {
		m, _, _ := fixture(t)
		m, _ = m.Update(eventKey("home"))
		before := m.View()
		selected := m.entries[m.position.entryIndex].Event.EventID
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp}))
		if m.View() == before || m.entries[m.position.entryIndex].Event.EventID != selected {
			t.Fatal("PageUp must move the timeline without selecting another event")
		}
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
		if m.View() != before {
			t.Fatal("PageDown must restore the same rendered-line position")
		}
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
		if got := strings.Split(ansi.Strip(m.timelineBody()), "\n")[0]; !strings.Contains(got, "╰") {
			t.Fatalf("got first row %q, want the final metrics border at the top", got)
		}
		m, _ = m.Update(eventKey("home"))
		if m.View() != before {
			t.Fatal("Home must reveal the first selected event again")
		}
	})
	t.Run("Up and Down select adjacent events and stop at the ends", func(t *testing.T) {
		m, _, _ := fixture(t)
		end := m.clock.Add(-10 * time.Minute)
		m.entries[0].Event.EndedAt = &end
		label := "Second event"
		m.entries = append(m.entries, eventEntries([]data.Event{{EventID: 2, StartedAt: end, ActivityType: &label}})...)
		m, _ = m.Update(eventKey("home"))
		before := m.View()
		for range 2 {
			m, _ = m.Update(eventKey("down"))
			if m.entries[m.position.entryIndex].Event.EventID != 2 || !strings.Contains(m.View(), label) {
				t.Fatal("Down must select and reveal the second event, stopping at the last event")
			}
		}
		if m.position.target != targetDayBoundary {
			t.Fatal("second Down did not reach Now")
		}
		m, _ = m.Update(eventKey("up"))
		if m.position.target != targetEntry || m.entries[m.position.entryIndex].Event.EventID != 2 {
			t.Fatal("Up from Now did not select the latest entry")
		}
		for range 2 {
			m, _ = m.Update(eventKey("up"))
			if m.entries[m.position.entryIndex].Event.EventID != 1 || m.View() != before {
				t.Fatal("Up must restore the first event and stop there")
			}
		}
	})
}
