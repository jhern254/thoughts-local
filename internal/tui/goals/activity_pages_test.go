package goals

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
)

func historyEvents(count int, newestID int64) []data.GoalEventLineage {
	events := make([]data.GoalEventLineage, count)
	for i := range events {
		id := newestID - int64(i)
		events[i] = data.GoalEventLineage{EventID: id, Activity: "Historical work", StartedAt: time.Unix(id*60, 0), EndedAt: time.Unix(id*60+30, 0), ElapsedSeconds: 30}
	}
	return events
}

func TestModel_ContinuousActivityPages(t *testing.T) {
	t.Run("ten-page windows continue forward and return to the tenth dot without accumulating Events", func(t *testing.T) {
		m, s := activityModel(t)
		m.Resize(100, 35)
		s.eventsMore = true
		cmd := m.loadActivityEvents(data.GoalEventPageRequest{})
		count := m.activity.eventRequest.Limit
		s.earnedEvents = historyEvents(count, 1000)
		m, _ = m.Update(activityReadReply(cmd))
		for window := 0; window < 3; window++ {
			if got := m.activity.list.Paginator.TotalPages; got != 10 {
				t.Fatalf("pages in window: got %d, want 10", got)
			}
			for page := 1; page < 10; page++ {
				m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
				if cmd != nil || m.activity.list.Paginator.Page != page {
					t.Fatalf("page %d triggered a read or selected page %d", page, m.activity.list.Paginator.Page)
				}
			}
			if len(s.eventRequests) != window+2 {
				t.Fatalf("reads: got %d, want %d", len(s.eventRequests), window+2)
			}
			m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
			if cmd == nil {
				t.Fatal("tenth page did not continue to older Events")
			}
			s.earnedEvents = historyEvents(count, 1000-int64((window+1)*count))
			m, _ = m.Update(activityReadReply(cmd))
			if len(m.activity.events.Events) != count || m.activity.list.Paginator.Page != 0 {
				t.Fatal("next window accumulated Events or failed to reset to the first dot")
			}
		}
		m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
		if cmd == nil {
			t.Fatal("first dot did not continue to newer Events")
		}
		s.earnedEvents = historyEvents(count, 1000-int64(2*count))
		m, _ = m.Update(activityReadReply(cmd))
		if m.activity.list.Paginator.Page != 9 || m.activity.list.Index() != count-m.activity.list.Paginator.PerPage {
			t.Fatal("backward travel did not preserve the highlighted row on the tenth page")
		}
	})
	t.Run("a failed window preserves Events allows retry and rejects a cancelled reply", func(t *testing.T) {
		m, s := activityModel(t)
		s.eventsMore = true
		m, _ = m.Update(activityReadReply(m.loadActivityEvents(data.GoalEventPageRequest{})))
		m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
		if cmd == nil {
			t.Fatal("next window did not start")
		}
		m, duplicate := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
		if duplicate != nil {
			t.Fatal("repeated paging started a duplicate read")
		}
		s.activityErr = errors.New("private read failure")
		m, _ = m.Update(activityReadReply(cmd))
		if m.activity.list.SelectedItem().(activityRow).event.EventID != 10 || !strings.Contains(m.View(), "R: retry") {
			t.Fatal("failed paging lost Events or retry guidance")
		}
		s.activityErr = nil
		m, cmd = press(m, "r")
		reply := activityReadReply(cmd)
		m.Close()
		m, _ = m.Update(reply)
		if m.open || m.activity.owner != nil {
			t.Fatal("cancelled page reopened history")
		}
	})
	t.Run("the mouse wheel crosses an Event window and an empty reply can recover", func(t *testing.T) {
		m, s := activityModel(t)
		s.eventsMore = true
		m, _ = m.Update(activityReadReply(m.loadActivityEvents(data.GoalEventPageRequest{})))
		m, cmd := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown, Y: m.body.Height() + activityHeaderRows})
		if cmd == nil {
			t.Fatal("wheel did not continue through history")
		}
		s.earnedEvents = nil
		s.eventsMore = false
		m, _ = m.Update(activityReadReply(cmd))
		m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
		if cmd == nil {
			t.Fatal("empty window trapped navigation")
		}
		s.earnedEvents = historyEvents(1, 10)
		m, _ = m.Update(activityReadReply(cmd))
		request := s.eventRequests[len(s.eventRequests)-1]
		if request.Cursor != nil || m.activity.list.SelectedItem().(activityRow).event.EventID != 10 {
			t.Fatal("empty window did not recover the newest remaining Event")
		}
	})
	t.Run("dots stay in groups of ten when expansion or resizing adds display pages", func(t *testing.T) {
		m, s := activityModel(t)
		s.earnedEvents = historyEvents(80, 1000)
		m, _ = m.Update(activityReadReply(m.loadActivityEvents(data.GoalEventPageRequest{})))
		s.lineageThoughts = make([]data.ThoughtSummaryView, 50)
		for i := range s.lineageThoughts {
			s.lineageThoughts[i] = data.ThoughtSummaryView{ThoughtID: int64(i + 1), Preview: "A related idea"}
		}
		m, cmd := press(m, "enter")
		m, _ = m.Update(activityReadReply(cmd))
		if m.activity.list.Paginator.TotalPages <= 10 {
			t.Fatal("fixture did not expand beyond ten pages")
		}
		m.Resize(48, 18)
		for _, page := range []int{0, 9, 10, 19, 20} {
			m.activity.list.Select(page * m.activity.list.Paginator.PerPage)
			active := page % 10
			expected := strings.Repeat(m.activity.list.Paginator.InactiveDot, active) + m.activity.list.Paginator.ActiveDot + strings.Repeat(m.activity.list.Paginator.InactiveDot, 9-active)
			if !strings.Contains(m.View(), expected) {
				t.Fatalf("page %d did not highlight dot %d", page, active+1)
			}
			for _, row := range strings.Split(ansi.Strip(m.View()), "\n") {
				dots := strings.TrimSpace(row)
				if strings.Trim(dots, "•") == "" && len([]rune(dots)) > 10 {
					t.Fatalf("got %d dots, want at most 10", len([]rune(dots)))
				}
			}
		}
	})
}
