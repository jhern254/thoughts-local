// Package goals owns Goal browsing and its independent editing workflows.
package goals

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/failure"
	"github.com/jhern254/go-thoughts/internal/goal"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/tui/listfilter"
)

type GoalService interface {
	List(context.Context, string) ([]data.Goal, error)
	ListActive(context.Context, string) ([]data.Goal, error)
	ListInactive(context.Context, string) ([]data.Goal, error)
	Get(context.Context, string, int64) (*data.Goal, error)
	Update(context.Context, string, int64, int64, goal.UpdateGoalInput) (*data.Goal, error)
}
type Assignments interface {
	Create(context.Context, string, goal.CreateGoalInput, []int64) (*data.Goal, error)
	ReplaceSubjects(context.Context, string, int64, int64, []int64) (*data.Goal, error)
}
type Memberships interface {
	ListLiveSubjectsForGoal(context.Context, string, int64) ([]data.Subject, error)
}
type Subjects interface {
	List(context.Context, string) ([]data.Subject, error)
}

type view uint8

const (
	listView view = iota
	detailView
	createView
	editView
	subjectsView
)

type BackMsg struct{}
type result struct {
	owner     *byte
	rows      []list.Item
	item      *data.Goal
	subjects  []data.Subject
	available []data.Subject
	err       error
}

func Owns(msg tea.Msg) bool { _, ok := msg.(result); return ok }

type createRow struct{}

func (createRow) Title() string       { return "Create Goal…" }
func (createRow) Description() string { return "Add a new Goal" }
func (createRow) FilterValue() string { return "" }

type goalRow struct {
	item     data.Goal
	subjects []data.Subject
}

func (r goalRow) Title() string       { return r.item.GoalName }
func (r goalRow) FilterValue() string { return r.item.GoalName }
func (r goalRow) Description() string {
	names := subjectNames(r.subjects)
	if len(names) > 3 {
		names = append(names[:3], "...")
	}
	return strings.Join(names, ", ") + "\n" + goalSummary(r.item)
}

type subjectRow struct {
	item     data.Subject
	selected bool
}

func (r subjectRow) Title() string {
	mark := "[ ] "
	if r.selected {
		mark = "[x] "
	}
	return mark + r.item.SubjectName
}
func (r subjectRow) Description() string { return "" }
func (r subjectRow) FilterValue() string { return r.item.SubjectName }

type Model struct {
	logger                logging.Logger
	operation             logging.Operation
	ctx                   context.Context
	userID                string
	goals                 GoalService
	assignments           Assignments
	memberships           Memberships
	subjects              Subjects
	screen                view
	list                  list.Model
	picker                list.Model
	filter, pickerFilter  listfilter.Scope
	body                  viewport.Model
	fields                [5]textinput.Model
	focus                 int
	cadence               int
	selected              *data.Goal
	live                  []data.Subject
	available             []data.Subject
	selectedIDs           map[int64]bool
	pickerBefore          map[int64]bool
	pickerReturn          view
	owner                 *byte
	cancel                context.CancelFunc
	loading, saving, open bool
	status                string
	width, height         int
	visibility            int
}

