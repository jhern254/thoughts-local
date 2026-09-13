package events

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
)

type blockedEventRead struct {
	Service
	started chan context.Context
}

func TestModel_DayTransition(t *testing.T) {
	t.Run("retains the displayed day until the list arrives without waiting for counts or preview", func(t *testing.T) {
		m, s, _ := fixture(t)
		before, day := m.View(), m.day
		start := day.AddDate(0, 0, -1).Add(9 * time.Hour)
		label := "Yesterday's event"
		s.items = []data.Event{{EventID: 2, StartedAt: start, ActivityType: &label}}
		m, cmd := m.Update(eventKey("left"))
		if m.View() != before {
			t.Fatal("pending navigation changed the displayed date, cards, or feedback")
		}
		batch := cmd().(tea.BatchMsg)
		m, latest := m.Update(batch[0]())
		view := m.View()
		if !m.day.Equal(displaytime.Day(start)) || !strings.Contains(view, label) || strings.Contains(view, "Loading events") || !m.countPending || !m.latestPending || latest == nil {
			t.Fatalf("got %q, want new day immediately while counts and preview remain pending", view)
		}
		m = execute(t, m, batch[1])
		if batch[2]() != nil {
			t.Fatal("list completion did not cancel the feedback timer")
		}
		m.Close()
	})
	t.Run("counts arriving first cannot change the displayed day's cards", func(t *testing.T) {
		m, s, v := fixture(t)
		before := m.View()
		start := m.day.AddDate(0, 0, -1).Add(9 * time.Hour)
		end := start.Add(time.Hour)
		s.items = []data.Event{{EventID: 1, StartedAt: start, EndedAt: &end}}
		v.items = v.items[:1]
		m, cmd := m.Update(eventKey("left"))
		batch := cmd().(tea.BatchMsg)
		m, _ = m.Update(batch[1]())
		if m.View() != before {
			t.Fatal("early counts changed the old day before its replacement arrived")
		}
		m = execute(t, m, batch[0])
		if !strings.Contains(m.View(), "1 thought") || strings.Contains(m.View(), "230 thoughts") {
			t.Fatal("new day did not adopt its previously completed counts")
		}
		m.Close()
	})
	t.Run("rapid arrows advance the requested day without changing the displayed day", func(t *testing.T) {
		m, _, _ := fixture(t)
		before, day := m.View(), m.day
		var cmd tea.Cmd
		for range 3 {
			m, cmd = m.Update(eventKey("left"))
			if m.View() != before {
				t.Fatal("rapid navigation blanked or relabeled the displayed day")
			}
		}
		m = execute(t, m, cmd)
		if !m.day.Equal(day.AddDate(0, 0, -3)) {
			t.Fatalf("got day %v, want three days earlier", m.day)
		}
		m.Close()
	})
	t.Run("failure retains the visible day and original error without exposing its text", func(t *testing.T) {
		m, s, _ := fixture(t)
		day, selected := m.day, m.items[m.index].EventID
		var logs bytes.Buffer
		logger, err := logging.New(&logs, "test", "info")
		if err != nil {
			t.Fatal(err)
		}
		m.logger = logger
		s.listErr = errors.New("PRIVATE_DAY_READ_MARKER")
		m, cmd := m.Update(eventKey("left"))
		m = execute(t, m, cmd)
		if m.err != s.listErr || !m.day.Equal(day) || m.items[m.index].EventID != selected || !strings.Contains(m.View(), "Could not load events for September 10, 2026") {
			t.Fatal("failure lost the displayed day, selection, original error, or safe feedback")
		}
		if strings.Contains(m.View()+logs.String(), "PRIVATE_DAY_READ_MARKER") || strings.Contains(m.View(), "Loading events") {
			t.Fatal("failure exposed private text or left loading feedback visible")
		}
		s.listErr = nil
		m, cmd = m.Update(eventKey("left"))
		m = execute(t, m, cmd)
		if !m.day.Equal(day.AddDate(0, 0, -1)) || m.err != nil {
			t.Fatal("navigation did not recover from the displayed day")
		}
		m.Close()
	})
	t.Run("empty day is shown only after its list arrives", func(t *testing.T) {
		m, s, _ := fixture(t)
		s.items = nil
		m, cmd := m.Update(eventKey("left"))
		if strings.Contains(m.View(), "No events on this day") {
			t.Fatal("pending read was presented as an empty day")
		}
		m = execute(t, m, cmd)
		if !strings.Contains(m.View(), "No events on this day") || strings.Contains(m.View(), "Loading events") {
			t.Fatal("completed empty day was not presented normally")
		}
		m.Close()
	})
	t.Run("End supersedes pending navigation without opening the old day's event", func(t *testing.T) {
		m, _, _ := fixture(t)
		today := m.day
		m, old := m.Update(eventKey("left"))
		m, cmd := m.Update(eventKey("enter"))
		if cmd != nil || m.expanded != 0 {
			t.Fatal("pending navigation opened the previous day's card")
		}
		m, cmd = m.Update(eventKey("end"))
		m = execute(t, m, cmd)
		before := m.View()
		m = execute(t, m, old)
		if !m.day.Equal(today) || !m.following || m.View() != before {
			t.Fatal("End failed to supersede pending navigation and follow today")
		}
		m.Close()
	})
	t.Run("pending cards still respond to panel focus and terminal resize", func(t *testing.T) {
		m, _, _ := fixture(t)
		m, cmd := m.Update(eventKey("left"))
		focused := m.View()
		m.SetFocused(false)
		if m.View() == focused {
			t.Fatal("pending timeline retained its active highlight after panel switching")
		}
		m.SetFocused(true)
		if m.View() != focused {
			t.Fatal("returning focus did not restore the pending timeline's selection")
		}
		m.Resize(40, 20)
		for _, line := range strings.Split(m.View(), "\n") {
			if ansi.StringWidth(line) > 40 {
				t.Fatalf("pending row exceeded resized width: %q", line)
			}
		}
		if strings.Count(m.View(), "\n") >= 20 || !strings.Contains(m.View(), "Reading") {
			t.Fatal("resize lost pending cards or exceeded terminal height")
		}
		m = execute(t, m, cmd)
		m.Close()
	})
	t.Run("refresh still permits opening the visible current event and navigation permits Create", func(t *testing.T) {
		m, _, _ := fixture(t)
		m, _ = m.Update(eventKey("r"))
		m, cmd := m.Update(eventKey("right"))
		if cmd == nil || m.expanded == 0 {
			t.Fatal("refresh blocked opening the current event")
		}
		m, _ = m.Update(eventKey("esc"))
		m, _ = m.Update(eventKey("left"))
		m, _ = m.Update(eventKey("n"))
		if !m.form.open {
			t.Fatal("pending day navigation blocked Create")
		}
		m.Close()
	})
}

