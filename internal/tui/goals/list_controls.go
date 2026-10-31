package goals

import (
	"sort"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/tui/appearance"
)

type listControl uint8

const (
	goalListControl listControl = iota
	statusControl
	subjectControl
	priorityControl
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
		if a.IsActive != b.IsActive {
			return a.IsActive
		}
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
	return m.subjectFilterOptionLabel(m.subjectFilter)
}

func (m Model) subjectFilterOptionLabel(index int) string {
	switch index {
	case 0:
		return "All Subjects"
	case 1:
		return "Misc — no live Subjects"
	}
	if index-2 >= len(m.filterSubjects) {
		return "All Subjects"
	}
	subject := m.filterSubjects[index-2]
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
		if selected || m.listFocus == focus && focus != statusControl && focus != priorityControl {
			text = m.listControlStyle(m.listFocus == focus).Render(text)
		}
		line += text
	}
	line = m.list.Styles.Title.Padding(0).Margin(0).Underline(false).Render("Goals")
	if m.width < 60 {
		flush()
	} else {
		line += strings.Repeat(" ", max(1, m.width-ansi.StringWidth(line)-26))
	}
	for i, label := range []string{"All", "Active", "Inactive"} {
		add("["+label+"] ", statusControl, i, m.visibility == i)
	}
	flush()
	layout.rows = append(layout.rows, "")
	add("Subject:", subjectControl, 0, true)
	value := ansi.Truncate(" ["+m.subjectFilterLabel()+" ▾]", max(1, m.width-ansi.StringWidth(line)), "…")
	line += value
	layout.hits[len(layout.hits)-1].width += ansi.StringWidth(value)
	flush()
	if m.subjectMenuOpen {
		search := "/ Search Subjects"
		if m.subjectMenu.SettingFilter() {
			search = m.subjectMenu.FilterInput.View()
		} else if m.subjectMenu.FilterValue() != "" {
			search = "/ " + m.subjectMenu.FilterValue()
		}
		layout.rows = append(layout.rows, ansi.Truncate(search, m.width, "…"))
		layout.rows = append(layout.rows, strings.Split(m.subjectMenu.View(), "\n")...)
		return layout
	}
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
	if m.subjectMenuOpen {
		return true, m.updateSubjectMenu(msg)
	}
	if click, ok := msg.(tea.MouseClickMsg); ok && click.Button == tea.MouseLeft {
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
					return true, m.openSubjectMenu()
				}
			}
		}
		m.listFocus = goalListControl
	}
	if _, ok := msg.(tea.MouseWheelMsg); ok && m.listFocus != goalListControl {
		return true, nil
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
		m.listFocus = listControl((int(m.listFocus) + step + 4) % 4)

		return true, nil
	}
	switch key.String() {
	case "j", "k", "down", "up":
		return m.listFocus != goalListControl, nil
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
			return true, m.openSubjectMenu()
		default:
			return false, nil
		}
		return true, m.applyListControls()
	}
	return false, nil
}

const mutedGoalSelection = "#B7A8CD"

func (m Model) listControlStyle(focused bool) lipgloss.Style {
	style := m.list.Styles.Title.Padding(0).Margin(0).Underline(false)
	if !focused {
		style = style.Background(lipgloss.Color("#403752")).Foreground(lipgloss.Color(mutedGoalSelection))
	}
	return style
}

func goalListDelegate(focused bool) list.DefaultDelegate {
	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(4)
	if !focused {
		muted := lipgloss.Color(mutedGoalSelection)
		delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Foreground(muted).BorderForeground(muted)
		delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Foreground(muted).BorderForeground(muted)
	}
	return delegate
}