func New(ctx context.Context, userID string, service GoalService, assignments Assignments, memberships Memberships, subjects Subjects) Model {
	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(4)
	m := Model{
		logger:      logging.Nop(),
		ctx:         ctx,
		userID:      userID,
		goals:       service,
		assignments: assignments,
		memberships: memberships,
		subjects:    subjects,
		list:        list.New(nil, delegate, 80, 18),
		picker:      list.New(nil, list.NewDefaultDelegate(), 80, 18),
		body:        viewport.New(),
		width:       80,
		height:      24,
	}

	m.list.Title = "Goals"
	m.picker.Title = "Select Subjects"
	m.list.DisableQuitKeybindings()
	m.picker.DisableQuitKeybindings()
	m.list.SetShowStatusBar(false)
	m.picker.SetShowStatusBar(false)
	for i := range m.fields {
		m.fields[i] = textinput.New()
		m.fields[i].CharLimit = 0
	}
	return m
}
func (m *Model) Close() {
	if m.cancel != nil {
		m.cancel()
	}
	m.owner = nil
	m.open = false
	m.filter.Invalidate()
	m.pickerFilter.Invalidate()
}
func (m *Model) beginRequest(operation logging.Operation) context.Context {
	m.operation = operation
	if m.cancel != nil {
		m.cancel()
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	m.owner = new(byte)
	m.loading = true
	m.status = "Loading…"
	return ctx
}
func (m *Model) Resize(width, height int) {
	m.width = max(1, width)
	m.height = max(1, height)
	m.list.SetSize(m.width, max(1, height-4))
	m.picker.SetSize(m.width, max(1, height-4))
	m.body.SetWidth(m.width)
	m.body.SetHeight(max(1, height-4))
	for i := range m.fields {
		m.fields[i].SetWidth(max(1, width-4))
	}
}
func (m *Model) Open() tea.Cmd { m.open = true; m.screen = listView; return m.loadList() }
func (m *Model) loadList() tea.Cmd {
	ctx := m.beginRequest(logging.GoalList)
	owner, user, service, memberships, visibility := m.owner, m.userID, m.goals, m.memberships, m.visibility
	return func() tea.Msg {
		var items []data.Goal
		var err error
		switch visibility {
		case 1:
			items, err = service.ListActive(ctx, user)
		case 2:
			items, err = service.ListInactive(ctx, user)
		default:
			items, err = service.List(ctx, user)
		}
		rows := make([]list.Item, 0, len(items)+1)
		rows = append(rows, createRow{})
		for _, item := range items {
			if err != nil {
				break
			}
			var subjects []data.Subject
			subjects, err = memberships.ListLiveSubjectsForGoal(ctx, user, item.GoalID)
			rows = append(rows, goalRow{item, subjects})
		}
		return result{owner: owner, rows: rows, err: err}
	}
}
func (m *Model) loadDetail(id int64) tea.Cmd {
	ctx := m.beginRequest(logging.GoalGet)
	owner, user, service, memberships := m.owner, m.userID, m.goals, m.memberships
	return func() tea.Msg {
		item, err := service.Get(ctx, user, id)
		var subjects []data.Subject
		if err == nil {
			subjects, err = memberships.ListLiveSubjectsForGoal(ctx, user, id)
		}
		return result{owner: owner, item: item, subjects: subjects, err: err}
	}
}
func (m *Model) loadPicker() tea.Cmd {
	ctx := m.beginRequest(logging.GoalSubjectsList)
	owner, user, subjects, service, memberships := m.owner, m.userID, m.subjects, m.goals, m.memberships
	var id int64
	if m.selected != nil {
		id = m.selected.GoalID
	}
	manage := m.pickerReturn == detailView
	return func() tea.Msg {
		available, err := subjects.List(ctx, user)
		var item *data.Goal
		var live []data.Subject
		if err == nil && manage {
			item, err = service.Get(ctx, user, id)
			if err == nil {
				live, err = memberships.ListLiveSubjectsForGoal(ctx, user, id)
			}
		}
		return result{owner: owner, item: item, subjects: live, available: available, err: err}
	}
}
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.update(msg)
	if next.screen == createView || next.screen == editView {
		next.body.SetContent(ansi.Hardwrap(next.formContent(), next.width, true))
		row := 2 + next.focus*2
		if next.focus >= 5 {
			row = 12 + next.focus - 5
		}
		if row < next.body.YOffset() {
			next.body.SetYOffset(row)
		}
		if row >= next.body.YOffset()+next.body.Height() {
			next.body.SetYOffset(row - next.body.Height() + 1)
		}
	} else if next.screen == detailView && next.selected != nil {
		next.body.SetContent(ansi.Hardwrap(next.detailContent(), next.width, true))
	}
	return next, cmd
}

