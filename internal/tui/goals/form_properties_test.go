package goals

import (
	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"testing"
)

func TestModel_FormProperties(t *testing.T) {
	for _, priority := range []string{"high", "normal", "low"} {
		for _, active := range []bool{true, false} {
			t.Run(priority+map[bool]string{true: " active", false: " inactive"}[active], func(t *testing.T) {
				m, s := testModel(t)
				m.open = true
				m.startForm(true)
				if m.priority != "normal" || !m.isActive {
					t.Fatal("incorrect creation defaults")
				}
				m.priority = priority
				m.isActive = active
				m.fields[0].SetValue("New")
				m.fields[1].SetValue("1")
				m.selectedIDs = map[int64]bool{1: true, 2: true}
				m.saveProperties()()
				if s.createInput.Priority != priority || s.createInput.IsActive == nil || *s.createInput.IsActive != active || s.creates != 1 || len(s.ids) != 2 {
					t.Fatalf("creation input: %+v", s.createInput)
				}
				s.item.Priority = priority
				s.item.IsActive = active
				m.selected = &s.item
				m.startForm(false)
				if m.priority != priority || m.isActive != active {
					t.Fatal("edit did not populate persisted values")
				}
				m.saveProperties()()
				if s.input.Priority == nil || *s.input.Priority != priority || s.input.IsActive == nil || *s.input.IsActive != active {
					t.Fatalf("update input: %+v", s.input)
				}
				if s.version != s.item.Version || s.input.TZ != s.item.TZ {
					t.Fatal("lost snapshot settings")
				}
			})
		}
	}
	t.Run("failed creation preserves selectors", func(t *testing.T) {
		m, s := testModel(t)
		m.open = true
		m.startForm(true)
		m.priority = "high"
		m.isActive = false
		m.fields[1].SetValue("1")
		s.err = data.ErrDuplicateRecord
		reply := m.saveProperties()()
		m, _ = m.Update(reply)
		if m.priority != "high" || m.isActive || m.screen != createView {
			t.Fatal("lost failed draft")
		}
	})
}

func TestModel_FormSelectorNavigation(t *testing.T) {
	for _, create := range []bool{true, false} {
		t.Run(map[bool]string{true: "create", false: "edit"}[create], func(t *testing.T) {
			m, s := testModel(t)
			m.open = true
			m.selected = &s.item
			m.startForm(create)
			m.Resize(32, 12)
			for range formPriority - formName {
				m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			}
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
			expected := "normal"
			if create {
				expected = "low"
			}
			if m.priority != expected {
				t.Fatalf("priority got %s, want %s", m.priority, expected)
			}
			active := m.isActive
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			m, _ = press(m, "enter")
			if m.isActive == active {
				t.Fatal("Status did not toggle")
			}
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
			if m.focus != formPriority {
				t.Fatal("Up did not return to Priority")
			}
			m, _ = press(m, "esc")
			if s.creates != 0 || s.updates != 0 {
				t.Fatal("cancel wrote properties")
			}
		})
	}
}
