package events

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
)

type SubjectReader interface {
	List(context.Context, string) ([]data.Subject, error)
	Get(context.Context, string, int64) (*data.Subject, error)
}

// CreateSubjectRequest asks the root to open its existing creation screen.
// The private identity limits the return to the draft that requested it.
type CreateSubjectRequest struct {
	Query string
	owner *int
	save  uint64
}

type subjectsLoaded struct {
	owner          *int
	save, revision uint64
	items          []data.Subject
	err            error
}

func (subjectsLoaded) eventMessage() {}

type subjectPicker struct {
	cancel    context.CancelFunc
	items     []data.Subject
	selected  map[int64]bool
	before    map[int64]bool
	open      bool
	searching bool
	row, top  int
	loading   bool
	err       error
	revision  uint64
}

func (m *Model) loadSubjects() tea.Cmd {
	m.form.subjects.loading = true
	m.form.subjects.revision++
	owner, save, revision := m.owner, m.save, m.form.subjects.revision
	if m.form.subjects.cancel != nil {
		m.form.subjects.cancel()
	}
	ctx, cancel := context.WithCancel(m.form.ctx)
	m.form.subjects.cancel = cancel
	user, reader := m.userID, m.subjectReader
	return func() tea.Msg {
		if ctx.Err() != nil {
			return nil
		}
		items, err := reader.List(ctx, user)
		if ctx.Err() != nil {
			return nil
		}
		return subjectsLoaded{owner, save, revision, items, err}
	}
}

func (m *Model) acceptSubjects(result subjectsLoaded) {
	if result.owner != m.owner || result.save != m.save || !m.form.open || result.revision != m.form.subjects.revision || !m.form.subjects.loading {
		return
	}
	m.form.subjects.loading = false
	m.form.subjects.err = result.err
	m.fail(logging.SubjectList, result.err)
	if result.err == nil {
		m.form.subjects.items = result.items
	}
	m.form.subjects.row, m.form.subjects.top = 0, 0
}

func (m Model) matchingSubjects() []data.Subject {
	query := strings.ToLower(m.form.fields[1].Value())
	var items []data.Subject
	for _, item := range m.form.subjects.items {
		if strings.Contains(strings.ToLower(item.SubjectName), query) {
			items = append(items, item)
		}
	}
	return items
}

func (m Model) subjectRowsVisible() int {
	// Reserve labels, inputs, feedback and the help rows after wrapping.
	reserved := 13 + strings.Count(m.formHelp(), "\n") + strings.Count(ansi.Wrap(m.form.message, m.width, ""), "\n")
	return max(1, min(6, m.height-reserved))
}
func (m *Model) anchorSubjects() {
	p := &m.form.subjects
	p.row = max(0, min(p.row, len(m.matchingSubjects())))
	visible := m.subjectRowsVisible()
	p.top = max(0, min(p.top, p.row))
	if p.row >= p.top+visible {
		p.top = p.row - visible + 1
	}
}

func (m *Model) activateSubject() tea.Cmd {
	if m.form.subjects.row == 0 {
		m.form.suspended = true
		m.form.stopClipboard()
		m.form.revision++
		m.form.fields[1].Blur()
		request := CreateSubjectRequest{m.form.fields[1].Value(), m.owner, m.save}
		return func() tea.Msg { return request }
	}
	item := m.matchingSubjects()[m.form.subjects.row-1]
	m.selectSubject(item)
	return m.focusForm()
}
func (m *Model) selectSubject(item data.Subject) {
	if m.form.subjects.selected == nil {
		m.form.subjects.selected = make(map[int64]bool)
	}
	m.form.subjects.selected[item.SubjectID] = !m.form.subjects.selected[item.SubjectID]
	m.form.stopClipboard()
	m.form.revision++
	m.form.message = ""
}
func (m Model) selectedSubjectIDs() []int64 {
	ids := []int64{}
	for id, selected := range m.form.subjects.selected {
		if selected {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
func (m *Model) openSubjectPicker() {
	p := &m.form.subjects
	p.before = make(map[int64]bool, len(p.selected))
	for id, selected := range p.selected {
		p.before[id] = selected
	}
	p.open = true
	p.searching = false
}

func (m Model) AwaitingSubject(request CreateSubjectRequest) bool {
	return m.form.open && m.form.suspended && request.owner == m.owner && request.save == m.save
}

// ReturnFromSubjectCreation resumes the preserved draft. A nil subject cancels.
func (m *Model) ReturnFromSubjectCreation(request CreateSubjectRequest, subject *data.Subject) tea.Cmd {
	if !m.AwaitingSubject(request) {
		return nil
	}
	m.form.suspended = false
	m.form.index = 1
	if subject != nil {
		// A list started before creation cannot overwrite the new choice.
		if m.form.subjects.cancel != nil {
			m.form.subjects.cancel()
		}
		m.form.subjects.revision++
		m.form.subjects.loading = false
		m.form.subjects.err = nil
		m.form.subjects.items = append(m.form.subjects.items, *subject)
		m.selectSubject(*subject)
		m.form.subjects.selected[subject.SubjectID] = true
	}
	return m.focusForm()
}

func (m Model) subjectView() string {
	body := fmt.Sprintf("Subjects (%d selected)", len(m.selectedSubjectIDs()))
	if !m.form.subjects.open {
		return ansi.Truncate(body+" · Enter: choose", max(1, m.width), "…")
	}
	body += "\n/ " + m.form.fields[1].View()
	if m.form.index != 1 {
		return body
	}
	items := m.matchingSubjects()
	p := m.form.subjects
	end := min(len(items)+1, p.top+m.subjectRowsVisible())
	for row := p.top; row < end; row++ {
		label := "Create subject…"
		if row > 0 {
			mark := "[ ] "
			if p.selected[items[row-1].SubjectID] {
				mark = "[x] "
			}
			label = mark + singleLine(items[row-1].SubjectName)
		}
		prefix := "    "
		if row == p.row {
			prefix = "  > "
		}
		body += "\n" + ansi.Truncate(prefix+label, max(1, m.width), "…")
	}
	status := ""
	switch {
	case p.loading:
		status = "Loading subjects…"
	case p.err != nil:
		status = "Could not load subjects. You can still create one."
	case len(items) == 0:
		status = "No matching subjects."
	case p.top > 0 || end < len(items)+1:
		status = "↑/↓: more subjects"
	}
	if status != "" {
		body += "\n" + ansi.Truncate(status, max(1, m.width), "…")
	}
	return body
}
