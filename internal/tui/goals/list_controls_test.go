package goals

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
)

func TestModel_ListControls(t *testing.T) {
	setup := func(t *testing.T) Model {
		m, _ := testModel(t)
		cmd := m.Open()
		reply := cmd().(result)
		reply.rows = []list.Item{
			goalRow{item: data.Goal{GoalID: 3, GoalName: "Normal", Priority: "normal", IsActive: true}, subjects: []data.Subject{{SubjectID: 9, SubjectName: "Misc"}}},
			goalRow{item: data.Goal{GoalID: 2, GoalName: "High inactive", Priority: "high", IsActive: false}, subjects: []data.Subject{{SubjectID: 8, SubjectName: "Reading"}}},
			goalRow{item: data.Goal{GoalID: 1, GoalName: "High unassigned", Priority: "high", IsActive: true}},
		}
		m, _ = m.Update(reply)
		return m
	}
	t.Run("status Subject and priority controls combine without another read", func(t *testing.T) {
		m := setup(t)
		if got := m.list.Items()[0].(goalRow).item.GoalID; got != 1 {
			t.Fatalf("first Goal got %d, want 1", got)
		}
		m, _ = press(m, "tab") // status
		m, cmd := press(m, "l")
		if cmd != nil {
			t.Fatal("status change started unnecessary async work")
		}
		if m.visibility != 1 || len(m.list.Items()) != 2 {
			t.Fatal("Active did not filter Goals")
		}
		m, _ = press(m, "tab") // search
		m, _ = press(m, "tab") // Subject
		m, _ = press(m, "l")   // Unassigned
		if len(m.list.Items()) != 1 || m.list.Items()[0].(goalRow).item.GoalID != 1 {
			t.Fatal("Unassigned matched a literal Misc Subject")
		}
		m, _ = press(m, "l") // literal Misc
		if len(m.list.Items()) != 1 || m.list.Items()[0].(goalRow).item.GoalID != 3 {
			t.Fatal("literal Misc Subject did not remain distinct")
		}
		m, _ = press(m, "l") // Reading, inactive => empty
		if len(m.list.Items()) != 0 {
			t.Fatal("Subject and status did not combine")
		}
		m, _ = press(m, "h") // literal Misc

		m, _ = press(m, "tab") // priority
		m, _ = press(m, "l")   // High
		if len(m.list.Items()) != 0 {
			t.Fatal("High filter retained Normal Goal")
		}
		m, _ = press(m, "h") // All
		if len(m.list.Items()) != 1 {
			t.Fatal("All priority did not restore matching Goal")
		}
	})
	t.Run("vertical and Vim navigation leave filters unchanged and move Goals", func(t *testing.T) {
		for _, focus := range []listControl{statusControl, subjectControl, priorityControl} {
			for _, key := range []tea.KeyPressMsg{{Code: 'j', Text: "j"}, {Code: tea.KeyDown}} {
				m := setup(t)
				m.listFocus = focus
				m, _ = m.Update(key)
				if m.visibility != 0 || m.subjectFilter != 0 || m.priorityFilter != 0 || m.list.Index() != 1 {
					t.Fatalf("vertical navigation changed filters or failed to move Goal: focus %d", focus)
				}
				m, _ = press(m, "k")
				if m.list.Index() != 0 {
					t.Fatal("k failed to move back up the Goals")
				}
			}
		}
	})
	t.Run("name search retains global priority ordering", func(t *testing.T) {
		m := setup(t)
		m.list.SetFilterText("i")
		var previous string
		for _, item := range m.list.VisibleItems() {
			priority := item.(goalRow).item.Priority
			if previous == "normal" && priority == "high" {
				t.Fatal("search moved Normal ahead of High")
			}
			previous = priority
		}
		if len(m.list.VisibleItems()) != 2 {
			t.Fatalf("search matches got %d, want 2", len(m.list.VisibleItems()))
		}
	})

	t.Run("clickable status buttons and keyboard priority focus", func(t *testing.T) {
		m := setup(t)
		for _, hit := range m.listControlLayout().hits {
			if hit.focus == statusControl && hit.value == 2 {
				m, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: hit.x, Y: hit.y})
				break
			}
		}
		if m.visibility != 2 || len(m.list.Items()) != 1 {
			t.Fatal("Inactive button did not filter")
		}
		m.listFocus = priorityControl
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
		if m.priorityFilter != 1 || m.screen != listView {
			t.Fatal("Right did not select High without opening a Goal")
		}
	})
	t.Run("clicking a filter leaves search editing and keeps its query", func(t *testing.T) {
		m := setup(t)
		m.list.SetFilterText("High")
		m.list.SetFilterState(list.Filtering)
		for _, hit := range m.listControlLayout().hits {
			if hit.focus == statusControl && hit.value == 1 {
				m, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: hit.x, Y: hit.y})
				break
			}
		}
		m, _ = press(m, "l")
		if m.list.SettingFilter() || m.visibility != 2 || m.list.FilterValue() != "High" {
			t.Fatal("filter navigation edited or lost the name query")
		}
	})
	t.Run("confirming a status or priority never advances it or opens a Goal", func(t *testing.T) {
		for _, focus := range []listControl{statusControl, priorityControl} {
			m := setup(t)
			m.listFocus = focus
			m, cmd := press(m, "enter")
			if cmd != nil || m.screen != listView || m.visibility != 0 || m.priorityFilter != 0 {
				t.Fatal("confirm changed a filter or opened a Goal")
			}
		}
	})

	t.Run("visiting an empty search does not consume Back", func(t *testing.T) {
		m := setup(t)
		for range 3 {
			m, _ = press(m, "tab")
		}
		if m.list.FilterState() != list.Unfiltered {
			t.Fatal("empty search remained an applied filter")
		}
		m, cmd := press(m, "esc")
		if m.IsOpen() || cmd == nil {
			t.Fatal("Escape did not leave Goals after visiting empty search")
		}
		if _, ok := cmd().(BackMsg); !ok {
			t.Fatal("Escape did not return Back")
		}
	})

	t.Run("controls stay inside normal and narrow terminal widths", func(t *testing.T) {
		m := setup(t)
		for _, width := range []int{80, 48, 32} {
			m.Resize(width, 24)
			content := ansi.Strip(m.View())
			for _, row := range strings.Split(content, "\n") {
				if ansi.StringWidth(row) > width {
					t.Fatalf("width %d overflow: %q", width, row)
				}
			}
			if !strings.Contains(content, "Priority:") || !strings.Contains(content, "Subject") {
				t.Fatal("missing visible controls")
			}
			if len(strings.Split(content, "\n")) > 24 {
				t.Fatalf("height overflow at width %d", width)
			}
		}
	})
	t.Run("filtering preserves selection when possible and resets when excluded", func(t *testing.T) {
		m := setup(t)
		m.list.Select(2)
		m.visibility = 1
		m.applyListControls()
		if m.list.SelectedItem().(goalRow).item.GoalID != 3 {
			t.Fatal("matching selection moved")
		}
		m.priorityFilter = 1
		m.applyListControls()
		if m.list.SelectedItem().(goalRow).item.GoalID != 1 {
			t.Fatal("excluded selection did not reset")
		}
	})
}
