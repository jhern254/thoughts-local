package goals

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/tui/listfilter"
)

func TestModel_SubjectPickerSearch(t *testing.T) {
	for _, confirm := range []string{"enter", "down", "tab"} {
		t.Run(confirm+" then Space selects the typed Subject even when its search reply is late", func(t *testing.T) {
			m, _ := testModel(t)
			m.open = true
			m.screen = subjectsView
			m.pickerReturn = detailView
			m.pickerReady = true
			m.available = []data.Subject{{SubjectID: 9, SubjectName: "Created for Goal"}}
			m.selectedIDs = make(map[int64]bool)
			m.refreshPicker()
			m, _ = press(m, "/")
			var pending tea.Cmd
			m, pending = m.Update(tea.PasteMsg{Content: "Created for Goal"})
			// The old results still put Create subject first. A user can finish typing
			// and press Enter/Space before the latest search command has completed.
			if _, ok := m.picker.SelectedItem().(createSubjectRow); !ok {
				t.Fatal("fixture: want Create subject selected before the search reply")
			}
			m, _ = press(m, confirm)
			var refreshed tea.Cmd
			m, refreshed = press(m, "space")
			if !m.selectedIDs[9] || len(m.ids()) != 1 {
				t.Fatalf("selected Subject IDs: got %v, want [9]", m.ids())
			}
			if m.screen != subjectsView || m.picker.SettingFilter() {
				t.Fatal("confirmation should finish search and leave the Subject picker open")
			}

			// Deliver the real old command after selection. It must not replace the
			// checked row with an earlier unchecked copy of that Subject.
			var deliver func(tea.Cmd)
			deliver = func(cmd tea.Cmd) {
				if cmd == nil {
					return
				}
				switch reply := cmd().(type) {
				case tea.BatchMsg:
					for _, child := range reply {
						deliver(child)
					}
				case listfilter.Reply:
					m, _ = m.Update(reply)
				}
			}
			deliver(refreshed)
			deliver(pending)
			row, ok := m.picker.SelectedItem().(subjectRow)
			if !ok || row.item.SubjectID != 9 || !row.selected {
				t.Fatalf("row after late search: got %+v, want checked Subject 9", m.picker.SelectedItem())
			}
		})
	}
	t.Run("confirming no matches keeps the existing reset behavior without selecting a Subject", func(t *testing.T) {
		m, _ := testModel(t)
		m.open = true
		m.screen = subjectsView
		m.pickerReturn = detailView
		m.pickerReady = true
		m.available = []data.Subject{{SubjectID: 9, SubjectName: "Reading"}}
		m.selectedIDs = make(map[int64]bool)
		m.refreshPicker()
		m, _ = press(m, "/")
		m, _ = m.Update(tea.PasteMsg{Content: "no matching Subject"})
		m, _ = press(m, "enter")
		if m.picker.SettingFilter() || m.picker.FilterValue() != "" || len(m.ids()) != 0 || m.screen != subjectsView {
			t.Fatalf("no-match confirmation: filter=%q, selected=%v, screen=%v; want cleared filter, no selection, open picker", m.picker.FilterValue(), m.ids(), m.screen)
		}
	})

}
