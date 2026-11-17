package goals

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
)

func (s *services) ListEarnedEvents(ctx context.Context, _ string, id int64, request data.GoalEventPageRequest) (data.GoalEventPage, error) {
	s.activityContexts = append(s.activityContexts, ctx)
	s.eventRequests = append(s.eventRequests, request)
	return data.GoalEventPage{Events: s.earnedEvents, More: s.eventsMore}, s.activityErr
}
func (s *services) ListEventLineageThoughts(ctx context.Context, _ string, goalID, eventID int64, request data.ThoughtSummaryViewRequest) (data.GoalEventThoughtPage, error) {
	s.activityContexts = append(s.activityContexts, ctx)
	s.thoughtRequests = append(s.thoughtRequests, request)
	for _, item := range s.earnedEvents {
		if item.EventID == eventID {
			return data.GoalEventThoughtPage{Event: item, Thoughts: data.ThoughtSummaryViewResult{Items: s.lineageThoughts, More: s.thoughtsMore}}, s.activityErr
		}
	}
	return data.GoalEventThoughtPage{}, data.ErrRecordNotFound
}
func (s *services) GetEventLineageThought(context.Context, string, int64, int64, int64) (*data.Thought, error) {
	return nil, data.ErrRecordNotFound
}

func activityModel(t *testing.T) (Model, *services) {
	t.Helper()
	m, s := testModel(t)
	s.earnedEvents = []data.GoalEventLineage{{EventID: 10, Activity: "Implement lineage", StartedAt: time.Unix(100, 0), EndedAt: time.Unix(200, 0), ElapsedSeconds: 100}}
	name := "Programming"
	s.lineageThoughts = []data.ThoughtSummaryView{{ThoughtID: 20, Preview: "Events own time", SubjectName: &name, ObservedAt: time.Unix(150, 0)}}
	m.open = true
	m.screen = detailView
	m, cmd := m.Update(m.loadDetail(42)())
	if cmd == nil {
		t.Fatal("Details did not load Activity History")
	}
	m, _ = m.Update(cmd())
	return m, s
}
func TestModel_ActivityHistory(t *testing.T) {
	t.Run("expands an earned Event and opens a matching Thought through the shared detail loader", func(t *testing.T) {
		m, _ := activityModel(t)
		if !strings.Contains(m.View(), "ACTIVITY HISTORY") || !strings.Contains(m.View(), "Implement lineage") {
			t.Fatalf("missing history: %s", m.View())
		}
		m, cmd := press(m, "enter")
		if cmd == nil {
			t.Fatal("expand returned no read")
		}
		m, _ = m.Update(cmd())
		if !strings.Contains(m.View(), "Events own time") {
			t.Fatalf("missing Thought: %s", m.View())
		}
		m, _ = press(m, "down")
		_, cmd = press(m, "enter")
		if cmd == nil {
			t.Fatal("Thought selection did not open detail")
		}
		if _, ok := cmd().(OpenLineageThoughtMsg); !ok {
			t.Fatal("did not request shared Thought Details")
		}
	})
	t.Run("a failed expansion retries and cancellation rejects its late result", func(t *testing.T) {
		m, s := activityModel(t)
		s.activityErr = errors.New("private failure")
		m, cmd := press(m, "enter")
		m, _ = m.Update(cmd())
		if !strings.Contains(m.View(), "retry") || strings.Contains(m.View(), "private failure") {
			t.Fatalf("unsafe or missing error: %s", m.View())
		}
		s.activityErr = nil
		m, cmd = press(m, "r")
		reply := cmd()
		m, _ = press(m, "esc")
		m, _ = m.Update(reply)
		if m.activity.expandedEventID != 0 || strings.Contains(m.View(), "Events own time") {
			t.Fatal("canceled expansion accepted late Thoughts")
		}
	})
	t.Run("changing Goals rejects old history and closing cancels its context", func(t *testing.T) {
		m, _ := activityModel(t)
		old := m.loadActivityEvents(data.GoalEventPageRequest{})
		reply := old()
		m.loadDetail(99)
		m, _ = m.Update(reply)
		if len(m.activity.events.Events) != 0 {
			t.Fatal("old Goal history replaced new selection")
		}
		m.Close()
	})
	t.Run("metadata and history fit narrow layouts and remain keyboard reachable", func(t *testing.T) {
		m, _ := activityModel(t)
		m.Resize(48, 18)
		if !strings.Contains(m.View(), "ACTIVITY HISTORY") || !strings.Contains(m.View(), "Implement lineage") {
			t.Fatalf("narrow history: %s", m.View())
		}
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		if m.activity.focused {
			t.Fatal("Tab did not move to metadata")
		}
	})
}

