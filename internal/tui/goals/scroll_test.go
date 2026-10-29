package goals

import (
	"charm.land/bubbles/v2/list"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
)

func TestModel_SubjectPickerScroll(t *testing.T) {
	m, s := testModel(t)
	m.open = true
	m.startForm(true)
	m.screen = subjectsView
	m.pickerReturn = createView
	m.Resize(48, 18)
	for i := range 32 {
		s.subjects = append(s.subjects, data.Subject{SubjectID: int64(i + 1), SubjectName: fmt.Sprintf("Subject %02d", i+1)})
	}
	m, _ = m.Update(m.loadPicker()())
	before := m.picker.Index()
	m, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m.picker.Index() != before+1 {
		t.Fatal("wheel did not advance the picker")
	}
	for range 40 {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if m.picker.Index() != 32 {
		t.Fatalf("last reachable row got %d, want 32", m.picker.Index())
	}
	m, _ = press(m, "space")
	if !m.selectedIDs[32] {
		t.Fatal("last offscreen Subject could not be selected")
	}
	m, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if m.picker.Index() != 31 || !m.selectedIDs[32] {
		t.Fatal("scrolling lost selection or failed to move back")
	}
}

func TestModel_GoalListAndDetailScroll(t *testing.T) {
	t.Run("wheel and page keys reach later Goals without activating them", func(t *testing.T) {
		m, _ := testModel(t)
		m.open = true
		m.screen = listView
		m.Resize(48, 18)
		rows := []list.Item{createRow{}}
		for i := range 48 {
			rows = append(rows, goalRow{item: data.Goal{GoalID: int64(i + 1), GoalName: fmt.Sprintf("Goal %02d", i+1)}})
		}
		m.filter.SetItems(&m.list, rows)
		m, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
		if m.list.Index() != 1 || m.screen != listView {
			t.Fatal("wheel did not move to the first Goal without opening it")
		}
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
		if m.list.Index() <= 1 {
			t.Fatal("Page Down did not reach a later Goal")
		}
	})
	t.Run("long Goal Details scroll by wheel without changing membership", func(t *testing.T) {
		m, s := testModel(t)
		m.open = true
		m.screen = detailView
		m.selected = &s.item
		m.Resize(48, 18)
		for i := range 32 {
			m.live = append(m.live, data.Subject{SubjectID: int64(i + 1), SubjectName: fmt.Sprintf("Subject %02d", i+1)})
		}
		m.refreshBody()
		for range 40 {
			m, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
		}
		if m.body.YOffset() == 0 || !strings.Contains(m.View(), "Updated") {
			t.Fatal("wheel could not reach the bottom of Goal Details")
		}
		if s.replaces != 0 || s.updates != 0 {
			t.Fatal("scrolling changed Goal data")
		}
	})
}
