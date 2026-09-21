package events

import (
	"bytes"
	tea "charm.land/bubbletea/v2"
	"context"
	"errors"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"strings"
	"testing"
)

func TestModel_SubjectSelection(t *testing.T) {
	t.Run("subject follows activity and an unresolved query blocks save", func(t *testing.T) {
		m, service, _ := fixture(t)
		m, _ = m.Update(eventKey("n"))
		m, _ = m.Update(eventKey("tab"))
		m, _ = m.Update(tea.PasteMsg{Content: "cod"})
		if view := m.View(); !strings.Contains(view, "Subject (optional)") || !strings.Contains(view, "> Create subject…") {
			t.Fatalf("got view:\n%s\nwant focused subject picker with creation first", view)
		}
		m, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
		if cmd != nil || service.creates != 0 || !strings.Contains(m.View(), "Choose an existing subject or create one.") {
			t.Fatal("unresolved subject query must block saving")
		}
	})
}

type subjectReaderStub struct {
	items []data.Subject
	err   error
}

func (s *subjectReaderStub) List(context.Context, string) ([]data.Subject, error) {
	return s.items, s.err
}

func TestModel_SubjectPicker(t *testing.T) {
	t.Run("filters case insensitively selects explicitly and clears edited assignment", func(t *testing.T) {
		m, s, _ := fixture(t)
		m.subjectReader = &subjectReaderStub{items: []data.Subject{{SubjectID: 1, SubjectName: "Coding"}, {SubjectID: 2, SubjectName: "Coding practice"}, {SubjectID: 3, SubjectName: "Reading"}}}
		m, cmd := m.Update(eventKey("n"))
		m, _ = m.Update(cmd().(tea.BatchMsg)[1]())
		m, _ = m.Update(eventKey("tab"))
		m, _ = m.Update(tea.PasteMsg{Content: "OD"})
		view := ansi.Strip(m.View())
		if strings.Contains(view, "Reading") || strings.Index(view, "Create subject…") > strings.Index(view, "Coding") || !strings.Contains(view, "Coding practice") {
			t.Fatalf("got filtered view:\n%s", view)
		}
		m, _ = m.Update(eventKey("down"))
		m, _ = m.Update(eventKey("down"))
		m, _ = m.Update(eventKey("enter"))
		if m.form.index != 2 || !m.form.fields[2].Focused() || m.form.subjects.selected == nil || *m.form.subjects.selected != 2 || m.form.fields[1].Value() != "Coding practice" {
			t.Fatal("selection lost ID, display name, or time focus")
		}
		m, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
		cmd()
		if s.subjectID == nil || *s.subjectID != 2 {
			t.Fatalf("got saved subject %v, want 2", s.subjectID)
		}
		m.form.saving = false
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
		m, _ = m.Update(tea.PasteMsg{Content: " changed"})
		if m.form.subjects.selected != nil {
			t.Fatal("query edit retained stale subject ID")
		}
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'a', Mod: tea.ModCtrl}))
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'k', Mod: tea.ModCtrl}))
		if m.form.fields[1].Value() != "" {
			t.Fatalf("got query %q, want cleared", m.form.fields[1].Value())
		}
		m, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
		cmd()
		if s.subjectID != nil {
			t.Fatalf("got saved subject %v, want nil", s.subjectID)
		}
	})
	for _, state := range []string{"loading", "empty", "no match", "error"} {
		t.Run("creation remains first and available during "+state, func(t *testing.T) {
			m, _, _ := fixture(t)
			reader := &subjectReaderStub{}
			if state == "error" {
				reader.err = errors.New("PRIVATE_SUBJECT_LIST")
			}
			if state == "no match" {
				reader.items = []data.Subject{{SubjectID: 1, SubjectName: "Reading"}}
			}
			m.subjectReader = reader
			var logs bytes.Buffer
			logger, err := logging.New(&logs, "test", "info")
			if err != nil {
				t.Fatal(err)
			}
			m.logger = logger
			m, cmd := m.Update(eventKey("n"))
			if state != "loading" {
				m, _ = m.Update(cmd().(tea.BatchMsg)[1]())
			}
			m, _ = m.Update(eventKey("tab"))
			m, _ = m.Update(tea.PasteMsg{Content: "cod"})
			view := ansi.Strip(m.View())
			if !strings.Contains(view, "> Create subject…") || strings.Contains(view, "PRIVATE_SUBJECT_LIST") || strings.Contains(logs.String(), "PRIVATE_SUBJECT_LIST") {
				t.Fatalf("unsafe or missing creation row:\n%s\nlogs %s", view, logs.String())
			}
			if state == "error" && strings.Count(logs.String(), "\n") != 1 {
				t.Fatalf("got logs %q, want one failure", logs.String())
			}
			m, cmd = m.Update(eventKey("enter"))
			request, ok := cmd().(CreateSubjectRequest)
			if !ok || request.Query != "cod" || !m.AwaitingSubject(request) {
				t.Fatal("creation did not request current query")
			}
		})
	}
	t.Run("scrolls and resizes keeping highlighted Unicode row visible", func(t *testing.T) {
		m, _, _ := fixture(t)
		reader := &subjectReaderStub{}
		for i := 1; i <= 20; i++ {
			reader.items = append(reader.items, data.Subject{SubjectID: int64(i), SubjectName: fmt.Sprintf("界 %02d %s", i, strings.Repeat("界", 30))})
		}
		m.subjectReader = reader
		m, cmd := m.Update(eventKey("n"))
		m, _ = m.Update(cmd().(tea.BatchMsg)[1]())
		m, _ = m.Update(eventKey("tab"))
		m, _ = m.Update(tea.PasteMsg{Content: "界"})
		for i := 0; i < 15; i++ {
			m, _ = m.Update(eventKey("down"))
		}
		for _, size := range [][2]int{{80, 27}, {32, 18}, {60, 22}} {
			m.form.message = ""
			m.Resize(size[0], size[1])
			m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
			view := ansi.Strip(m.View())
			if !strings.Contains(view, "Esc: cancel") || !strings.Contains(view, "Ctrl+S: save") {
				t.Fatalf("missing narrow form help: %s", view)
			}
			if !strings.Contains(view, "> 界 15") || strings.Contains(view, "界 01") {
				t.Fatalf("selection not visible after resize: %s", view)
			}
			if len(strings.Split(view, "\n")) > size[1] {
				t.Fatalf("got %d rows, want <= %d", len(strings.Split(view, "\n")), size[1])
			}
			for _, row := range strings.Split(view, "\n") {
				if ansi.StringWidth(row) > size[0] {
					t.Fatalf("got row width %d, want <= %d: %q", ansi.StringWidth(row), size[0], row)
				}
			}
		}
	})
	t.Run("creation cancellation retains selected ID draft cursor and timeline position", func(t *testing.T) {
		m, _, _ := fixture(t)
		m.subjectReader = &subjectReaderStub{items: []data.Subject{{SubjectID: 7, SubjectName: "Coding"}}}
		m, cmd := m.Update(eventKey("n"))
		m, _ = m.Update(cmd().(tea.BatchMsg)[1]())
		m, _ = m.Update(tea.PasteMsg{Content: "draft activity"})
		m, _ = m.Update(eventKey("tab"))
		m, _ = m.Update(eventKey("down"))
		m, _ = m.Update(eventKey("enter"))
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
		before := m.form
		position := m.position
		m, cmd = m.Update(eventKey("enter"))
		request := cmd().(CreateSubjectRequest)
		m.ReturnFromSubjectCreation(request, nil)
		for i := range before.fields {
			if m.form.fields[i].Value() != before.fields[i].Value() || m.form.fields[i].Position() != before.fields[i].Position() {
				t.Fatalf("field %d changed after cancellation", i)
			}
		}
		if m.form.subjects.selected != before.subjects.selected || m.form.subjects.row != before.subjects.row || m.form.subjects.top != before.subjects.top || m.form.index != 1 || !m.form.fields[1].Focused() || m.position != position {
			t.Fatal("cancel changed selection or navigation")
		}
	})

	t.Run("old list replies cannot overwrite created subject or a new draft", func(t *testing.T) {
		m, _, _ := fixture(t)
		m, _ = m.Update(eventKey("n"))
		old := m.loadSubjects()()
		m, _ = m.Update(eventKey("tab"))
		m, cmd := m.Update(eventKey("enter"))
		request := cmd().(CreateSubjectRequest)
		m.ReturnFromSubjectCreation(request, &data.Subject{SubjectID: 42, SubjectName: "Created"})
		m, _ = m.Update(old)
		if m.form.subjects.selected == nil || *m.form.subjects.selected != 42 || len(m.form.subjects.items) != 1 {
			t.Fatal("old list replaced created selection")
		}
		m, _ = m.Update(eventKey("esc"))
		m, _ = m.Update(eventKey("n"))
		before := m.form.fields[1].Value()
		m, _ = m.Update(old)
		if m.form.fields[1].Value() != before || !m.form.subjects.loading {
			t.Fatal("old list changed new draft")
		}
		if cmd := m.ReturnFromSubjectCreation(request, &data.Subject{SubjectID: 99, SubjectName: "stale"}); cmd != nil || m.form.subjects.selected != nil {
			t.Fatal("old creation changed new draft")
		}
	})
}

func TestModel_EventFormResize(t *testing.T) {
	t.Run("resizing preserves input and cursor while keeping the field visible", func(t *testing.T) {
		m, _, _ := fixture(t)
		m, _ = m.Update(eventKey("n"))
		m, _ = m.Update(tea.PasteMsg{Content: strings.Repeat("界", 30)})
		cursor := m.form.fields[0].Position()
		m.Resize(32, 18)
		for _, row := range strings.Split(m.View(), "\n") {
			if width := ansi.StringWidth(row); width > 32 {
				t.Fatalf("got row width %d, want <=32: %q", width, row)
			}
		}
		if m.form.fields[0].Position() != cursor || m.form.fields[0].Value() != strings.Repeat("界", 30) {
			t.Fatal("resize changed input or cursor")
		}
	})
}
