package goals

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/failure"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/tui/appearance"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
)

// ActivityReader supplies earned history; attribution and Subject matching stay
// behind the Goal progress application boundary, outside the presentation layer.
type ActivityReader interface {
	ListEarnedEvents(context.Context, string, int64, data.GoalEventPageRequest) (data.GoalEventPage, error)
	ListEventLineageThoughts(context.Context, string, int64, int64, data.ThoughtSummaryViewRequest) (data.GoalEventThoughtPage, error)
	GetEventLineageThought(context.Context, string, int64, int64, int64) (*data.Thought, error)
}

// Details puts one blank row, the history heading, and its status row before
// the list. Mouse coordinates and height allocation must use the same offset.
const activityHeaderRows = 3

type activityState struct {
	list                         list.Model
	focused                      bool
	events                       data.GoalEventPage
	eventRequest                 data.GoalEventPageRequest
	eventsOlder, eventsNewer     bool
	expandedEventID              int64
	thoughts                     data.ThoughtSummaryViewResult
	thoughtRequest               data.ThoughtSummaryViewRequest
	thoughtsOlder, thoughtsNewer bool
	// One history read owns this token and context. Collapsing, paging, changing
	// Goals, or leaving Details invalidates replies from the previous read.
	owner        *byte
	cancel       context.CancelFunc
	loading      bool
	errorMessage string
}

type activityResult struct {
	owner       *byte
	eventID     int64
	events      data.GoalEventPage
	thoughtPage data.GoalEventThoughtPage
	err         error
}

type activityRowKind uint8

const (
	activityEvent activityRowKind = iota
	activityThought
	activityEventsOlder
	activityEventsNewer
	activityThoughtsOlder
	activityThoughtsNewer
)

type activityRow struct {
	kind     activityRowKind
	event    data.GoalEventLineage
	thought  data.ThoughtSummaryView
	expanded bool
}

