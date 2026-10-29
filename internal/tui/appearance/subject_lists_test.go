package appearance

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
)

type subjectListItem struct{ title, description string }

func (item subjectListItem) Title() string       { return item.title }
func (item subjectListItem) Description() string { return item.description }
func (item subjectListItem) FilterValue() string { return "Programming" }
func TestSubjectListView(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {48, 18}, {80, 14}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			var rendered [2][]string
			for i, checkboxes := range []bool{false, true} {
				title := "Programming 界"
				if checkboxes {
					title = "[x] " + title
				}
				model := list.New([]list.Item{subjectListItem{title, "Metadata"}}, SubjectListDelegate(checkboxes), size[0], size[1]-SubjectListReservedRows(size[1]))
				ConfigureSubjectList(&model)
				model.Title = "Subjects"
				view := SubjectListView(model, size[0], size[1], data.BorderHeavy, "Context", "Details", "1 subject", "Enter: open · Esc: back", "↑/↓: move · /: search")
				rendered[i] = strings.Split(ansi.Strip(view), "\n")
				if len(rendered[i]) > size[1] {
					t.Fatalf("view rows got %d, want at most %d: %q", len(rendered[i]), size[1], view)
				}
				if !strings.Contains(view, "━") {
					t.Fatal("divider did not respect heavy borders")
				}
				for _, row := range rendered[i] {
					if ansi.StringWidth(row) > size[0] {
						t.Fatalf("row exceeds %d cells: %q", size[0], row)
					}
				}
			}
			var positions [2][2]int
			for i, rows := range rendered {
				for y, row := range rows {
					if x := strings.Index(row, "Programming"); x >= 0 {
						positions[i] = [2]int{ansi.StringWidth(row[:x]), y}
						break
					}
				}
			}
			if positions[0] != positions[1] || positions[0][0] != 6 {
				t.Fatalf("Subject name positions got %v, want equal columns at cell 6", positions)
			}
		})
	}
}
