package goals

import (
	tea "charm.land/bubbletea/v2"
	"strings"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
)

func TestModel_CreateSubject(t *testing.T) {
	t.Run("first picker action opens existing creation and preserves draft membership", func(t *testing.T) {
		m, s := testModel(t)
		s.subjects = []data.Subject{{SubjectID: 1, SubjectName: "Existing"}}
		m.open = true
		m.startForm(true)
		m.fields[0].SetValue("Draft Goal")
		m.selectedIDs[1] = true
		m.screen = subjectsView
		m.pickerReturn = createView
		m, _ = m.Update(m.loadPicker()())
		if !strings.Contains(m.picker.Items()[0].(interface{ Title() string }).Title(), "Create subject") {
			t.Fatal("first choice is not Create subject")
		}
		m, cmd := press(m, "enter")
		request, ok := cmd().(CreateSubjectRequest)
		if !ok || !m.AwaitingSubject(request) {
			t.Fatal("creation did not suspend the picker")
		}
		_, duplicate := press(m, "enter")
		if duplicate != nil {
			t.Fatal("suspended picker requested creation twice")
		}
		m.ReturnFromSubjectCreation(request, &data.Subject{SubjectID: 9, SubjectName: "New Subject"})
		if !m.selectedIDs[1] || !m.selectedIDs[9] || m.fields[0].Value() != "Draft Goal" || m.screen != subjectsView || s.creates != 0 || s.replaces != 0 {
			t.Fatal("return lost draft or saved Goal membership prematurely")
		}
	})
	t.Run("cancel restores picker and retired returns are rejected", func(t *testing.T) {
		m, _ := testModel(t)
		m.open = true
		m.startForm(true)
		m.screen = subjectsView
		m.pickerReturn = createView
		m.pickerReady = true
		m.refreshPicker()
		m, cmd := press(m, "enter")
		request := cmd().(CreateSubjectRequest)
		m.ReturnFromSubjectCreation(request, nil)
		if m.AwaitingSubject(request) || m.screen != subjectsView {
			t.Fatal("cancel failed to resume picker")
		}
		m.Close()
		m.ReturnFromSubjectCreation(request, &data.Subject{SubjectID: 99})
		if m.selectedIDs[99] {
			t.Fatal("retired creation selected a Subject")
		}
	})
}

func TestModel_EmptySubjectConfirmation(t *testing.T) {
	m, _ := testModel(t)
	m.open = true
	m.startForm(true)
	m.screen = subjectsView
	m.pickerReturn = createView
	m.pickerReady = true
	m.refreshPicker()
	m, cmd := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if m.screen != createView || cmd != nil || len(m.ids()) != 0 {
		t.Fatal("empty picker could not confirm without creating a Subject")
	}
}
