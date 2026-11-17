package goals

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// A selectable detail can wrap across several terminal rows. Keep those rows
// together when moving focus or scrolling it into view.
type detailFocusRows struct{ first, last int }

func (m Model) detailContent() string {
	content, _ := m.detailContentLayout()
	return content
}

func (m Model) detailContentLayout() (string, []detailFocusRows) {
	if m.selected == nil {
		return "", nil
	}
	item := m.selected
	rows := []string{m.list.Styles.Title.Render("Goals")}
	rows = append(rows, strings.Split(ansi.Hardwrap(item.GoalName, m.width, true), "\n")...)
	targets := make([]detailFocusRows, 0, len(m.live)+1)
	appendSelection := func(text string, index int) {
		first := len(rows)
		rows = append(rows, strings.Split(m.detailContentGroup(text, index), "\n")...)
		targets = append(targets, detailFocusRows{first: first, last: len(rows) - 1})
	}
	appendSelection(goalSummary(*item), 0)
	rows = append(rows, "", m.list.Styles.Title.Render("Subjects"))
	for index, name := range numberedSubjectNames(m.live, m.goalNumbers) {
		appendSelection(name, index+1)
	}
	rows = append(rows, "", fmt.Sprintf("Created %s", item.CreatedAt.Format("Jan 2, 2006")), fmt.Sprintf("Updated %s", item.UpdatedAt.Format("Jan 2, 2006")))
	return strings.Join(rows, "\n"), targets
}

func (m Model) detailContentGroup(content string, index int) string {
	styles := goalListDelegate(!m.activity.focused).Styles
	style := styles.NormalTitle
	if index == m.activity.detailIndex {
		style = styles.SelectedTitle
	}
	width := max(1, m.width-style.GetHorizontalFrameSize())
	return style.Render(ansi.Hardwrap(content, width, true))
}

func (m *Model) moveDetailFocus(step int) bool {
	_, targets := m.detailContentLayout()
	if len(targets) == 0 {
		return false
	}
	next := max(0, min(m.activity.detailIndex+step, len(targets)-1))
	if next == m.activity.detailIndex {
		return false
	}
	m.activity.detailIndex = next
	m.anchorDetailFocus()
	return true
}

func (m *Model) anchorDetailFocus() {
	content, targets := m.detailContentLayout()
	if m.activity.detailIndex >= len(targets) {
		return
	}
	m.body.SetContent(ansi.Hardwrap(content, m.width, true))
	target := targets[m.activity.detailIndex]
	if target.first < m.body.YOffset() {
		m.body.SetYOffset(target.first)
	} else if target.last >= m.body.YOffset()+m.body.Height() {
		// Long Subject names start at their first wrapped row; shorter items
		// reveal their final row without unnecessarily moving earlier content.
		offset := min(target.first, target.last-m.body.Height()+1)
		m.body.SetYOffset(offset)
	}
}

func (m *Model) selectDetailAtRow(row int) {
	_, targets := m.detailContentLayout()
	for index, target := range targets {
		if row >= target.first && row <= target.last {
			m.activity.detailIndex = index
			return
		}
	}
}