func TestModel_ActivityLayoutAndOwnership(t *testing.T) {
	for _, size := range [][2]int{{100, 35}, {48, 18}} {
		t.Run(fmt.Sprintf("keeps metadata history and help within %dx%d", size[0], size[1]), func(t *testing.T) {
			m, _ := activityModel(t)
			m.Resize(size[0], size[1])
			m, cmd := press(m, "enter")
			m, _ = m.Update(cmd())
			rows := strings.Split(m.View(), "\n")
			if len(rows) > size[1] {
				t.Fatalf("rendered rows: got %d, want at most %d: %q", len(rows), size[1], ansi.Strip(m.View()))
			}
			header := -1
			event := -1
			for i, row := range rows {
				if ansi.StringWidth(row) > size[0] {
					t.Fatalf("row %d width got %d, want at most %d", i, ansi.StringWidth(row), size[0])
				}
				if strings.Contains(row, "ACTIVITY HISTORY") {
					header = i
				}
				if strings.Contains(row, "Implement lineage") {
					event = i
				}
			}
			if header < 1 || event <= header {
				t.Fatalf("history positions: heading %d, Event %d", header, event)
			}
			m, _ = press(m, "down")
			if !strings.Contains(m.View(), "Events own time") {
				t.Fatal("scrolling cannot reach expanded Thought")
			}
		})
	}
	t.Run("old expansion and queued Thought navigation cannot publish after collapse or another Event", func(t *testing.T) {
		m, s := activityModel(t)
		m, cmd := press(m, "enter")
		late := cmd()
		if ctx := s.activityContexts[len(s.activityContexts)-1]; ctx.Err() != nil {
			t.Fatal("active read already canceled")
		}
		m, _ = press(m, "esc")
		if ctx := s.activityContexts[len(s.activityContexts)-1]; ctx.Err() == nil {
			t.Fatal("collapse did not cancel expansion")
		}
		m, _ = m.Update(late)
		if m.activity.expandedEventID != 0 {
			t.Fatal("late expansion reopened Event")
		}
		m, cmd = press(m, "enter")
		m, _ = m.Update(cmd())
		m, _ = press(m, "down")
		m, cmd = press(m, "enter")
		message := cmd().(OpenLineageThoughtMsg)
		if !m.AcceptsLineageThought(message) {
			t.Fatal("current Thought navigation rejected")
		}
		m, _ = press(m, "esc")
		m, cmd = press(m, "enter")
		m, _ = m.Update(cmd())
		if m.AcceptsLineageThought(message) {
			t.Fatal("new expansion accepted old Thought navigation")
		}
		m.Close()
		if m.AcceptsLineageThought(message) {
			t.Fatal("closed Goals accepted navigation")
		}
	})
}

