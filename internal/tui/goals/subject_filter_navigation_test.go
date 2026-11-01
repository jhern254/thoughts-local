package goals

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
)

func TestModel_SubjectFilterPageNavigation(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyLeft}, {Code: 'h', Text: "h"}} {
		t.Run(key.String()+" visits earlier pages and stays on the first page", func(t *testing.T) {
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
			if !m.subjectMenuOpen || !m.open || m.subjectMenu.Paginator.Page != 0 {
				t.Fatal("Left from the first picker page should stay in the picker")
			}
		})
	}
}

func TestModel_SubjectFilterLayout(t *testing.T) {
	t.Run("search has a blank row beneath it and the list fills the available height", func(t *testing.T) {
		m, _ := testModel(t)
		m.open = true
		for i := range 64 {
			m.filterSubjects = append(m.filterSubjects, data.Subject{SubjectID: int64(i + 1), SubjectName: fmt.Sprintf("Subject %02d", i+1)})
		}
		m.listFocus = subjectControl
		m, _ = press(m, "enter")
		for _, size := range []struct{ width, height int }{{80, 24}, {48, 18}, {32, 12}, {100, 40}} {
			m.Resize(size.width, size.height)
			m.subjectMenu.Select(0)
			rows := strings.Split(ansi.Strip(m.View()), "\n")
			search := -1
			for i, row := range rows {
				if row == "/ Search..." {
					search = i
					break
				}
			}
			if search < 0 {
				t.Fatal("missing / Search... placeholder")
			}
			if rows[search+1] != "" || !strings.Contains(rows[search+2], "All Subjects") {
				t.Fatal("search must have one blank row before the first Subject")
			}
			if got := len(rows); got != size.height-1 {
				t.Fatalf("%dx%d picker has %d rows, want %d", size.width, size.height, got, size.height-1)
			}
			for _, row := range rows {
				if ansi.StringWidth(row) > size.width {
					t.Fatalf("picker row overflows %d columns: %q", size.width, row)
				}
			}
			m.subjectMenu.Select(len(m.subjectMenu.Items()) - 1)
			if !strings.Contains(ansi.Strip(m.View()), "Subject 64") {
				t.Fatal("last Subject is not visible after paging")
			}
		}
	})
}
