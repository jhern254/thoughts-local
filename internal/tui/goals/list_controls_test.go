package goals

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/tui/listfilter"
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
		m.visibility = 0
		m.applyListControls()
		return m
	}
	t.Run("creation uses n without a New Goal button or extra Tab stop", func(t *testing.T) {
		m := setup(t)
		for _, width := range []int{80, 48} {
			m.Resize(width, 24)
			if strings.Contains(ansi.Strip(m.View()), "New Goal") {
				t.Fatalf("removed New Goal button remains at width %d", width)
			}
		}
		for _, want := range []listControl{statusControl, subjectControl, priorityControl, goalListControl} {
			m, _ = press(m, "tab")
			if m.listFocus != want {
				t.Fatalf("Tab focus got %d, want %d", m.listFocus, want)
			}
		}
		m, _ = press(m, "n")
		if m.screen != createView || m.saving {
			t.Fatal("n did not open an unsaved Goal creation form")
		}
	})

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
		m, _ = press(m, "tab") // Subject
		m = selectSubjectFilter(m, 1)
		if len(m.list.Items()) != 1 || m.list.Items()[0].(goalRow).item.GoalID != 1 {
			t.Fatal("Unassigned matched a literal Misc Subject")
		}
		m = selectSubjectFilter(m, 2)
		if len(m.list.Items()) != 1 || m.list.Items()[0].(goalRow).item.GoalID != 3 {
			t.Fatal("literal Misc Subject did not remain distinct")
		}
		m = selectSubjectFilter(m, 3)
		if len(m.list.Items()) != 0 {
			t.Fatal("Subject and status did not combine")
		}
		m = selectSubjectFilter(m, 2)

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
	t.Run("only Tab moves focus between filters and Goals", func(t *testing.T) {
		for _, focus := range []listControl{statusControl, subjectControl, priorityControl} {
			for _, key := range []tea.KeyPressMsg{{Code: 'j', Text: "j"}, {Code: 'k', Text: "k"}, {Code: tea.KeyDown}, {Code: tea.KeyUp}} {
				m := setup(t)
				m.listFocus = focus
				m, _ = m.Update(key)
				if m.listFocus != focus || m.list.Index() != 0 || m.visibility != 0 || m.subjectFilter != 0 || m.priorityFilter != 0 {
					t.Fatalf("vertical input moved focus or changed selection from control %d", focus)
				}
			}
		}
		m := setup(t)
		m.listFocus = subjectControl
		m, _ = press(m, "tab")
		m, _ = press(m, "tab")
		if m.listFocus != goalListControl {
			t.Fatal("Tab did not reach Goals")
		}
		m, _ = press(m, "j")
		if m.list.Index() != 1 {
			t.Fatal("j did not move through focused Goals")
		}
		m, _ = press(m, "k")
		if m.list.Index() != 0 {
			t.Fatal("k did not move back through focused Goals")
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

func selectSubjectFilter(m Model, index int) Model {
	m.listFocus = subjectControl
	m, _ = press(m, "enter")
	m.subjectMenu.Select(index)
	m, _ = press(m, "enter")
	return m
}

func TestModel_SubjectFilterDropdown(t *testing.T) {
	t.Run("defaults to Active and every priority without a Goal search bar", func(t *testing.T) {
		m, _ := testModel(t)
		m, _ = m.Update(m.Open()())
		if m.visibility != 1 || m.priorityFilter != 0 {
			t.Fatal("default filters must be Active and All priorities")
		}
		if strings.Contains(m.View(), "Search Goals") {
			t.Fatal("Goal search bar remains")
		}
		m, _ = press(m, "/")
		if m.list.SettingFilter() {
			t.Fatal("hidden Goal search is still enabled")
		}
	})
	t.Run("focused controls use the highlight without underline", func(t *testing.T) {
		m, _ := testModel(t)
		m.list.Styles.Title = lipgloss.NewStyle().Background(lipgloss.Color("62")).Underline(true)
		for _, control := range []struct {
			focus listControl
			label string
		}{{statusControl, "[Active] "}, {priorityControl, "[All] "}, {subjectControl, "Subject: [All Subjects ▾]"}} {
			m.listFocus = control.focus
			expected := m.list.Styles.Title.Padding(0).Margin(0).Underline(false).Render(control.label)
			if !strings.Contains(m.View(), expected) {
				t.Fatalf("control %q did not use highlight without underline", control.label)
			}
		}
	})
	t.Run("search replies cannot change a reopened dropdown", func(t *testing.T) {
		m, _ := testModel(t)
		m.open = true
		m.filterSubjects = []data.Subject{{SubjectID: 8, SubjectName: "Reading"}, {SubjectID: 9, SubjectName: "Writing"}}
		m.listFocus = subjectControl
		m, _ = press(m, "enter")
		m.subjectMenu.SetFilterText("Reading")
		cmd := m.subjectMenuFilter.SetItems(&m.subjectMenu, m.subjectMenu.Items())
		reply := cmd()
		m, _ = press(m, "esc")
		m, _ = press(m, "enter")
		m, _ = m.Update(reply)
		if len(m.subjectMenu.VisibleItems()) != 4 {
			t.Fatal("stale search changed the reopened dropdown")
		}
	})
	t.Run("typing a query and selecting uses actual dropdown updates", func(t *testing.T) {
		m, _ := testModel(t)
		m.open = true
		m.filterSubjects = []data.Subject{{SubjectID: 8, SubjectName: "Reading"}, {SubjectID: 9, SubjectName: "Writing"}}
		m.listFocus = subjectControl
		m, _ = press(m, "enter")
		m, _ = press(m, "/")
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
		var cmd tea.Cmd
		m, cmd = m.Update(tea.PasteMsg{Content: "Writing"})
		deliver(cmd)
		if len(m.subjectMenu.VisibleItems()) != 1 {
			t.Fatalf("query matched %d Subjects, want 1", len(m.subjectMenu.VisibleItems()))
		}
		m, _ = press(m, "enter")
		if m.subjectFilter != 3 || m.subjectMenuOpen {
			t.Fatal("typed search did not select Writing")
		}
	})

	t.Run("dropdown scrolls and resizing does not shrink the restored Goal list", func(t *testing.T) {
		m, _ := testModel(t)
		m.open = true
		for i := range 32 {
			m.filterSubjects = append(m.filterSubjects, data.Subject{SubjectID: int64(i + 1), SubjectName: "Subject"})
		}
		m.listFocus = subjectControl
		m, _ = press(m, "enter")
		m.Resize(48, 18)
		for range 40 {
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		}
		if m.subjectMenu.Index() != 33 {
			t.Fatalf("dropdown reached %d, want last option 33", m.subjectMenu.Index())
		}
		for _, row := range strings.Split(ansi.Strip(m.View()), "\n") {
			if ansi.StringWidth(row) > 48 {
				t.Fatalf("dropdown overflows narrow layout: %q", row)
			}
		}
		m, _ = press(m, "esc")
		if got, want := m.list.Height(), 18-len(m.listControlLayout().rows)-3; got != want {
			t.Fatalf("restored list height got %d, want %d", got, want)
		}
	})
	t.Run("mouse opens the dropdown and selects an option", func(t *testing.T) {
		m, _ := testModel(t)
		m.open = true
		var subjectRow int
		for _, hit := range m.listControlLayout().hits {
			if hit.focus == subjectControl {
				subjectRow = hit.y
				m, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: hit.x, Y: hit.y})
				break
			}
		}
		if !m.subjectMenuOpen {
			t.Fatal("Subject click did not open the dropdown")
		}
		m, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 3, Y: subjectRow + 3})
		if m.subjectMenuOpen || m.subjectFilter != 1 {
			t.Fatal("click did not select Misc")
		}
	})

	t.Run("Subject opens a searchable dropdown and Escape preserves the filter", func(t *testing.T) {
		m, _ := testModel(t)
		m.open = true
		m.filterSubjects = []data.Subject{{SubjectID: 8, SubjectName: "Reading"}, {SubjectID: 9, SubjectName: "Writing"}}
		m.listFocus = subjectControl
		m, _ = press(m, "enter")
		if !m.subjectMenuOpen || !strings.Contains(m.View(), "Search Subjects") {
			t.Fatal("Subject did not open a searchable dropdown")
		}
		m.subjectMenu.SetFilterText("Writing")
		m, _ = press(m, "enter")
		if m.subjectMenuOpen || m.subjectFilter != 3 {
			t.Fatal("searched Subject was not selected")
		}
		m, _ = press(m, "enter")
		m.subjectMenu.Select(0)
		m, _ = press(m, "esc")
		if m.subjectMenuOpen || m.subjectFilter != 3 || !m.open {
			t.Fatal("dropdown cancel changed filter or left Goals")
		}
	})
}