func TestModel_ActivityPages(t *testing.T) {
	t.Run("scrolling and paging continue through older and newer Event windows", func(t *testing.T) {
		for _, key := range []tea.KeyPressMsg{{Code: tea.KeyRight}, {Code: tea.KeyDown}, {Code: tea.KeyPgDown}} {
			t.Run(key.String(), func(t *testing.T) {
				m, s := activityModel(t)
				s.eventsMore = true
				m, _ = m.Update(m.loadActivityEvents(data.GoalEventPageRequest{})())
				m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
				if strings.Contains(m.View(), "Older Events") || strings.Contains(m.View(), "Newer Events") {
					t.Fatal("database paging actions remain in the activity list")
				}
				m, cmd := m.Update(key)
				if cmd == nil {
					t.Fatal("moving beyond the last Event did not load the next window")
				}
				s.earnedEvents = []data.GoalEventLineage{{EventID: 9, Activity: "Older work", StartedAt: time.Unix(50, 0), EndedAt: time.Unix(100, 0), ElapsedSeconds: 50}}
				s.eventsMore = false
				m, _ = m.Update(cmd())
				request := s.eventRequests[len(s.eventRequests)-1]
				if request.Cursor == nil || request.Cursor.EventID != 10 || request.Direction != data.EarnedEventsOlder {
					t.Fatalf("older request: %+v", request)
				}
				m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
				if cmd == nil {
					t.Fatal("Left from the first older page did not load newer Events")
				}
				s.earnedEvents = []data.GoalEventLineage{{EventID: 10, Activity: "Implement lineage", StartedAt: time.Unix(100, 0), EndedAt: time.Unix(200, 0)}}
				m, _ = m.Update(cmd())
				request = s.eventRequests[len(s.eventRequests)-1]
				if request.Cursor == nil || request.Cursor.EventID != 9 || request.Direction != data.EarnedEventsNewer {
					t.Fatalf("newer request: %+v", request)
				}
				if got := m.activity.list.SelectedItem().(activityRow).event.EventID; got != 10 {
					t.Fatalf("returned Event: got %d, want 10", got)
				}
			})
		}
	})
	t.Run("Thought paging reuses the Event and cursor without retaining previous bodies", func(t *testing.T) {
		m, s := activityModel(t)
		s.thoughtsMore = true
		m, cmd := press(m, "enter")
		m, _ = m.Update(cmd())
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
		m, cmd = press(m, "enter")
		s.lineageThoughts = []data.ThoughtSummaryView{{ThoughtID: 19, Preview: "Older idea", ObservedAt: time.Unix(140, 0)}}
		s.thoughtsMore = false
		m, _ = m.Update(cmd())
		request := s.thoughtRequests[len(s.thoughtRequests)-1]
		if request.Cursor == nil || request.Cursor.ThoughtID != 20 || request.Direction != data.ThoughtsOlder {
			t.Fatalf("older Thoughts request: %+v", request)
		}
		if len(m.activity.thoughts.Items) != 1 || m.activity.thoughts.Items[0].ThoughtID != 19 {
			t.Fatal("Thought pages accumulated or lost current page")
		}
		if m.activity.expandedEventID != 10 {
			t.Fatal("Thought paging lost Event")
		}
	})
	t.Run("clicking an Event opens it and clicking metadata changes focus", func(t *testing.T) {
		m, _ := activityModel(t)
		m, cmd := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 3, Y: m.body.Height() + activityHeaderRows})
		if cmd == nil {
			t.Fatal("Event click did not start expansion")
		}
		m, _ = m.Update(cmd())
		if m.activity.expandedEventID != 10 {
			t.Fatal("Event click selected wrong Event")
		}
		m, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 1, Y: 0})
		if m.activity.focused {
			t.Fatal("metadata click retained history focus")
		}
	})
}

func TestModel_ActivityReturnToForm(t *testing.T) {
	t.Run("editing from split Details restores the full form viewport", func(t *testing.T) {
		m, _ := activityModel(t)
		m.Resize(48, 18)
		m, _ = press(m, "e")
		if got, want := m.body.Height(), m.height-4; got != want {
			t.Fatalf("form viewport height: got %d, want %d", got, want)
		}
		for m.focus < m.formSaveFocus() {
			m, _ = press(m, "down")
		}
		if !strings.Contains(m.View(), "Save") {
			t.Fatal("form cannot scroll to Save after history")
		}
	})
}

func TestModel_ActivityTimeFormatting(t *testing.T) {
	for _, tc := range []struct {
		name        string
		at          time.Time
		date, clock string
	}{
		{"afternoon uses twelve-hour minutes", time.Date(2026, 11, 17, 0, 7, 23, 0, time.UTC), "Nov 16", "04:07 PM"},
		{"midnight uses twelve AM", time.Date(2026, 11, 16, 8, 0, 59, 0, time.UTC), "Nov 16", "12:00 AM"},
		{"noon uses twelve PM", time.Date(2026, 11, 16, 20, 0, 59, 0, time.UTC), "Nov 16", "12:00 PM"},
		{"morning hours have two digits", time.Date(2026, 11, 16, 17, 5, 7, 0, time.UTC), "Nov 16", "09:05 AM"},
		{"summer dates follow Pacific daylight saving time", time.Date(2026, 7, 2, 4, 7, 23, 0, time.UTC), "Jul 1", "09:07 PM"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, service := activityModel(t)
			m.Resize(100, 35)
			service.earnedEvents[0].StartedAt = tc.at
			service.earnedEvents[0].EndedAt = tc.at.Add(time.Hour)
			service.lineageThoughts[0].ObservedAt = tc.at
			cmd := m.loadDetail(42)
			m, cmd = m.Update(cmd())
			m, _ = m.Update(cmd())
			m, cmd = press(m, "enter")
			m, _ = m.Update(cmd())
			rendered := ansi.Strip(m.View())
			for _, want := range []string{"▾ " + tc.date + " · " + tc.clock + " · Implement lineage", "Programming · " + tc.clock} {
				if !strings.Contains(rendered, want) {
					t.Fatalf("Activity History: got %q, want %q", rendered, want)
				}
			}
		})
	}
}