func TestModel_CollapsedCounts(t *testing.T) {
	t.Run("retry does not present a failed count as a successful previous value", func(t *testing.T) {
		m, _, v := fixture(t)
		v.err = errors.New("PRIVATE_COUNT_FAILURE")
		m = execute(t, m, m.loadDay(m.day))
		if m.count(1) != "Thought count unavailable" || m.countErr != v.err {
			t.Fatal("count failure lost its safe state or original error")
		}
		v.err = nil
		v.items = nil
		m, cmd := m.Update(eventKey("r"))
		batch := cmd().(tea.BatchMsg)
		m = execute(t, m, batch[0])
		if m.count(1) != "" || strings.Contains(m.View(), "PRIVATE_COUNT_FAILURE") {
			t.Fatal("retry retained failed count data or exposed private error text")
		}
		m = execute(t, m, batch[1])
		if m.count(1) != "0 thoughts" {
			t.Fatal("successful empty count was not displayed immediately")
		}
		m.Close()
	})
	t.Run("new day shows no invented count and only gives feedback for a slow read", func(t *testing.T) {
		m, s, v := fixture(t)
		start := m.day.AddDate(0, 0, -1).Add(9 * time.Hour)
		end := start.Add(time.Hour)
		s.items = []data.Event{{EventID: 1, StartedAt: start, EndedAt: &end}}
		v.items = v.items[:1]
		m, cmd := m.Update(eventKey("left"))
		batch := cmd().(tea.BatchMsg)
		m = execute(t, m, batch[0])
		if got := m.count(1); got != "" {
			t.Fatalf("got pending count %q, want an empty reserved row", got)
		}
		before := strings.Count(m.card(m.items[0], true), "\n")
		m, _ = m.Update(LoadDelayed{m.owner, m.request})
		if m.count(1) != "Counting thoughts…" || strings.Contains(m.View(), "Loading events") {
			t.Fatal("slow count did not show its own feedback independently of events")
		}
		m = execute(t, m, batch[1])
		if m.count(1) != "1 thought" || strings.Count(m.card(m.items[0], true), "\n") != before {
			t.Fatal("ready count did not replace feedback without changing card height")
		}
		m.Close()
	})
	t.Run("refresh retains a known count but changed event intervals discard it", func(t *testing.T) {
		m, s, v := fixture(t)
		v.items = v.items[:2]
		m, cmd := m.Update(eventKey("r"))
		batch := cmd().(tea.BatchMsg)
		m = execute(t, m, batch[0])
		if m.count(1) != "230 thoughts" {
			t.Fatal("refresh replaced the last successful count with loading feedback")
		}
		m, _ = m.Update(LoadDelayed{m.owner, m.request})
		if m.count(1) != "230 thoughts · refreshing…" {
			t.Fatal("slow refresh did not retain and qualify its previous count")
		}
		m = execute(t, m, batch[1])
		if m.count(1) != "2 thoughts" {
			t.Fatal("refresh did not apply its count immediately")
		}
		// Replace rather than mutate the fixture's shared event slice.
		s.items = []data.Event{{EventID: 1, StartedAt: s.items[0].StartedAt.Add(time.Minute)}}
		m, cmd = m.Update(eventKey("r"))
		m = execute(t, m, cmd().(tea.BatchMsg)[0])
		if m.count(1) != "" {
			t.Fatal("changed interval retained a count for a different time range")
		}
		m.Close()
	})
}

