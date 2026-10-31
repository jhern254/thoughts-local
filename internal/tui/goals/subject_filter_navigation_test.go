package goals

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
)

func TestModel_SubjectFilterPageNavigation(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyLeft}, {Code: 'h', Text: "h"}} {
		t.Run(key.String()+" visits earlier pages before returning back", func(t *testing.T) {
			m, _ := testModel(t)
			m.open = true
			m.Resize(48, 18)
			for i := range 32 {
				m.filterSubjects = append(m.filterSubjects, data.Subject{SubjectID: int64(i + 1), SubjectName: fmt.Sprintf("Subject %02d", i+1)})
			}
			m.listFocus = subjectControl
			m, _ = press(m, "enter")
			m.subjectMenu.Select(len(m.subjectMenu.Items()) - 1)
			if m.subjectMenu.Paginator.Page < 2 {
				t.Fatal("fixture must span at least three pages")
			}
			for m.subjectMenu.Paginator.Page > 0 {
				previous := m.subjectMenu.Paginator.Page
				m, _ = m.Update(key)
				if !m.subjectMenuOpen || m.subjectMenu.Paginator.Page != previous-1 {
					t.Fatalf("Left got open %t page %d, want open picker on page %d", m.subjectMenuOpen, m.subjectMenu.Paginator.Page, previous-1)
				}
			}
			m, _ = m.Update(key)
			if m.subjectMenuOpen || !m.open || m.listFocus != subjectControl {
				t.Fatal("Left from the first page did not return to the Subject filter")
			}
		})
	}
}
