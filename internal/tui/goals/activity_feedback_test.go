package goals

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
)

func TestModel_ActivityFeedback(t *testing.T) {
	t.Run("fast reads show results without loading or false empty feedback", func(t *testing.T) {
		m, _ := activityModel(t)
		cmd := m.loadActivityEvents(data.GoalEventPageRequest{})
		m.activity.events = data.GoalEventPage{}
		m.refreshActivityRows()
		before := ansi.Strip(m.View())
		for _, text := range []string{"Loading activity", "No earned Events", "No items"} {
			if strings.Contains(before, text) {
				t.Fatalf("pending history shows %q", text)
			}
		}
		m, _ = m.Update(activityReadReply(cmd))
		if strings.Contains(m.View(), "Loading activity") || !strings.Contains(m.View(), "Implement lineage") {
			t.Fatal("fast result was delayed or showed loading feedback")
		}
		if len(strings.Split(m.View(), "\n")) != len(strings.Split(before, "\n")) {
			t.Fatal("read completion changed the reserved layout height")
		}
	})
	t.Run("an Event with no matching Thoughts remains expanded without redundant feedback", func(t *testing.T) {
		m, s := activityModel(t)
		s.lineageThoughts = nil
		m, cmd := press(m, "enter")
		m, _ = m.Update(activityReadReply(cmd))
		if m.activity.expandedEventID != 10 || !strings.Contains(m.View(), "Implement lineage") || strings.Contains(m.View(), "No matching Thoughts") {
			t.Fatal("empty expansion lost its Event or showed redundant feedback")
		}
	})
	t.Run("slow reads show loading only after owned feedback and errors clear it immediately", func(t *testing.T) {
		m, s := activityModel(t)
		cmd := m.loadActivityEvents(data.GoalEventPageRequest{})
		batch := cmd().(tea.BatchMsg)
		feedback := activityFeedbackMsg{owner: m.activity.owner}
		if !Owns(feedback) {
			t.Fatal("root cannot route activity feedback")
		}
		m, _ = m.Update(feedback)
		if !strings.Contains(m.View(), "Loading activity") {
			t.Fatal("slow read did not show feedback")
		}
		s.activityErr = errors.New("private read failure")
		m, _ = m.Update(batch[0]())
		if strings.Contains(m.View(), "Loading activity") || !strings.Contains(m.View(), "R: retry") || strings.Contains(m.View(), "private read failure") {
			t.Fatal("read error did not replace loading with safe retry guidance")
		}
		m, _ = m.Update(feedback)
		if m.activity.showLoading || batch[1]() != nil {
			t.Fatal("finished read retained its timer or accepted queued feedback")
		}
	})
	t.Run("completion cancellation and replacement retire the old feedback timer", func(t *testing.T) {
		for _, action := range []string{"complete", "collapse", "close", "change Goal", "replace read"} {
			t.Run(action, func(t *testing.T) {
				m, _ := activityModel(t)
				batch := m.loadActivityThoughts(10, data.ThoughtSummaryViewRequest{})().(tea.BatchMsg)
				feedback := activityFeedbackMsg{owner: m.activity.owner}
				switch action {
				case "complete":
					m, _ = m.Update(batch[0]())
				case "collapse":
					m.collapseActivity()
				case "close":
					m.Close()
				case "change Goal":
					m.loadDetail(99)
				case "replace read":
					m.loadActivityEvents(data.GoalEventPageRequest{})
				}
				m, _ = m.Update(feedback)
				if m.activity.showLoading || batch[1]() != nil {
					t.Fatal("retired read left feedback active")
				}
				m.Close()
			})
		}
	})

}

// Activity reads return their data command first, then a feedback timer. Tests
// complete the read directly and deliver timer messages explicitly, without waits.
func activityReadReply(cmd tea.Cmd) tea.Msg {
	reply := cmd()
	if batch, ok := reply.(tea.BatchMsg); ok {
		return batch[0]()
	}
	return reply
}
