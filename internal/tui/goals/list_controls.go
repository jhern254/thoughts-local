package goals

import (
	"sort"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/tui/appearance"
)

type listControl uint8

const (
	goalListControl listControl = iota
	statusControl
	searchControl
	subjectControl
	priorityControl
	createControl
)

type controlHit struct {
	focus              listControl
	value, x, y, width int
}
type controlLayout struct {
	rows []string
	hits []controlHit
}

func (m *Model) collectFilterSubjects() {
	var selectedID int64
	if m.subjectFilter >= 2 && m.subjectFilter-2 < len(m.filterSubjects) {
		selectedID = m.filterSubjects[m.subjectFilter-2].SubjectID
	}
	seen := make(map[int64]bool)
	m.filterSubjects = nil
	for _, row := range m.listRows {
		if row, ok := row.(goalRow); ok {
			for _, subject := range row.subjects {
				if !seen[subject.SubjectID] {
					seen[subject.SubjectID] = true
					m.filterSubjects = append(m.filterSubjects, subject)
				}
			}
		}
	}
	sort.Slice(m.filterSubjects, func(i, j int) bool { return m.filterSubjects[i].SubjectName < m.filterSubjects[j].SubjectName })
	if m.subjectFilter >= 2 {
		m.subjectFilter = 0
		for i, subject := range m.filterSubjects {
			if subject.SubjectID == selectedID {
				m.subjectFilter = i + 2
			}
		}
	}
}

func (m Model) matchesSubject(row goalRow) bool {
	if m.subjectFilter == 0 {
		return true
	}
	if m.subjectFilter == 1 {
		return len(row.subjects) == 0
	}
	if m.subjectFilter-2 >= len(m.filterSubjects) {
		return false
	}
	id := m.filterSubjects[m.subjectFilter-2].SubjectID
	for _, subject := range row.subjects {
		if subject.SubjectID == id {
			return true
		}
	}
	return false
}

func (m *Model) applyListControls() tea.Cmd {
	var selectedID int64
	if row, ok := m.list.SelectedItem().(goalRow); ok {
		selectedID = row.item.GoalID
	}
	if m.selected != nil {
		selectedID = m.selected.GoalID
		m.selected = nil
	}
	rows := make([]list.Item, 0, len(m.listRows))
	priorities := []string{"", "high", "normal", "low"}
	for _, item := range m.listRows {
		row, ok := item.(goalRow)
		if !ok {
			continue
		}
		if m.visibility == 1 && !row.item.IsActive || m.visibility == 2 && row.item.IsActive {
			continue
		}
		if !m.matchesSubject(row) || m.priorityFilter > 0 && row.item.Priority != priorities[m.priorityFilter] {
			continue
		}
		rows = append(rows, row)
	}
	rank := func(priority string) int {
		switch priority {
		case "high":
			return 0
		case "normal":
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i].(goalRow).item, rows[j].(goalRow).item
		if rank(a.Priority) != rank(b.Priority) {
			return rank(a.Priority) < rank(b.Priority)
		}
		return a.GoalID < b.GoalID
	})
	cmd := m.filter.SetItems(&m.list, rows)
	m.list.Select(0)
	for i, item := range rows {
		if item.(goalRow).item.GoalID == selectedID {
			m.list.Select(i)
			break
		}
	}
	return cmd
}

func (m Model) subjectFilterLabel() string {
	switch m.subjectFilter {
	case 0:
		return "All Subjects"
	case 1:
		return "Misc — no live Subjects"
	}
	if m.subjectFilter-2 >= len(m.filterSubjects) {
		return "All Subjects"
	}
	subject := m.filterSubjects[m.subjectFilter-2]
	if subject.SubjectName == "Misc" {
		return "Misc (Subject)"
	}
	return subject.SubjectName
}

