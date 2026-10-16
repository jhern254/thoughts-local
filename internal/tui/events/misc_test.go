package events

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
)

func TestModel_MiscTimeline(t *testing.T) {
	t.Run("ghost cards share one rail timestamp per minute", func(t *testing.T) {
		for _, size := range []struct {
			name  string
			width int
		}{
			{name: "normal", width: 80},
			{name: "narrow", width: 40},
		} {
			t.Run(size.name, func(t *testing.T) {
				m, events, reader := fixture(t)
				m.Resize(size.width, 30)
				events.items = nil
				at := reader.items[0].ObservedAt.Truncate(time.Minute)
				reader.miscGroups = []data.MiscThoughtGroup{
					{ObservedAt: at, Count: 2},
					{ObservedAt: at.Add(45 * time.Second), Count: 1},
					{ObservedAt: at.Add(time.Minute), Count: 1},
				}
				m = execute(t, m, m.loadDay(m.day))
				for _, expanded := range []bool{false, true} {
					if expanded {
						m, _ = m.Update(eventKey("home"))
						var cmd tea.Cmd
						m, cmd = m.Update(eventKey("enter"))
						m = execute(t, m, cmd)
					}
					layout := m.measureTimeline()
					first := layout.positions[m.items[0].Key()]
					second := layout.positions[m.items[1].Key()]
					third := layout.positions[m.items[2].Key()]
					if !strings.HasPrefix(layout.lines[first], "10:37 AM") || strings.TrimSpace(layout.lines[second]) != "" || !strings.HasPrefix(layout.lines[third], "10:38 AM") {
						t.Fatalf("got rail rows %q, %q, %q, want one label per minute (expanded %v)", layout.lines[first], layout.lines[second], layout.lines[third], expanded)
					}
					for i, card := range layout.cards {
						if got := strings.TrimSpace(ansi.Strip(card.content[0])); got != "Misc" {
							t.Fatalf("got ghost card %d heading %q, want Misc without duplicate timestamp", i, got)
						}
					}
				}
			})
		}
	})
	t.Run("places timestamp groups on the rail and restores the expanded card after detail", func(t *testing.T) {
		m, events, reader := fixture(t)
		events.items = nil
		at := reader.items[0].ObservedAt
		for i := range reader.items {
			reader.items[i].ObservedAt = at
		}
		reader.miscGroups = []data.MiscThoughtGroup{{ObservedAt: at, Count: 230}, {ObservedAt: at.Add(time.Second), Count: 1}}
		m = execute(t, m, m.loadDay(m.day))
		m, _ = m.Update(eventKey("home"))
		if view := ansi.Strip(m.View()); (!strings.Contains(view, "Misc") || !strings.Contains(view, "10:37 AM")) || strings.Contains(view, "No events on this day") || !strings.Contains(view, "230 thoughts") {
			t.Fatalf("got view:\n%s\nwant Misc timestamp and count", view)
		}
		layout := m.measureTimeline()
		first, second := layout.positions[m.items[0].Key()], layout.positions[m.items[1].Key()]
		if first >= second || !strings.HasPrefix(ansi.Strip(layout.lines[first]), "10:37 AM") {
			t.Fatalf("got card positions %d,%d, want chronological cards anchored at their start", first, second)
		}
		m, cmd := m.Update(eventKey("enter"))
		m = execute(t, m, cmd)
		if !strings.Contains(ansi.Strip(m.View()), "230 thoughts · Newest first") {
			t.Fatalf("got picker:\n%s", ansi.Strip(m.View()))
		}
		for range 55 {
			m, cmd = m.Update(eventKey("down"))
			m = execute(t, m, cmd)
		}
		before, top := m.View(), m.position.topLine
		m, cmd = m.Update(eventKey("enter"))
		m = execute(t, m, cmd)
		if !m.picker.ShowingDetail() {
			t.Fatal("Misc thought detail did not open")
		}
		m, cmd = m.Update(eventKey("esc"))
		m = execute(t, m, cmd)
		if m.View() != before || m.position.topLine != top {
			t.Fatal("detail return lost Misc selection or scroll")
		}
		m.Resize(40, 20)
		if !strings.Contains(ansi.Strip(m.View()), "Misc") {
			t.Fatal("narrow view lost Misc card")
		}
		m, _ = m.Update(eventKey("left"))
		if !m.expanded.IsZero() {
			t.Fatal("Left did not collapse Misc")
		}
		m, _ = m.Update(eventKey("e"))
		if m.form.open || events.ends != 0 {
			t.Fatal("Misc card allowed ending a real event")
		}
	})
	t.Run("refresh preserves timestamp identity and rejects delayed expansion after collapse", func(t *testing.T) {
		m, events, reader := fixture(t)
		events.items = nil
		at := reader.items[0].ObservedAt
		reader.miscGroups = []data.MiscThoughtGroup{{ObservedAt: at, Count: 1}}
		m = execute(t, m, m.loadDay(m.day))
		m, _ = m.Update(eventKey("home"))
		m, cmd := m.Update(eventKey("enter"))
		old := cmd()
		m, _ = m.Update(eventKey("left"))
		before := m.View()
		m, next := m.Update(old)
		if next != nil || m.View() != before {
			t.Fatal("late reply reopened Misc")
		}
		m, cmd = m.Update(eventKey("enter"))
		m = execute(t, m, cmd)
		selected := m.expanded
		reader.miscGroups = append([]data.MiscThoughtGroup{{ObservedAt: at.Add(-time.Second), Count: 1}}, reader.miscGroups...)
		m = execute(t, m, m.loadDay(m.day))
		if m.expanded != selected || m.items[m.position.eventIndex].Key() != selected {
			t.Fatal("refresh changed selected Misc identity")
		}
		reader.miscGroups = nil
		m = execute(t, m, m.loadDay(m.day))
		if !m.expanded.IsZero() || !strings.Contains(m.View(), "No events on this day") {
			t.Fatal("removed group retained its picker")
		}
	})
}
