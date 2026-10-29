package goals

import (
	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
)

// CreateSubjectRequest delegates creation to the existing root form. The token
// restricts its return to the picker visit that requested it.
type CreateSubjectRequest struct {
	Query string
	owner *byte
}

type createSubjectRow struct{}

func (createSubjectRow) Title() string       { return "    Create subject…" }
func (createSubjectRow) Description() string { return "Add a new subject" }
func (createSubjectRow) FilterValue() string { return "Create subject" }

func (m *Model) requestSubjectCreation() tea.Cmd {
	if m.cancel != nil {
		m.cancel()
	}
	m.owner = nil
	m.pickerFilter.Invalidate()
	m.subjectCreation = new(byte)
	request := CreateSubjectRequest{Query: m.picker.FilterValue(), owner: m.subjectCreation}
	return func() tea.Msg { return request }
}
func (m Model) AwaitingSubject(request CreateSubjectRequest) bool {
	return m.open && m.screen == subjectsView && m.subjectCreation != nil && request.owner == m.subjectCreation
}
func (m *Model) ReturnFromSubjectCreation(request CreateSubjectRequest, subject *data.Subject) tea.Cmd {
	if !m.AwaitingSubject(request) {
		return nil
	}
	m.subjectCreation = nil
	if subject != nil {
		found := false
		for _, item := range m.available {
			if item.SubjectID == subject.SubjectID {
				found = true
				break
			}
		}
		if !found {
			m.available = append(m.available, *subject)
		}
		if m.selectedIDs == nil {
			m.selectedIDs = make(map[int64]bool)
		}
		m.selectedIDs[subject.SubjectID] = true
		m.picker.ResetFilter()
	}
	cmd := m.refreshPicker()
	if subject != nil {
		for i, item := range m.picker.Items() {
			if row, ok := item.(subjectRow); ok && row.item.SubjectID == subject.SubjectID {
				m.picker.Select(i)
				break
			}
		}
	}
	return cmd
}