// Layout and hit targets share terminal-cell coordinates, including wrapped controls.
func (m Model) listControlLayout() controlLayout {
	layout := controlLayout{}
	line := ""
	flush := func() { layout.rows = append(layout.rows, line); line = "" }
	add := func(text string, focus listControl, value int, selected bool) {
		text = ansi.Truncate(text, m.width, "…")
		width := ansi.StringWidth(text)
		if ansi.StringWidth(line)+width > m.width {
			flush()
		}
		x := ansi.StringWidth(line)
		layout.hits = append(layout.hits, controlHit{focus, value, x, len(layout.rows), width})
		if selected {
			text = m.list.Styles.Title.Padding(0).Margin(0).Underline(m.listFocus == focus).Render(text)
		} else if m.listFocus == focus && focus != statusControl && focus != priorityControl {
			text = m.list.Styles.Title.Padding(0).Margin(0).Underline(true).Render(text)
		}
		line += text
	}
	line = m.list.Styles.Title.Padding(0).Margin(0).Render("Goals")
	if m.width < 60 {
		line += strings.Repeat(" ", max(1, m.width-ansi.StringWidth(line)-12))
		add("[+ New Goal]", createControl, 0, m.listFocus == createControl)
		flush()
	} else {
		line += strings.Repeat(" ", max(1, m.width-ansi.StringWidth(line)-38))
	}
	for i, label := range []string{"All", "Active", "Inactive"} {
		add("["+label+"] ", statusControl, i, m.visibility == i)
	}
	if m.width >= 60 {
		add("[+ New Goal]", createControl, 0, m.listFocus == createControl)
	}
	flush()
	layout.rows = append(layout.rows, "")
	search := "Search Goals: / "
	if m.list.SettingFilter() {
		search = "Search Goals: " + m.list.FilterInput.View()
	} else if m.list.FilterValue() != "" {
		search = "Search Goals: " + m.list.FilterValue()
	}
	searchWidth := m.width
	if m.width >= 80 {
		searchWidth = m.width/2 - 2
	}
	add(ansi.Truncate(search, searchWidth, "…"), searchControl, 0, m.listFocus == searchControl && !m.list.SettingFilter())
	if m.width >= 80 {
		line += strings.Repeat(" ", max(2, m.width/2-ansi.StringWidth(line)))
	} else {
		flush()
	}
	add(ansi.Truncate("Subject: [← "+m.subjectFilterLabel()+" →]", m.width-ansi.StringWidth(line), "…"), subjectControl, 0, m.listFocus == subjectControl)
	flush()
	layout.rows = append(layout.rows, strings.Repeat(appearance.Border(m.borderWeight).Top, m.width))
	line = "Priority: "
	for i, label := range []string{"All", "High", "Normal", "Low"} {
		add("["+label+"] ", priorityControl, i, m.priorityFilter == i)
	}
	flush()
	layout.rows = append(layout.rows, "")
	return layout
}

func (m *Model) updateListControls(msg tea.Msg) (bool, tea.Cmd) {
	if click, ok := msg.(tea.MouseClickMsg); ok && click.Button == tea.MouseLeft {
		m.blurGoalSearch()
		for _, hit := range m.listControlLayout().hits {
			if click.Y == hit.y && click.X >= hit.x && click.X < hit.x+hit.width {
				m.listFocus = hit.focus
				switch hit.focus {
				case statusControl:
					m.visibility = hit.value
					return true, m.applyListControls()
				case priorityControl:
					m.priorityFilter = hit.value
					return true, m.applyListControls()
				case subjectControl:
					m.subjectFilter = (m.subjectFilter + 1) % (len(m.filterSubjects) + 2)
					return true, m.applyListControls()
				case searchControl:
					return true, m.focusGoalSearch()
				case createControl:
					m.startForm(true)
					return true, m.fields[0].Focus()
				}
			}
		}
		m.listFocus = goalListControl
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return false, nil
	}
	if key.String() == "tab" || key.String() == "shift+tab" {
		step := 1
		if key.String() == "shift+tab" {
			step = -1
		}
		m.listFocus = listControl((int(m.listFocus) + step + 6) % 6)
		m.blurGoalSearch()
		if m.listFocus == searchControl {
			return true, m.focusGoalSearch()
		}
		return true, nil
	}
	if m.list.SettingFilter() {
		return false, nil
	}
	switch key.String() {
	case "j", "k", "down", "up":
		m.listFocus = goalListControl
		return false, nil
	case "/":
		m.listFocus = searchControl
		return true, m.focusGoalSearch()
	case "h", "l", "left", "right", "enter", "space":
		direction := 1
		if key.String() == "h" || key.String() == "left" {
			direction = -1
		}
		switch m.listFocus {
		case statusControl:
			if key.String() == "enter" || key.String() == "space" {
				return true, nil
			}
			m.visibility = (m.visibility + direction + 3) % 3
		case priorityControl:
			if key.String() == "enter" || key.String() == "space" {
				return true, nil
			}
			m.priorityFilter = (m.priorityFilter + direction + 4) % 4
		case subjectControl:
			count := len(m.filterSubjects) + 2
			m.subjectFilter = (m.subjectFilter + direction + count) % count
		case searchControl:
			return true, m.focusGoalSearch()
		case createControl:
			if key.String() != "enter" && key.String() != "space" {
				return true, nil
			}
			m.startForm(true)
			return true, m.fields[0].Focus()
		default:
			return false, nil
		}
		return true, m.applyListControls()
	}
	return false, nil
}

func (m *Model) focusGoalSearch() tea.Cmd {
	m.listFocus = searchControl
	if m.list.FilterState() != list.Filtering {
		return m.filter.Update(&m.list, tea.KeyPressMsg{Code: '/', Text: "/"})
	}
	return m.list.FilterInput.Focus()
}

func (m *Model) blurGoalSearch() {
	if !m.list.SettingFilter() {
		return
	}
	if m.list.FilterValue() == "" {
		m.list.ResetFilter()
		m.filter.Invalidate()
		return
	}
	m.list.SetFilterState(list.FilterApplied)
	m.list.FilterInput.Blur()
}
