package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/diagnostics"
	"github.com/jhern254/go-thoughts/internal/logging"
)

type subjectUpdatedMsg struct {
	subject *data.Subject
	err     error
}

type subjectDeletedMsg struct {
	subjectID int64
	err       error
}

func (m Model) saveSubject(name string) tea.Cmd {
	ctx := m.ctx
	userID := m.user.UserID
	service := m.subjects.service
	id := m.subjects.selected.SubjectID
	return func() tea.Msg {
		item, err := service.Update(ctx, userID, id, name)
		return subjectUpdatedMsg{subject: item, err: err}
	}
}

func (m Model) deleteSubject() tea.Cmd {
	ctx := m.ctx
	userID := m.user.UserID
	service := m.subjects.service
	id := m.subjects.selected.SubjectID
	return func() tea.Msg {
		err := service.Delete(ctx, userID, id)
		return subjectDeletedMsg{subjectID: id, err: err}
	}
}

func (m Model) handleSubjectUpdated(message subjectUpdatedMsg) (tea.Model, tea.Cmd) {
	m.subjects.loading = false
	if message.err != nil {
		logSubjectError(m.logger, logging.SubjectUpdate, message.err)
		m.subjects.err = message.err
		m.subjects.errMessage = diagnostics.SubjectMessage(message.err, "Could not save the subject.")
		return m, m.subjects.input.Focus()
	}
	m.subjects.err = nil
	m.subjects.input.Blur()
	m.subjects.input.Reset()
	m.subjects.selected = message.subject
	m.subjects.detailTitle = "Updated subject"
	m.subjects.listStale = true
	m.screen = screenSubjectDetail
	m.logger.Mutation(logging.SubjectUpdated, message.subject.SubjectID)
	return m, nil
}

func (m Model) handleSubjectDeleted(message subjectDeletedMsg) (tea.Model, tea.Cmd) {
	m.subjects.loading = false
	if message.err != nil {
		logSubjectError(m.logger, logging.SubjectDelete, message.err)
		m.subjects.err = message.err
		m.subjects.errMessage = diagnostics.SubjectMessage(message.err, "Could not delete the subject.")
		return m, nil
	}
	m.subjects.err = nil
	m.subjects.selected = nil
	m.subjects.listStale = true
	m.subjects.loading = true
	m.screen = screenSubjectList
	m.logger.Mutation(logging.SubjectDeleted, message.subjectID)
	return m, m.listSubjects()
}

func (m Model) updateSubjectEdit(message tea.Msg) (tea.Model, tea.Cmd) {
	if m.subjects.loading {
		return m, nil
	}
	if key, ok := message.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			m.subjects.input.Blur()
			m.subjects.input.Reset()
			m.subjects.err = nil
			m.screen = screenSubjectDetail
			return m, nil
		case "enter":
			if m.subjects.selected == nil {
				return m, nil
			}
			m.subjects.loading = true
			m.subjects.err = nil
			m.subjects.input.Blur()
			return m, m.saveSubject(m.subjects.input.Value())
		}
	}
	var cmd tea.Cmd
	m.subjects.input, cmd = m.subjects.input.Update(message)
	return m, cmd
}

func (m Model) updateSubjectDelete(message tea.Msg) (tea.Model, tea.Cmd) {
	if m.subjects.loading {
		return m, nil
	}
	if key, ok := message.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc", "n":
			m.subjects.err = nil
			m.screen = screenSubjectDetail
		case "y":
			if m.subjects.selected == nil {
				return m, nil
			}
			m.subjects.loading = true
			m.subjects.err = nil
			return m, m.deleteSubject()
		}
	}
	return m, nil
}

func (m Model) viewSubjectEdit() string {
	status := ""
	if m.subjects.loading {
		status = "Saving subject…\n\n"
	} else if m.subjects.err != nil {
		status = fmt.Sprintf("Error: %s\n\n", m.subjects.errMessage)
	}
	return fmt.Sprintf("Edit subject\n\n%s%s\n\nEnter: save • Esc: cancel", status, m.subjects.input.View())
}

func (m Model) viewSubjectDelete() string {
	if m.subjects.selected == nil {
		return "Subject unavailable\n\nEsc: subject"
	}
	status := ""
	if m.subjects.loading {
		status = "Deleting subject…\n\n"
	} else if m.subjects.err != nil {
		status = fmt.Sprintf("Error: %s\n\n", m.subjects.errMessage)
	}
	return fmt.Sprintf("Delete subject\n\n%sDelete %q?\nIt will disappear from subjects. Existing thoughts will be kept.\n\ny: delete • n/Esc: cancel", status, m.subjects.selected.SubjectName)
}
