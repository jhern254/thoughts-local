package tui

import (
	"context"
	"fmt"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
)

const (
	createSubjectLabel = "Create subject…"
	subjectDateLayout  = "Jan 2, 2006"
)

type SubjectService interface {
	List(ctx context.Context, userID string) ([]data.Subject, error)
	Create(ctx context.Context, userID, name string) (*data.Subject, error)
	Get(ctx context.Context, userID string, subjectID int64) (*data.Subject, error)
}

type subjectRowKind uint8

const (
	subjectRowCreate subjectRowKind = iota
	subjectRowRecord
)

type subjectRow struct {
	kind    subjectRowKind
	subject data.Subject
}

func (row subjectRow) Title() string {
	if row.kind == subjectRowCreate {
		return createSubjectLabel
	}
	return row.subject.SubjectName
}

func (row subjectRow) Description() string {
	if row.kind == subjectRowCreate {
		return "Add a new subject"
	}
	return ""
}

func (row subjectRow) FilterValue() string {
	return row.Title()
}

type subjectsListedMsg struct {
	subjects []data.Subject
	err      error
}

type subjectCreatedMsg struct {
	subject *data.Subject
	err     error
}

type subjectFoundMsg struct {
	subject *data.Subject
	err     error
}

func subjectRows(subjects []data.Subject) []list.Item {
	rows := make([]list.Item, 0, len(subjects)+1)
	rows = append(rows, subjectRow{kind: subjectRowCreate})
	for _, subject := range subjects {
		rows = append(rows, subjectRow{kind: subjectRowRecord, subject: subject})
	}
	return rows
}

func (m Model) listSubjects() tea.Cmd {
	ctx := m.ctx
	userID := m.user.UserID
	service := m.subjects
	return func() tea.Msg {
		subjects, err := service.List(ctx, userID)
		return subjectsListedMsg{subjects: subjects, err: err}
	}
}

func (m Model) createSubject(name string) tea.Cmd {
	ctx := m.ctx
	userID := m.user.UserID
	service := m.subjects
	return func() tea.Msg {
		subject, err := service.Create(ctx, userID, name)
		return subjectCreatedMsg{subject: subject, err: err}
	}
}

func (m Model) getSubject(subjectID int64) tea.Cmd {
	ctx := m.ctx
	userID := m.user.UserID
	service := m.subjects
	return func() tea.Msg {
		subject, err := service.Get(ctx, userID, subjectID)
		return subjectFoundMsg{subject: subject, err: err}
	}
}

func (m Model) handleSubjectsListed(message subjectsListedMsg) (tea.Model, tea.Cmd) {
	m.subjectLoading = false
	if message.err != nil {
		m.subjectError = message.err
		return m, nil
	}

	m.subjectError = nil
	m.subjectListStale = false
	return m, m.subjectList.SetItems(subjectRows(message.subjects))
}

func (m Model) handleSubjectCreated(message subjectCreatedMsg) (tea.Model, tea.Cmd) {
	m.subjectLoading = false
	if message.err != nil {
		m.subjectError = message.err
		return m, m.subjectInput.Focus()
	}

	m.subjectInput.Blur()
	m.subjectInput.Reset()
	m.subjectError = nil
	m.selectedSubject = message.subject
	m.subjectListStale = true
	m.screen = screenSubjectDetail
	return m, nil
}

func (m Model) handleSubjectFound(message subjectFoundMsg) (tea.Model, tea.Cmd) {
	m.subjectLoading = false
	if message.err != nil {
		m.subjectError = message.err
		return m, nil
	}

	m.subjectError = nil
	m.selectedSubject = message.subject
	m.screen = screenSubjectDetail
	return m, nil
}

func (m Model) updateSubjectList(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			if !m.subjectList.SettingFilter() && !m.subjectList.IsFiltered() {
				m.screen = screenEntities
				m.subjectError = nil
				return m, nil
			}
		case "q":
			if !m.subjectList.SettingFilter() {
				return m, tea.Quit
			}
		case "enter":
			if !m.subjectList.SettingFilter() && !m.subjectLoading {
				row, ok := m.subjectList.SelectedItem().(subjectRow)
				if !ok {
					return m, nil
				}
				m.subjectError = nil
				switch row.kind {
				case subjectRowCreate:
					m.screen = screenSubjectCreate
					m.subjectInput.Reset()
					return m, m.subjectInput.Focus()
				case subjectRowRecord:
					m.subjectLoading = true
					return m, m.getSubject(row.subject.SubjectID)
				}
			}
		}
	}

	var command tea.Cmd
	m.subjectList, command = m.subjectList.Update(message)
	return m, command
}

func (m Model) updateSubjectCreate(message tea.Msg) (tea.Model, tea.Cmd) {
	if m.subjectLoading {
		return m, nil
	}

	if key, ok := message.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			m.subjectInput.Blur()
			m.subjectInput.Reset()
			m.subjectError = nil
			m.screen = screenSubjectList
			return m, nil
		case "enter":
			m.subjectLoading = true
			m.subjectError = nil
			m.subjectInput.Blur()
			return m, m.createSubject(m.subjectInput.Value())
		}
	}

	var command tea.Cmd
	m.subjectInput, command = m.subjectInput.Update(message)
	return m, command
}

func (m Model) updateSubjectDetail(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "q":
			return m, tea.Quit
		case "esc":
			m.selectedSubject = nil
			m.subjectError = nil
			m.screen = screenSubjectList
			if m.subjectListStale {
				m.subjectLoading = true
				return m, m.listSubjects()
			}
			return m, nil
		}
	}
	return m, nil
}

func (m Model) viewSubjectList() string {
	status := ""
	if m.subjectLoading {
		status = "Loading subjects…\n\n"
	} else if m.subjectError != nil {
		status = fmt.Sprintf("Error: %s\n\n", m.subjectError)
	}
	return fmt.Sprintf("%s%s\nEsc: entities • q: quit", status, m.subjectList.View())
}

func (m Model) viewSubjectCreate() string {
	status := ""
	if m.subjectLoading {
		status = "Creating subject…\n\n"
	} else if m.subjectError != nil {
		status = fmt.Sprintf("Error: %s\n\n", m.subjectError)
	}
	return fmt.Sprintf("Create subject\n\n%s%s\n\nEnter: create • Esc: cancel", status, m.subjectInput.View())
}

func (m Model) viewSubjectDetail() string {
	if m.selectedSubject == nil {
		return "Subject unavailable\n\nEsc: subjects • q: quit"
	}
	title := "Subject"
	if m.subjectListStale {
		title = "Created subject"
	}
	return fmt.Sprintf(
		"%s\n\nName: %s\nAdded: %s\n\nEsc: subjects • q: quit",
		title,
		m.selectedSubject.SubjectName,
		m.selectedSubject.CreatedAt.UTC().Format(subjectDateLayout),
	)
}