func (r activityRow) FilterValue() string { return r.Title() }
func (r activityRow) Title() string {
	switch r.kind {
	case activityEvent:
		mark := "▸ "
		if r.expanded {
			mark = "▾ "
		}
		activity := strings.Join(strings.Fields(r.event.Activity), " ")
		if activity == "" {
			activity = "Event"
		}
		return mark + r.event.StartedAt.Local().Format("Jan 2 · 15:04") + " · " + activity
	case activityThought:
		badge := ""
		if r.thought.IsMindset {
			badge = "M · "
		}
		return "  " + badge + strings.Join(strings.Fields(r.thought.Preview), " ")
	case activityEventsOlder:
		return "Older Events…"
	case activityEventsNewer:
		return "Newer Events…"
	case activityThoughtsOlder:
		return "  Older Thoughts…"
	default:
		return "  Newer Thoughts…"
	}
}
func (r activityRow) Description() string {
	if r.kind == activityEvent {
		return "  " + formatDuration(r.event.ElapsedSeconds)
	}
	if r.kind == activityThought {
		subject := ""
		if r.thought.SubjectName != nil {
			subject = *r.thought.SubjectName
		}
		return "  " + subject + " · " + r.thought.ObservedAt.Local().Format("15:04:05")
	}
	return ""
}
func (m *Model) resetActivity() {
	m.cancelActivityRead()
	delegate := activityDelegate(true)
	m.activity = activityState{list: list.New(nil, delegate, m.width, 6), focused: true}
	m.activity.list.DisableQuitKeybindings()
	m.activity.list.SetFilteringEnabled(false)
	m.activity.list.SetShowTitle(false)
	m.activity.list.SetShowStatusBar(false)
	m.activity.list.SetShowHelp(false)
	m.activity.list.SetShowPagination(true)
	m.resizeDetail()
}
func (m *Model) cancelActivityRead() {
	if m.activity.cancel != nil {
		m.activity.cancel()
	}
	m.activity.cancel = nil
	m.activity.owner = nil
	m.activity.loading = false
}
func (m *Model) beginActivityRead() context.Context {
	m.cancelActivityRead()
	ctx, cancel := context.WithCancel(m.ctx)
	m.activity.cancel = cancel
	m.activity.owner = new(byte)
	m.activity.loading = true
	m.activity.errorMessage = ""
	return ctx
}
func (m *Model) loadActivityEvents(request data.GoalEventPageRequest) tea.Cmd {
	ctx := m.beginActivityRead()
	m.activity.eventRequest = request
	m.activity.expandedEventID = 0
	m.activity.thoughts = data.ThoughtSummaryViewResult{}
	m.refreshActivityRows()
	owner, reader, user, id := m.activity.owner, m.activityReader, m.userID, m.requestedGoalID
	return func() tea.Msg {
		page, err := reader.ListEarnedEvents(ctx, user, id, request)
		return activityResult{owner: owner, events: page, err: err}
	}
}
func (m *Model) loadActivityThoughts(eventID int64, request data.ThoughtSummaryViewRequest) tea.Cmd {
	ctx := m.beginActivityRead()
	if m.activity.expandedEventID != eventID {
		m.activity.thoughts = data.ThoughtSummaryViewResult{}
		m.activity.thoughtsOlder, m.activity.thoughtsNewer = false, false
	}
	m.activity.expandedEventID = eventID
	m.activity.thoughtRequest = request
	m.refreshActivityRows()
	owner, reader, user, id := m.activity.owner, m.activityReader, m.userID, m.requestedGoalID
	return func() tea.Msg {
		page, err := reader.ListEventLineageThoughts(ctx, user, id, eventID, request)
		return activityResult{owner: owner, eventID: eventID, thoughtPage: page, err: err}
	}
}
func (m *Model) receiveActivity(reply activityResult) tea.Cmd {
	if m.screen != detailView || reply.owner == nil || reply.owner != m.activity.owner {
		return nil
	}
	m.activity.loading = false
	if reply.err != nil {
		operation := logging.GoalActivityList
		if reply.eventID != 0 {
			operation = logging.GoalActivityThoughtsList
		}
		if category, emit := failure.Classify(operation, reply.err); emit {
			m.logger.Failure(operation, category)
		}
		m.activity.errorMessage = "Could not load activity. R: retry."
		return nil
	}
	if reply.eventID == 0 {
		m.activity.events = reply.events
		request := m.activity.eventRequest
		m.activity.eventsOlder = reply.events.More && request.Direction == data.EarnedEventsOlder || request.Cursor != nil && request.Direction == data.EarnedEventsNewer
		m.activity.eventsNewer = reply.events.More && request.Direction == data.EarnedEventsNewer || request.Cursor != nil && request.Direction == data.EarnedEventsOlder
		m.activity.list.Select(0)
	} else {
		for i := range m.activity.events.Events {
			if m.activity.events.Events[i].EventID == reply.eventID {
				m.activity.events.Events[i] = reply.thoughtPage.Event
			}
		}
		m.activity.thoughts = reply.thoughtPage.Thoughts
		request := m.activity.thoughtRequest
		m.activity.thoughtsOlder = reply.thoughtPage.Thoughts.More && request.Direction == data.ThoughtsOlder || request.Cursor != nil && request.Direction == data.ThoughtsNewer
		m.activity.thoughtsNewer = reply.thoughtPage.Thoughts.More && request.Direction == data.ThoughtsNewer || request.Cursor != nil && request.Direction == data.ThoughtsOlder
	}
	return m.refreshActivityRows()
}
func (m *Model) refreshActivityRows() tea.Cmd {
	selected, _ := m.activity.list.SelectedItem().(activityRow)
	rows := make([]list.Item, 0, len(m.activity.events.Events)+len(m.activity.thoughts.Items)+4)
	if m.activity.eventsNewer {
		rows = append(rows, activityRow{kind: activityEventsNewer})
	}
	for _, item := range m.activity.events.Events {
		expanded := item.EventID == m.activity.expandedEventID
		rows = append(rows, activityRow{kind: activityEvent, event: item, expanded: expanded})
		if expanded {
			if m.activity.thoughtsNewer {
				rows = append(rows, activityRow{kind: activityThoughtsNewer})
			}
			for _, thought := range m.activity.thoughts.Items {
				rows = append(rows, activityRow{kind: activityThought, event: item, thought: thought})
			}
			if m.activity.thoughtsOlder {
				rows = append(rows, activityRow{kind: activityThoughtsOlder})
			}
		}
	}
	if m.activity.eventsOlder {
		rows = append(rows, activityRow{kind: activityEventsOlder})
	}
	cmd := m.activity.list.SetItems(rows)
	for i, row := range rows {
		r := row.(activityRow)
		if r.kind == selected.kind && r.event.EventID == selected.event.EventID && r.thought.ThoughtID == selected.thought.ThoughtID {
			m.activity.list.Select(i)
			break
		}
	}
	return cmd
}
func (m *Model) collapseActivity() tea.Cmd {
	eventID := m.activity.expandedEventID
	m.cancelActivityRead()
	m.activity.expandedEventID = 0
	m.activity.errorMessage = ""
	m.activity.thoughts = data.ThoughtSummaryViewResult{}
	cmd := m.refreshActivityRows()
	for i, row := range m.activity.list.Items() {
		if r := row.(activityRow); r.kind == activityEvent && r.event.EventID == eventID {
			m.activity.list.Select(i)
			break
		}
	}
	return cmd
}

