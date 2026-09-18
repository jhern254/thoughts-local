package events

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
)

type eventForm struct {
	open, ending, saving bool
	suspended            bool
	subjects             subjectPicker
	fields               [3]textinput.Model
	// index owns keyboard focus; revision owns asynchronous clipboard input.
	index    int
	revision uint64
	event    data.Event
	err      error
	message  string
}
type formClipboard struct {
	// save identifies the form session; revision identifies its unchanged draft.
	owner          *int
	save, revision uint64
	text           string
	err            error
}

func (formClipboard) eventMessage() {}

func (m *Model) startForm(ending bool) tea.Cmd {
	m.save++
	m.form = eventForm{open: true, ending: ending}
	for i := range m.form.fields {
		m.form.fields[i] = textinput.New()
		m.form.fields[i].CharLimit = 0
	}
	m.form.fields[0].Placeholder = "Activity (optional)"
	m.form.fields[1].Placeholder = "Search subjects"
	m.form.fields[2].SetValue(displaytime.FormatInput(m.now()))
	if ending {
		m.form.event = m.items[m.position.eventIndex]
		m.form.index = 2
	}
	m.resizeForm()
	if ending {
		return m.focusForm()
	}
	return tea.Batch(m.focusForm(), m.loadSubjects())
}
func (m *Model) resizeForm() {
	m.anchorSubjects()
	for i := range m.form.fields {
		m.form.fields[i].SetWidth(max(1, m.width-4))
		m.form.fields[i].SetCursor(m.form.fields[i].Position())
	}
}
func (m *Model) focusForm() tea.Cmd {
	m.anchorSubjects()
	for i := range m.form.fields {
		m.form.fields[i].Blur()
	}
	return m.form.fields[m.form.index].Focus()
}
func (m *Model) updateForm(msg tea.Msg) tea.Cmd {
	if m.form.saving || m.form.suspended {
		return nil
	}
	var text string
	switch input := msg.(type) {
	case formClipboard:
		if input.owner != m.owner || input.save != m.save || input.revision != m.form.revision {
			return nil
		}
		if input.err != nil {
			m.form.err = input.err
			m.form.message = "Could not read clipboard. Draft unchanged."
			m.anchorSubjects()
			return nil
		}
		return m.updateForm(tea.PasteMsg{Content: input.text})
	case tea.PasteMsg:
		text = input.Content
	case tea.KeyPressMsg:
		switch input.String() {
		case "esc":
			m.save++
			m.form = eventForm{}
			return nil
		case "tab", "shift+tab":
			if !m.form.ending {
				step := 1
				if input.String() == "shift+tab" {
					step = -1
				}
				m.form.index = (m.form.index + step + 3) % 3
				m.form.revision++
				return m.focusForm()
			}
			return nil
		case "up", "down":
			if !m.form.ending && m.form.index == 1 {
				if input.String() == "up" {
					m.form.subjects.row--
				} else {
					m.form.subjects.row++
				}
				m.anchorSubjects()
				return nil
			}
		case "ctrl+s", "enter":
			if input.String() == "enter" && !m.form.ending && m.form.index == 1 {
				return m.activateSubject()
			}
			if !m.form.ending && m.form.fields[1].Value() != "" && m.form.subjects.selected == nil {
				m.form.message = "Choose an existing subject or create one."
				m.anchorSubjects()
				return nil
			}
			at, err := displaytime.ParseInput(m.form.fields[2].Value())
			if err != nil {
				m.form.err = err
				m.form.message = displaytime.InputHelp
				m.anchorSubjects()
				return nil
			}
			m.form.saving = true
			for i := range m.form.fields {
				m.form.fields[i].Blur()
			}
			owner, save, ctx, user, service, ending, item, label := m.owner, m.save, m.ctx, m.userID, m.service, m.form.ending, m.form.event, m.form.fields[0].Value()
			if ending {
				return func() tea.Msg {
					saved, err := service.End(ctx, user, item.EventID, item.Version, at)
					return savedMsg{owner, save, saved, true, err}
				}
			}
			subjectID := m.form.subjects.selected
			return func() tea.Msg {
				saved, err := service.Create(ctx, user, label, at, subjectID)
				return savedMsg{owner, save, saved, false, err}
			}
		}
		if key.Matches(input, m.form.fields[m.form.index].KeyMap.Paste) {
			owner, save, revision := m.owner, m.save, m.form.revision
			return func() tea.Msg {
				text, err := clipboard.ReadAll()
				return formClipboard{owner, save, revision, text, err}
			}
		}
		text = input.Text
	}
	// A single-line widget cannot preserve authored multiline/control input.
	// Reject the complete operation before it touches the existing draft.
	if text != "" {
		if !utf8.ValidString(text) || strings.ContainsFunc(text, func(r rune) bool { return unicode.IsControl(r) || r == utf8.RuneError }) {
			m.form.message = "Input rejected: unsupported single-line input. Draft unchanged."
			m.anchorSubjects()
			return nil
		}
		m.form.message = ""
	}
	switch msg.(type) {
	case tea.KeyPressMsg, tea.PasteMsg:
		m.form.revision++
	}
	before := m.form.fields[1].Value()
	var cmd tea.Cmd
	m.form.fields[m.form.index], cmd = m.form.fields[m.form.index].Update(msg)
	if m.form.fields[1].Value() != before {
		m.form.subjects.selected = nil
		m.form.subjects.row, m.form.subjects.top = 0, 0
		m.form.message = ""
	}
	return cmd
}
func (m Model) formView() string {
	title := "Start event"
	body := "Activity\n" + m.form.fields[0].View() + "\n" + m.subjectView() + "\nStart time"
	if m.form.ending {
		title = "End event"
		body = "End time"
	}
	status := m.form.message
	if m.form.saving {
		status = "Saving event…"
	}
	return title + "\n\n" + body + "\n" + m.form.fields[2].View() + "\n\n" + ansi.Wrap(status, m.width, "") + "\n" + m.formHelp()
}

func (m Model) formHelp() string {
	help := "Tab/Shift+Tab: field • Ctrl+S/Enter: save • Esc: cancel"
	if m.width < 52 {
		help = "Tab/Shift+Tab: field\nCtrl+S/Enter: save • Esc: cancel"
	}
	if !m.form.ending && m.form.index == 1 {
		help = "↑/↓: choose • Enter: select/create\nTab/Shift+Tab: field • Ctrl+S: save • Esc: cancel"
		if m.width < 52 {
			help = "↑/↓: choose • Enter: select\nTab/Shift+Tab: field\nCtrl+S: save • Esc: cancel"
		}
	}
	return ansi.Wrap(help, m.width, "")
}
