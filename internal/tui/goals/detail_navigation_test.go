package goals

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
)

func TestModel_DetailBackNavigation(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyLeft}, {Code: 'h', Text: "h"}} {
		for _, historyFocused := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s returns from Details with history focus %t", key.String(), historyFocused), func(t *testing.T) {
				m, s := activityModel(t)
				s.item.IsActive = true
				m.activity.focused = historyFocused
				readContext := s.activityContexts[len(s.activityContexts)-1]
				m, cmd := m.Update(key)
				if m.screen != listView || cmd == nil {
					t.Fatal("back did not return to the Goals list")
				}
				if readContext.Err() == nil {
					t.Fatal("back did not cancel the abandoned history read")
				}
				m, _ = m.Update(cmd())
				selected, ok := m.list.SelectedItem().(goalRow)
				if !ok || selected.item.GoalID != 42 {
					t.Fatal("return did not preserve the selected Goal")
				}
			})
		}
	}
	t.Run("Left pages backward and held repeats stop at the newest page", func(t *testing.T) {
		m, s := activityModel(t)
		s.earnedEvents = historyEvents(40, 1000)
		m, _ = m.Update(m.loadActivityEvents(data.GoalEventPageRequest{})())
		m.activity.list.Select(39)
		for m.activity.list.Paginator.Page > 0 {
			page := m.activity.list.Paginator.Page
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft, IsRepeat: true})
			if m.screen != detailView || m.activity.list.Paginator.Page != page-1 {
				t.Fatal("Left left Details instead of paging backward")
			}
		}
		m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyLeft, IsRepeat: true})
		if m.screen != detailView || cmd != nil {
			t.Fatal("held Left crossed the back boundary")
		}
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
		if m.screen != listView {
			t.Fatal("fresh Left at the newest page did not return")
		}
	})
	t.Run("Left collapses an expansion before returning and rejects its late reply", func(t *testing.T) {
		m, _ := activityModel(t)
		m, cmd := press(m, "enter")
		reply := cmd()
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
		if m.screen != detailView || m.activity.expandedEventID != 0 {
			t.Fatal("first Left did not collapse the expansion")
		}
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
		m, _ = m.Update(reply)
		if m.screen != listView || m.activity.expandedEventID != 0 {
			t.Fatal("late expansion reopened Details after returning")
		}
	})
	t.Run("Left cancels Details before its Goal read finishes", func(t *testing.T) {
		m, _ := testModel(t)
		m.open = true
		m.screen = detailView
		reply := m.loadDetail(42)()
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
		m, _ = m.Update(reply)
		if m.screen != listView || m.selected != nil {
			t.Fatal("abandoned Goal read reopened Details")
		}
	})
}