func (s blockedEventRead) List(ctx context.Context, _ string, _, _ time.Time) ([]data.Event, error) {
	s.started <- ctx
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestModel_ReloadCancellation(t *testing.T) {
	t.Run("superseded commands do not invoke the list or count services", func(t *testing.T) {
		m, s, v := fixture(t)
		lists, counts := s.lists, v.counts
		old := m.loadDay(m.day)
		current := m.loadDay(m.day)
		m = execute(t, m, old)
		if s.lists != lists || v.counts != counts || !m.loading {
			t.Fatal("obsolete commands invoked services or cleared current loading")
		}
		m = execute(t, m, current)
		if m.loading || m.err != nil || s.lists != lists+1 || v.counts != counts+1 {
			t.Fatal("current reload did not complete normally")
		}
		m.Close()
	})
	t.Run("refresh and close cancel a read already in progress", func(t *testing.T) {
		for _, closeView := range []bool{false, true} {
			m, _, _ := fixture(t)
			started := make(chan context.Context, 1)
			m.service = blockedEventRead{Service: m.service, started: started}
			cmd := m.loadDay(m.day)().(tea.BatchMsg)[0]
			reply := make(chan tea.Msg, 1)
			go func() { reply <- cmd() }()
			ctx := <-started
			if closeView {
				m.Close()
			} else {
				_ = m.loadDay(m.day)
				defer m.Close()
			}
			if !errors.Is(ctx.Err(), context.Canceled) {
				t.Error("obsolete in-flight read was not canceled")
				// Bound the red test without stranding a goroutine.
				return
			}
			before := m.View()
			m = execute(t, m, func() tea.Msg { return <-reply })
			if m.View() != before {
				t.Fatal("canceled reply changed current presentation")
			}
		}
	})
	t.Run("late latest-preview command inherits its reload cancellation", func(t *testing.T) {
		m, _, _ := fixture(t)
		batch := m.loadDay(m.day)().(tea.BatchMsg)
		m, latest := m.Update(batch[0]())
		_ = m.loadDay(m.day)
		result := latest().(Latest)
		if !errors.Is(result.err, context.Canceled) {
			t.Fatalf("got latest error %v, want canceled", result.err)
		}
		m.Close()
	})
}
