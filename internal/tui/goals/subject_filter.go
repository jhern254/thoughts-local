package goals

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/tui/appearance"
)

type subjectFilterOption struct {
	label string
	index int
}

func (o subjectFilterOption) Title() string       { return o.label }
func (o subjectFilterOption) Description() string { return "" }
func (o subjectFilterOption) FilterValue() string { return o.label }

func (m *Model) openSubjectMenu() tea.Cmd {
	options := make([]list.Item, 0, len(m.filterSubjects)+2)
	selected := m.subjectFilter
	for i := range len(m.filterSubjects) + 2 {
		options = append(options, subjectFilterOption{label: m.subjectFilterOptionLabel(i), index: i})
	}
	m.subjectMenu.ResetFilter()
	m.subjectMenuOpen = true
	cmd := m.subjectMenuFilter.SetItems(&m.subjectMenu, options)
	m.subjectMenu.Select(selected)
	return cmd
}

func (m *Model) closeSubjectMenu() {
	m.subjectMenuOpen = false
	m.subjectMenuFilter.Invalidate()
}

func (m *Model) selectSubjectMenuOption() tea.Cmd {
	option, ok := m.subjectMenu.SelectedItem().(subjectFilterOption)
	if !ok {
		return nil
	}
	m.subjectFilter = option.index
	m.closeSubjectMenu()
	return m.applyListControls()
}

func (m *Model) updateSubjectMenu(msg tea.Msg) tea.Cmd {
	if click, ok := msg.(tea.MouseClickMsg); ok && click.Button == tea.MouseLeft {
		// The menu starts below the Subject control and its search row. Entries
		// occupy one terminal row; the Bubbles paginator owns the page offset.
		menuTop := 0
		for _, hit := range m.listControlLayout().hits {
			if hit.focus == subjectControl {
				menuTop = hit.y + 2
				break
			}
		}
		if click.Y == menuTop-1 {
			if m.subjectMenu.SettingFilter() {
				return m.subjectMenu.FilterInput.Focus()
			}
			return m.subjectMenuFilter.Update(&m.subjectMenu, tea.KeyPressMsg{Code: '/', Text: "/"})
		}
		index := click.Y - menuTop
		if index >= 0 && index < m.subjectMenu.Paginator.PerPage {
			index += m.subjectMenu.Paginator.Page * m.subjectMenu.Paginator.PerPage
			if index < len(m.subjectMenu.VisibleItems()) {
				m.subjectMenu.Select(index)
				return m.selectSubjectMenuOption()
			}
		}
		m.closeSubjectMenu()
		return nil
	}
	msg = appearance.ListScrollInput(msg)
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			m.closeSubjectMenu()
			return nil
		case "left", "h":
			if !m.subjectMenu.SettingFilter() && m.subjectMenu.Paginator.Page == 0 {
				m.closeSubjectMenu()
				return nil
			}
		case "enter":
			return m.selectSubjectMenuOption()
		case "tab", "shift+tab":
			m.closeSubjectMenu()
			m.listFocus = goalListControl
			if key.String() == "shift+tab" {
				m.listFocus = statusControl
			}
			return nil
		case "down", "up":
			if key.String() == "down" {
				m.subjectMenu.CursorDown()
			} else {
				m.subjectMenu.CursorUp()
			}
			return nil
		}
	}
	return m.subjectMenuFilter.Update(&m.subjectMenu, msg)
}
