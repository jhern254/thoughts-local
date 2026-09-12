package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
)

func TestModel_SubjectListCount(t *testing.T) {
	for _, tt := range []struct {
		name, query, want string
		records           int
		createVisible     bool
	}{
		{"empty", "", "0 subjects", 0, true},
		{"one record", "", "1 subject", 1, true},
		{"two records", "", "2 subjects", 2, true},
		{"matching records across pages without Create", "alpha", "2 subjects", 2, false},
		{"only Create matches", "Create", "0 subjects", 2, true},
		{"zero matches", "zzzz", "0 subjects", 2, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := newRootTestModel()
			m.screen = screenSubjectList
			records := []data.Subject{{SubjectID: 1, SubjectName: "alpha one"}, {SubjectID: 2, SubjectName: "alpha two"}}
			m.subjects.list.SetItems(subjectRows(records[:tt.records]))
			m, _ = rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 14})
			if tt.query != "" {
				m.subjects.list.FilterInput.SetVirtualCursor(false)
				m, _ = rootUpdate(m, runeKey('/'))
				var cmd tea.Cmd
				m, cmd = rootUpdate(m, tea.PasteMsg{Content: tt.query})
				m, _ = rootUpdate(m, rootFilterReply(t, cmd))
			}
			if tt.query == "alpha" && m.subjects.list.Paginator.TotalPages < 2 {
				t.Fatal("want matching records spread across pages")
			}
			if got := m.View().Content; !strings.Contains(got, "\n"+tt.want+"\n") {
				t.Fatalf("got view %q, want count %q", got, tt.want)
			}
			if tt.createVisible {
				if tt.query != "" {
					m, _ = rootUpdate(m, enterKey()) // Accept the filter before selecting Create.
				}
				m, _ = rootUpdate(m, enterKey())
				if m.screen != screenSubjectCreate {
					t.Fatal("Create is not selectable")
				}
			}
		})
	}
	t.Run("count preserves loading and error feedback", func(t *testing.T) {
		m := newRootTestModel()
		m.resizeSubjects(80, 14)
		m.subjects.loading = true
		if got := m.viewSubjectList(); !strings.Contains(got, "Loading subjects…") || !strings.Contains(got, "\n0 subjects\n") {
			t.Fatalf("got view %q, want loading feedback and record count", got)
		}
		if got := strings.Count(m.viewSubjectList(), "\n") + 1; got > 14 {
			t.Fatalf("got %d rendered lines, want at most 14", got)
		}
		m.subjects.loading = false
		m.subjects.err = errors.New("failed")
		m.subjects.errMessage = "Could not load subjects."
		if got := m.viewSubjectList(); !strings.Contains(got, m.subjects.errMessage) || !strings.Contains(got, "\n0 subjects\n") {
			t.Fatalf("got view %q, want error feedback and record count", got)
		}
	})
}
