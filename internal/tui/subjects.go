package tui

import (
	"context"
	"fmt"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/diagnostics"
	"github.com/jhern254/go-thoughts/internal/failure"
	"github.com/jhern254/go-thoughts/internal/logging"
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

type subjectState struct {
	service SubjectService

	list       list.Model
	input      textinput.Model
	selected   *data.Subject
	err        error
	errMessage string
	loading    bool
	listStale  bool
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

func newSubjectState(service SubjectService) subjectState {
	subjectList := list.New(subjectRows(nil), list.NewDefaultDelegate(), defaultWidth, defaultHeight)
	subjectList.Title = "Subjects"
	subjectList.SetStatusBarItemName("subject", "subjects")

	input := textinput.New()
	input.Prompt = "Subject name: "
	input.Placeholder = "What is this about?"

	return subjectState{
		service: service,
		list:    subjectList,
		input:   input,
	}
}

func (m *Model) resizeSubjects(width, height int) {
	m.subjects.list.SetSize(width, max(0, height))
	m.subjects.input.SetWidth(max(0, width-2))
}

func (m Model) openSubjects() (tea.Model, tea.Cmd) {
	m.screen = screenSubjectList
	m.subjects.err = nil
	m.subjects.loading = true
	return m, m.listSubjects()
}

func (m Model) listSubjects() tea.Cmd {
	ctx := m.ctx
	userID := m.user.UserID
	service := m.subjects.service
	return func() tea.Msg {
		subjects, err := service.List(ctx, userID)
		return subjectsListedMsg{subjects: subjects, err: err}
	}
}

func (m Model) createSubject(name string) tea.Cmd {
	ctx := m.ctx
	userID := m.user.UserID
	service := m.subjects.service
	return func() tea.Msg {
		subject, err := service.Create(ctx, userID, name)
		return subjectCreatedMsg{subject: subject, err: err}
	}
}

func (m Model) getSubject(subjectID int64) tea.Cmd {
	ctx := m.ctx
	userID := m.user.UserID
	service := m.subjects.service
	return func() tea.Msg {
		subject, err := service.Get(ctx, userID, subjectID)
		return subjectFoundMsg{subject: subject, err: err}
	}
}

func (m Model) handleSubjectsListed(message subjectsListedMsg) (tea.Model, tea.Cmd) {
	m.subjects.loading = false
	if message.err != nil {
		logSubjectError(m.logger, logging.SubjectList, message.err)
		m.subjects.err = message.err
		m.subjects.errMessage = diagnostics.SubjectMessage(message.err, "Could not list subjects.")
		return m, nil
	}

	m.subjects.err = nil
	m.subjects.listStale = false
	return m, m.subjects.list.SetItems(subjectRows(message.subjects))
}

func (m Model) handleSubjectCreated(message subjectCreatedMsg) (tea.Model, tea.Cmd) {
	m.subjects.loading = false
	if message.err != nil {
		logSubjectError(m.logger, logging.SubjectCreate, message.err)
		m.subjects.err = message.err
		m.subjects.errMessage = diagnostics.SubjectMessage(message.err, "Could not save the subject.")
		return m, m.subjects.input.Focus()
	}

	m.subjects.input.Blur()
	m.subjects.input.Reset()
	m.subjects.err = nil
	m.subjects.selected = message.subject
	m.subjects.listStale = true
	m.logger.Mutation(logging.SubjectCreated, message.subject.SubjectID)
	m.screen = screenSubjectDetail
	return m, nil
}

func (m Model) handleSubjectFound(message subjectFoundMsg) (tea.Model, tea.Cmd) {
	m.subjects.loading = false
	if message.err != nil {
		logSubjectError(m.logger, logging.SubjectGet, message.err)
		m.subjects.err = message.err
		m.subjects.errMessage = diagnostics.SubjectMessage(message.err, "Could not retrieve the subject.")
		return m, nil
	}

	m.subjects.err = nil
	m.subjects.selected = message.subject
	m.screen = screenSubjectDetail
	return m, nil
}

func logSubjectError(logger logging.Logger, operation logging.Operation, err error) {
	if category, emit := failure.Classify(operation, err); emit {
		logger.Failure(operation, category)
	}
}

func (m Model) updateSubjectList(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			if !m.subjects.list.SettingFilter() && !m.subjects.list.IsFiltered() {
				m.screen = screenEntities
				m.subjects.err = nil
				return m, nil
			}
		case "q":
			if !m.subjects.list.SettingFilter() {
				return m, tea.Quit
			}
		case "enter":
			if !m.subjects.list.SettingFilter() && !m.subjects.loading {
				row, ok := m.subjects.list.SelectedItem().(subjectRow)
				if !ok {
					return m, nil
				}
				m.subjects.err = nil
				switch row.kind {
				case subjectRowCreate:
					m.screen = screenSubjectCreate
					m.subjects.input.Reset()
					return m, m.subjects.input.Focus()
				case subjectRowRecord:
					m.subjects.loading = true
					return m, m.getSubject(row.subject.SubjectID)
				}
			}
		}
	}

	var command tea.Cmd
	m.subjects.list, command = m.subjects.list.Update(message)
	return m, command
}

func (m Model) updateSubjectCreate(message tea.Msg) (tea.Model, tea.Cmd) {
	if m.subjects.loading {
		return m, nil
	}

	if key, ok := message.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			m.subjects.input.Blur()
			m.subjects.input.Reset()
			m.subjects.err = nil
			m.screen = screenSubjectList
			return m, nil
		case "enter":
			m.subjects.loading = true
			m.subjects.err = nil
			m.subjects.input.Blur()
			return m, m.createSubject(m.subjects.input.Value())
		}
	}

	var command tea.Cmd
	m.subjects.input, command = m.subjects.input.Update(message)
	return m, command
}

func (m Model) updateSubjectDetail(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "q":
			return m, tea.Quit
		case "esc":
			m.subjects.selected = nil
			m.subjects.err = nil
			m.screen = screenSubjectList
			if m.subjects.listStale {
				m.subjects.loading = true
				return m, m.listSubjects()
			}
			return m, nil
		}
	}
	return m, nil
}

func (m Model) viewSubjectList() string {
	status := ""
	if m.subjects.loading {
		status = "Loading subjects…\n\n"
	} else if m.subjects.err != nil {
		status = fmt.Sprintf("Error: %s\n\n", m.subjects.errMessage)
	}
	return fmt.Sprintf("%s%s\nEsc: entities • q: quit", status, m.subjects.list.View())
}

func (m Model) viewSubjectCreate() string {
	status := ""
	if m.subjects.loading {
		status = "Creating subject…\n\n"
	} else if m.subjects.err != nil {
		status = fmt.Sprintf("Error: %s\n\n", m.subjects.errMessage)
	}
	return fmt.Sprintf("Create subject\n\n%s%s\n\nEnter: create • Esc: cancel", status, m.subjects.input.View())
}

func (m Model) viewSubjectDetail() string {
	if m.subjects.selected == nil {
		return "Subject unavailable\n\nEsc: subjects • q: quit"
	}
	title := "Subject"
	if m.subjects.listStale {
		title = "Created subject"
	}
	return fmt.Sprintf(
		"%s\n\nName: %s\nAdded: %s\n\nEsc: subjects • q: quit",
		title,
		m.subjects.selected.SubjectName,
		m.subjects.selected.CreatedAt.UTC().Format(subjectDateLayout),
	)
}
