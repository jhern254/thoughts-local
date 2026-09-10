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
	"github.com/jhern254/go-thoughts/internal/tui/listfilter"
)

const (
	createSubjectLabel = "Create subject…"
	subjectDateLayout  = "Jan 2, 2006"
)

type SubjectService interface {
	Update(ctx context.Context, userID string, subjectID int64, name string) (*data.Subject, error)
	Delete(ctx context.Context, userID string, subjectID int64) error
	List(ctx context.Context, userID string) ([]data.Subject, error)
	Create(ctx context.Context, userID, name string) (*data.Subject, error)
	Get(ctx context.Context, userID string, subjectID int64) (*data.Subject, error)
}

type MetricsService interface {
	ThoughtCountsBySubject(context.Context, string) ([]data.SubjectThoughtCount, error)
}

type subjectState struct {
	service SubjectService
	filter  listfilter.Scope

	list        list.Model
	input       textinput.Model
	selected    *data.Subject
	err         error
	errMessage  string
	loading     bool
	listStale   bool
	detailTitle string
}

type subjectRowKind uint8

const (
	subjectRowCreate subjectRowKind = iota
	subjectRowRecord
)

type subjectRow struct {
	kind    subjectRowKind
	subject data.Subject
	count   *int64
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
	if row.count == nil {
		return "Thought count unavailable"
	}
	if *row.count == 1 {
		return "1 thought"
	}
	return fmt.Sprintf("%d thoughts", *row.count)
}

func (row subjectRow) FilterValue() string {
	return row.Title()
}

type subjectsListedMsg struct {
	subjects []data.Subject
	err      error
	counts   []data.SubjectThoughtCount
	countErr error
}

type subjectCreatedMsg struct {
	subject *data.Subject
	err     error
}

type subjectFoundMsg struct {
	subject *data.Subject
	err     error
}

type subjectUpdatedMsg struct {
	subject *data.Subject
	err     error
}

type subjectDeletedMsg struct {
	subjectID int64
	err       error
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
	subjectList := list.New(subjectRows(nil), list.NewDefaultDelegate(), defaultWidth, max(0, defaultHeight-4))
	subjectList.Title = "Subjects"
	subjectList.SetShowStatusBar(false)

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
	// Reserve feedback (two lines), the record count, and navigation help.
	m.subjects.list.SetSize(width, max(0, height-4))
	m.subjects.input.SetWidth(max(0, width-2))
}

func (m Model) openSubjects() (tea.Model, tea.Cmd) {
	m.subjects.filter.Invalidate()
	m.screen = screenSubjectList
	m.subjects.err = nil
	m.subjects.loading = true
	return m, m.listSubjects()
}

func (m Model) listSubjects() tea.Cmd {
	ctx := m.ctx
	userID := m.user.UserID
	service := m.subjects.service
	metrics := m.metrics
	return func() tea.Msg {
		subjects, err := service.List(ctx, userID)
		result := subjectsListedMsg{subjects: subjects, err: err}
		if err == nil {
			result.counts, result.countErr = metrics.ThoughtCountsBySubject(ctx, userID)
		}
		return result
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

func (m Model) updateSubject(name string) tea.Cmd {
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
	rows := subjectRows(message.subjects)
	if message.countErr != nil {
		logSubjectError(m.logger, logging.ThoughtCountsBySubject, message.countErr)
		m.subjects.err = message.countErr
		m.subjects.errMessage = "Could not load thought counts. Press r to retry."
	} else {
		counts := make(map[int64]int64, len(message.counts))
		for _, count := range message.counts {
			counts[count.SubjectID] = count.Count
		}
		for i, item := range rows {
			row := item.(subjectRow)
			if count, ok := counts[row.subject.SubjectID]; ok {
				row.count = &count
				rows[i] = row
			}
		}
	}
	cmd := m.subjects.filter.SetItems(&m.subjects.list, rows)
	return m, cmd
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
	m.subjects.detailTitle = "Created subject"
	m.subjects.listStale = true
	m.logger.Mutation(logging.SubjectCreated, message.subject.SubjectID)
	m.screen = screenSubjectDetail
	cmd := m.thoughts.Open(message.subject.SubjectID)
	return m, cmd
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
	m.subjects.detailTitle = "Subject"
	m.screen = screenSubjectDetail
	cmd := m.thoughts.Open(message.subject.SubjectID)
	return m, cmd
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
	m.thoughts.Reset()
	m.subjects.listStale = true
	m.subjects.loading = true
	m.screen = screenSubjectList
	m.logger.Mutation(logging.SubjectDeleted, message.subjectID)
	return m, m.listSubjects()
}

func logSubjectError(logger logging.Logger, operation logging.Operation, err error) {
	if category, emit := failure.Classify(operation, err); emit {
		logger.Failure(operation, category)
	}
}

func (m Model) updateSubjectList(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "r":
			if !m.subjects.list.SettingFilter() && !m.subjects.loading {
				return m.openSubjects()
			}
		case "esc":
			if !m.subjects.list.SettingFilter() && !m.subjects.list.IsFiltered() {
				m.subjects.filter.Invalidate()
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

	command := m.subjects.filter.Update(&m.subjects.list, message)
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
	if !m.thoughts.Browsing() {
		var cmd tea.Cmd
		m.thoughts, cmd = m.thoughts.Update(message)
		return m, cmd
	}
	if key, ok := message.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "e":
			if m.subjects.selected == nil {
				return m, nil
			}
			m.screen = screenSubjectEdit
			m.subjects.err = nil
			m.subjects.input.SetValue(m.subjects.selected.SubjectName)
			m.subjects.input.CursorEnd()
			return m, m.subjects.input.Focus()
		case "d":
			if m.subjects.selected == nil {
				return m, nil
			}
			m.screen = screenSubjectDelete
			m.subjects.err = nil
			return m, nil
		case "q":
			return m, tea.Quit
		case "esc":
			m.thoughts.Reset()
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
	var cmd tea.Cmd
	m.thoughts, cmd = m.thoughts.Update(message)
	return m, cmd
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
			return m, m.updateSubject(m.subjects.input.Value())
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

func (m Model) viewSubjectList() string {
	status := ""
	if m.subjects.loading {
		status = "Loading subjects…\n\n"
	} else if m.subjects.err != nil {
		status = fmt.Sprintf("Error: %s\n\n", m.subjects.errMessage)
	}
	count := 0
	for _, item := range m.subjects.list.VisibleItems() {
		if row, ok := item.(subjectRow); ok && row.kind == subjectRowRecord {
			count++
		}
	}
	label := "subjects"
	if count == 1 {
		label = "subject"
	}
	return fmt.Sprintf("%s%s\n%d %s\nEsc: entities • q: quit", status, m.subjects.list.View(), count, label)
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
	title := m.subjects.detailTitle
	if title == "" {
		title = "Subject"
	}
	return fmt.Sprintf(
		"%s\n\nName: %s\nAdded: %s\n\n%s\n%s",
		title,
		m.subjects.selected.SubjectName,
		m.subjects.selected.CreatedAt.UTC().Format(subjectDateLayout),
		m.thoughts.View(), m.subjectDetailHelp(),
	)
}

func (m Model) subjectDetailHelp() string {
	if m.thoughts.Browsing() {
		return "e: edit subject • d: delete subject • Esc: subjects • q: quit"
	}
	return ""
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