func (m Model) update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.open {
		return m, nil
	}
	if reply, ok := msg.(result); ok {
		if reply.owner != m.owner {
			return m, nil
		}
		wasSaving := m.saving
		m.loading = false
		m.saving = false
		if reply.err != nil {
			if category, emit := failure.Classify(m.operation, reply.err); emit {
				m.logger.Failure(m.operation, category)
			}
			m.status = errorMessage(reply.err)
			return m, nil
		}
		m.status = ""
		if wasSaving {
			event := logging.GoalUpdated
			if m.operation == logging.GoalCreate {
				event = logging.GoalCreated
			}
			if m.operation == logging.GoalSubjectsReplace {
				event = logging.GoalSubjectsUpdated
			}
			m.logger.Mutation(event, reply.item.GoalID)

			m.screen = detailView
			m.selected = reply.item
			return m, m.loadDetail(reply.item.GoalID)
		}
		switch m.screen {
		case listView:
			var selectedID int64
			if row, ok := m.list.SelectedItem().(goalRow); ok {
				selectedID = row.item.GoalID
			}
			if m.selected != nil {
				selectedID = m.selected.GoalID
			}
			cmd := m.filter.SetItems(&m.list, reply.rows)
			for i, row := range reply.rows {
				if record, ok := row.(goalRow); ok && record.item.GoalID == selectedID {
					m.list.Select(i)
					break
				}
			}
			return m, cmd
		case subjectsView:
			m.available = reply.available
			if m.pickerReturn == detailView {
				m.selected = reply.item
				m.selectedIDs = make(map[int64]bool)
				for _, s := range reply.subjects {
					m.selectedIDs[s.SubjectID] = true
				}
			}
			return m, m.refreshPicker()
		default:
			m.selected = reply.item
			m.live = reply.subjects
			if m.screen == editView {
				m.populateForm()
			}
		}
		return m, nil
	}
	if reply, ok := msg.(listfilter.Reply); ok {
		if m.screen == listView {
			return m, m.filter.Update(&m.list, reply)
		}
		if m.screen == subjectsView {
			return m, m.pickerFilter.Update(&m.picker, reply)
		}
		return m, nil
	}
	key, ok := msg.(tea.KeyPressMsg)
	if m.saving {
		return m, nil
	}
	if ok && key.String() == "esc" {
		if m.screen == listView && m.list.FilterState() != list.Unfiltered {
			m.list.ResetFilter()
			m.filter.Invalidate()
			return m, nil
		}
		if m.screen == subjectsView && m.picker.FilterState() != list.Unfiltered {
			m.picker.ResetFilter()
			m.pickerFilter.Invalidate()
			return m, nil
		}
		if m.cancel != nil {
			m.cancel()
		}
		m.owner = nil
		m.loading = false
		m.status = ""
		switch m.screen {
		case listView:
			m.Close()
			return m, func() tea.Msg { return BackMsg{} }
		case detailView:
			m.screen = listView
			return m, m.loadList()
		case createView:
			m.screen = listView
			return m, m.loadList()
		case editView:
			m.screen = detailView
			return m, m.loadDetail(m.selected.GoalID)
		case subjectsView:
			m.selectedIDs = m.pickerBefore
			m.screen = m.pickerReturn
			if m.screen == detailView {
				return m, m.loadDetail(m.selected.GoalID)
			}
			return m, nil
		}
	}
	if m.loading {
		return m, nil
	}
	if ok && (key.String() == "r" || key.String() == "R") && m.status != "" {
		switch m.screen {
		case listView:
			return m, m.loadList()
		case detailView, editView:
			return m, m.loadDetail(m.selected.GoalID)
		case subjectsView:
			return m, m.loadPicker()
		}
	}
	switch m.screen {
	case listView:
		if ok && !m.list.SettingFilter() {
			switch key.String() {
			case "n":
				m.startForm(true)
				return m, m.fields[0].Focus()
			case "f":
				m.visibility = (m.visibility + 1) % 3
				m.selected = nil
				m.list.ResetFilter()
				m.list.Select(0)
				return m, m.loadList()
			case "enter":
				if _, ok := m.list.SelectedItem().(createRow); ok {
					m.startForm(true)
					return m, m.fields[0].Focus()
				}
				if row, ok := m.list.SelectedItem().(goalRow); ok {
					m.screen = detailView
					return m, m.loadDetail(row.item.GoalID)
				}
			}
		}
		return m, m.filter.Update(&m.list, msg)
	case detailView:
		if ok && m.selected != nil {
			switch key.String() {
			case "e":
				m.startForm(false)
				return m, m.fields[0].Focus()
			case "s":
				m.pickerReturn = detailView
				m.pickerBefore = cloneSelection(m.selectedIDs)
				m.screen = subjectsView
				return m, m.loadPicker()
			}
		}
		var cmd tea.Cmd
		m.body, cmd = m.body.Update(msg)
		return m, cmd
	case createView, editView:
		return m, m.updateForm(msg)
	case subjectsView:
		if ok && !m.picker.SettingFilter() {
			switch key.String() {
			case "space":
				if row, ok := m.picker.SelectedItem().(subjectRow); ok {
					m.selectedIDs[row.item.SubjectID] = !m.selectedIDs[row.item.SubjectID]
					return m, m.refreshPicker()
				}
			case "enter":
				if m.pickerReturn == createView {
					m.screen = createView
					return m, nil
				}
				return m, m.saveSubjects()
			}
		}
		return m, m.pickerFilter.Update(&m.picker, msg)
	}
	return m, nil
}
func (m *Model) refreshPicker() tea.Cmd {
	rows := make([]list.Item, 0, len(m.available))
	for _, s := range m.available {
		rows = append(rows, subjectRow{s, m.selectedIDs[s.SubjectID]})
	}
	return m.pickerFilter.SetItems(&m.picker, rows)
}
func cloneSelection(ids map[int64]bool) map[int64]bool {
	out := make(map[int64]bool, len(ids))
	for id, value := range ids {
		out[id] = value
	}
	return out
}
func subjectNames(subjects []data.Subject) []string {
	if len(subjects) == 0 {
		return []string{"Misc"}
	}
	names := make([]string, len(subjects))
	for i, s := range subjects {
		names[i] = s.SubjectName
	}
	return names
}
func goalSummary(item data.Goal) string {
	state := "Active"
	if !item.IsActive {
		state = "Inactive"
	}
	summary := fmt.Sprintf("%s / %s · %s · %s", formatDuration(item.TargetSeconds), cadenceLabel(item.Cadence), item.Priority, state)
	if item.StartDate != nil {
		summary += "\nFrom " + *item.StartDate
	}
	if item.EndDate != nil {
		summary += "\nUntil " + *item.EndDate
	}
	return summary
}
func (m Model) View() string {
	var content, help string
	switch m.screen {
	case listView:
		content = m.list.View()
		help = "n: new · Enter: details · f: " + []string{"All", "Active", "Inactive"}[m.visibility] + " · Esc: back"
	case subjectsView:
		content = m.picker.View()
		help = "Space: select · Enter: confirm · /: search · Esc: cancel"
	case detailView:
		content = m.detailContent()
		help = "e: Edit Goal · s: Manage Subjects · Esc: Back"
	default:
		content = m.formContent()
		help = "Tab: next · Ctrl+S: save · Esc: cancel"
	}
	if m.screen != listView && m.screen != subjectsView {
		m.body.SetContent(ansi.Hardwrap(content, m.width, true))
		content = m.body.View()
	}
	status := m.status
	if status == "" && ((m.screen == subjectsView && m.picker.FilterValue() != "" && len(m.picker.VisibleItems()) == 0) || (m.screen == listView && m.list.FilterValue() != "" && len(m.list.VisibleItems()) == 0)) {
		status = "No matches."
	}
	return content + "\n" + ansi.Truncate(status, m.width, "…") + "\n" + ansi.Truncate(help, m.width, "…")
}

func (m Model) detailContent() string {
	if m.selected == nil {
		return ""
	}
	item := m.selected
	return item.GoalName + "\n\n" + goalSummary(*item) + "\n\nSubjects\n" + strings.Join(subjectNames(m.live), "\n") + fmt.Sprintf("\n\nGoal %d\nCreated %s\nUpdated %s", item.GoalID, item.CreatedAt.Format("Jan 2, 2006"), item.UpdatedAt.Format("Jan 2, 2006"))
}

func (m Model) IsOpen() bool { return m.open }

func (m *Model) SetLogger(logger logging.Logger) { m.logger = logger }