// OpenLineageThoughtMsg asks the root to use the existing Thought Details screen.
// The loader keeps both its first read and reload inside this Goal's lineage.
type OpenLineageThoughtMsg struct {
	owner           *byte
	GoalID, EventID int64
	Thought         data.ThoughtSummaryView
	Load            func(context.Context) (*data.Thought, error)
}

func (m Model) AcceptsLineageThought(msg OpenLineageThoughtMsg) bool {
	if !m.open || m.screen != detailView || m.selected == nil || msg.owner == nil {
		return false
	}
	return msg.owner == m.activity.owner && msg.GoalID == m.selected.GoalID && msg.EventID == m.activity.expandedEventID
}
func (m Model) openLineageThought(row activityRow) tea.Cmd {
	reader, user, goalID, eventID, thoughtID := m.activityReader, m.userID, m.requestedGoalID, row.event.EventID, row.thought.ThoughtID
	message := OpenLineageThoughtMsg{
		owner:   m.activity.owner,
		GoalID:  goalID,
		EventID: eventID,
		Thought: row.thought,
		Load: func(ctx context.Context) (*data.Thought, error) {
			return reader.GetEventLineageThought(ctx, user, goalID, eventID, thoughtID)
		},
	}
	return func() tea.Msg { return message }
}
func (m *Model) updateActivity(msg tea.Msg) tea.Cmd {
	if wheel, ok := msg.(tea.MouseWheelMsg); ok {
		var cmd tea.Cmd
		if wheel.Y < m.body.Height() {
			m.activity.focused = false
			m.body, cmd = m.body.Update(msg)
		} else {
			m.activity.focused = true
			m.activity.list, cmd = m.activity.list.Update(appearance.ListScrollInput(msg))
		}
		return cmd
	}
	if click, ok := msg.(tea.MouseClickMsg); ok && click.Button == tea.MouseLeft {
		if click.Y < m.body.Height() {
			m.activity.focused = false
			return nil
		}
		m.activity.focused = true
		delegate := activityDelegate(true)
		row := (click.Y - m.body.Height() - activityHeaderRows) / (delegate.Height() + delegate.Spacing())
		start, end := m.activity.list.Paginator.GetSliceBounds(len(m.activity.list.Items()))
		if click.Y >= m.body.Height()+activityHeaderRows && start+row < end {
			m.activity.list.Select(start + row)
			return m.updateActivity(tea.KeyPressMsg{Code: tea.KeyEnter})
		}
		return nil
	}
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "tab", "shift+tab":
			m.activity.focused = !m.activity.focused
			return nil
		case "r", "R":
			if m.activity.expandedEventID != 0 {
				return m.loadActivityThoughts(m.activity.expandedEventID, m.activity.thoughtRequest)
			}
			if m.activity.errorMessage != "" {
				return m.loadActivityEvents(m.activity.eventRequest)
			}
			return m.loadActivityEvents(data.GoalEventPageRequest{})
		case "enter":
			if !m.activity.focused {
				return nil
			}
			row, ok := m.activity.list.SelectedItem().(activityRow)
			if !ok {
				return nil
			}
			switch row.kind {
			case activityEvent:
				if m.activity.expandedEventID == row.event.EventID {
					return m.collapseActivity()
				}
				m.activity.thoughts = data.ThoughtSummaryViewResult{}
				return m.loadActivityThoughts(row.event.EventID, data.ThoughtSummaryViewRequest{})
			case activityThought:
				if !m.activity.loading && m.activity.errorMessage == "" {
					return m.openLineageThought(row)
				}
			case activityEventsOlder, activityEventsNewer:
				if len(m.activity.events.Events) == 0 {
					// Concurrent deletion can empty a page. Restart history
					// instead of trapping navigation without a cursor row.
					return m.loadActivityEvents(data.GoalEventPageRequest{})
				}
				direction, index := data.EarnedEventsOlder, len(m.activity.events.Events)-1
				if row.kind == activityEventsNewer {
					direction, index = data.EarnedEventsNewer, 0
				}
				cursor := m.activity.events.Events[index].Cursor()
				return m.loadActivityEvents(data.GoalEventPageRequest{Cursor: &cursor, Direction: direction})
			case activityThoughtsOlder, activityThoughtsNewer:
				if len(m.activity.thoughts.Items) == 0 {
					return m.loadActivityThoughts(m.activity.expandedEventID, data.ThoughtSummaryViewRequest{})
				}
				direction, index := data.ThoughtsOlder, len(m.activity.thoughts.Items)-1
				if row.kind == activityThoughtsNewer {
					direction, index = data.ThoughtsNewer, 0
				}
				cursor := m.activity.thoughts.Items[index].Cursor()
				return m.loadActivityThoughts(m.activity.expandedEventID, data.ThoughtSummaryViewRequest{Cursor: &cursor, Direction: direction})
			}
		}
	}
	var cmd tea.Cmd
	if m.activity.focused {
		m.activity.list, cmd = m.activity.list.Update(appearance.ListScrollInput(msg))
	} else {
		m.body, cmd = m.body.Update(msg)
	}
	return cmd
}
func (m *Model) resizeDetail() {
	if m.screen != detailView {
		return
	}
	available := max(3, m.height-3) // Footer status/help plus a spare terminal row.
	metadataHeight := max(1, available/3)
	m.body.SetHeight(metadataHeight)
	m.activity.list.SetSize(m.width, max(1, available-metadataHeight-activityHeaderRows-1))
}
func (m Model) activityView() string {
	if m.selected == nil {
		return m.body.View()
	}
	activityList := m.activity.list
	activityList.SetDelegate(activityDelegate(m.activity.focused))
	body := activityList.View()
	status := ""
	if m.activity.loading {
		status = "Loading activity…"
	} else if m.activity.errorMessage != "" {
		status = m.activity.errorMessage
	} else if len(m.activity.events.Events) == 0 {
		status = "No earned Events yet."
	} else if m.activity.expandedEventID != 0 && len(m.activity.thoughts.Items) == 0 {
		status = "No matching Thoughts in this Event."
	}
	body = ansi.Truncate(status, m.width, "…") + "\n" + body
	return m.body.View() + "\n\nACTIVITY HISTORY\n" + body
}
func (m *Model) applyActivityMindset(change thoughts.MindsetChanged) tea.Cmd {
	for i := range m.activity.thoughts.Items {
		if m.activity.thoughts.Items[i].ThoughtID == change.ThoughtID {
			m.activity.thoughts.Items[i].IsMindset = change.Active
		}
	}
	return m.refreshActivityRows()
}

func activityDelegate(focused bool) list.DefaultDelegate {
	delegate := goalListDelegate(focused)
	delegate.SetHeight(2)
	delegate.SetSpacing(1)
	return delegate
}
