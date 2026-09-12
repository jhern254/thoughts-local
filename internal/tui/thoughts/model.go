// Package thoughts owns the list, editor, and detail for subject and unassigned thoughts.
package thoughts

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/diagnostics"
	"github.com/jhern254/go-thoughts/internal/failure"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
	"github.com/jhern254/go-thoughts/internal/tui/listfilter"
)

type Service interface {
	BrowseView(context.Context, string, data.ThoughtViewRequest) (data.ThoughtView, error)
	ListUnassigned(context.Context, string) ([]data.Thought, error)
	List(context.Context, string, int64) ([]data.Thought, error)
	Get(context.Context, string, int64) (*data.Thought, error)
	Create(context.Context, string, string, *int64, time.Time) (*data.Thought, error)
}

// ChangedMsg invalidates picker counts after creation; nil SubjectID means unassigned.
type ChangedMsg struct{ SubjectID *int64 }

type screen uint8

const (
	browse screen = iota
	create
	detail
)

type Model struct {
	owner      *int
	ctx        context.Context
	userID     string
	service    Service
	logger     logging.Logger
	subjectID  *int64
	request    uint64
	screen     screen
	list       list.Model
	input      textarea.Model
	viewport   viewport.Model
	selected   *data.Thought
	loading    bool
	stale      bool
	err        error
	errMessage string

	inputWarning         string
	filter               listfilter.Scope
	browsingThoughtsView bool
	browseThoughts       browseThoughtsState
	countRequest         uint64
	itemStyles           list.DefaultItemStyles
	blurred              bool
}

// SetFocused changes selection styling without changing the selected row.
func (m *Model) SetFocused(focused bool) { m.blurred = !focused }

type rowKind uint8

const (
	rowCreate rowKind = iota
	rowRecord
)

type row struct {
	kind       rowKind
	item       data.Thought
	unassigned bool
}

func (r row) Title() string {
	if r.kind == rowCreate {
		return "Create thought…"
	}
	// Build a bounded, single-line preview without copying a potentially large body.
	preview := make([]rune, 0, 81)
	for _, c := range r.item.Thought {
		if len(preview) == 80 {
			return string(preview) + "…"
		}
		if unicode.IsSpace(c) {
			c = ' '
		}
		preview = append(preview, c)
	}
	return string(preview)
}
func (r row) Description() string {
	if r.kind == rowCreate {
		if r.unassigned {
			return "Write a new thought"
		}
		return "Add a thought to this subject"
	}
	return displaytime.Format(r.item.ObservedAt, "Jan 2, 2006 3:04 PM MST")
}
func (r row) FilterValue() string { return r.Title() }

// Result carries one asynchronous service result with its request context.
// Only the component constructs and interprets these messages.
type Result struct {
	owner     *int
	request   uint64
	operation logging.Operation
	items     []data.Thought
	item      *data.Thought
	err       error
}

func New(ctx context.Context, userID string, service Service, logger logging.Logger) Model {
	delegate := list.NewDefaultDelegate()
	items := list.New([]list.Item{row{kind: rowCreate}}, delegate, 80, 14)
	items.Title = "Thoughts"
	items.SetShowStatusBar(false)
	items.DisableQuitKeybindings()
	input := textarea.New()
	input.Placeholder = "What is on your mind?"
	input.CharLimit = 0
	input.MaxHeight = 0
	input.MaxWidth = 0
	view := viewport.New()
	view.SoftWrap = true
	m := Model{owner: new(int), ctx: ctx, userID: userID, service: service, logger: logger, list: items, input: input, viewport: view, itemStyles: delegate.Styles}
	m.Resize(80, 14)
	return m
}

func (m *Model) Resize(width, height int) {
	m.browseThoughts.width, m.browseThoughts.height = max(1, width), max(1, height-5)
	m.browseThoughts.keepVisible()
	m.list.SetSize(max(1, width), max(1, height-4))
	m.input.SetWidth(max(1, width-2))
	m.input.SetHeight(max(1, height-5))
	m.viewport.SetWidth(max(1, width))
	m.viewport.SetHeight(max(1, height-5))
}

func (m *Model) Reset() {
	m.countRequest++
	m.browsingThoughtsView = false
	m.browseThoughts = browseThoughtsState{width: m.browseThoughts.width, height: m.browseThoughts.height}
	m.filter.Invalidate()
	m.request++
	m.subjectID = nil
	m.selected = nil
	m.screen = browse
	m.loading = false
	m.stale = false
	m.err = nil
	m.input.Blur()
	m.input.Reset()
	m.inputWarning = ""
	m.list.ResetFilter()
	m.list.SetItems([]list.Item{row{kind: rowCreate}})
	m.list.Select(0)
}

func (m *Model) Open(subjectID int64) tea.Cmd {
	m.Reset()
	m.subjectID = &subjectID
	return m.listThoughts()
}

func (m *Model) OpenUnassigned() tea.Cmd {
	m.Reset()
	m.list.SetItems([]list.Item{row{kind: rowCreate, unassigned: true}})
	return m.listThoughts()
}

// Browsing allows the parent to handle subject shortcuts, but not editor input
// or keys used to set/clear a thought-list filter.
func (m Model) Browsing() bool {
	return m.screen == browse && !m.list.SettingFilter() && !m.list.IsFiltered()
}

// ShowingDetail lets the parent render the shared detail without list context.
func (m Model) ShowingDetail() bool { return m.screen == detail }

func (m *Model) listThoughts() tea.Cmd {
	m.request++
	m.loading = true
	m.err = nil
	owner := m.owner
	request, ctx, userID, subjectID, service := m.request, m.ctx, m.userID, m.subjectID, m.service
	return func() tea.Msg {
		var items []data.Thought
		var err error
		if subjectID == nil {
			items, err = service.ListUnassigned(ctx, userID)
		} else {
			items, err = service.List(ctx, userID, *subjectID)
		}
		return Result{owner: owner, request: request, operation: logging.ThoughtList, items: items, err: err}
	}
}

func (m *Model) getThought(id int64) tea.Cmd {
	m.request++
	m.loading = true
	m.err = nil
	owner := m.owner
	request, ctx, userID, service := m.request, m.ctx, m.userID, m.service
	return func() tea.Msg {
		item, err := service.Get(ctx, userID, id)
		return Result{owner: owner, request: request, operation: logging.ThoughtGet, item: item, err: err}
	}
}

func (m *Model) createThought(body string) tea.Cmd {
	m.request++
	m.loading = true
	m.err = nil
	owner := m.owner
	request, ctx, userID, subjectID, service := m.request, m.ctx, m.userID, m.subjectID, m.service
	return func() tea.Msg {
		item, err := service.Create(ctx, userID, body, subjectID, time.Time{})
		return Result{owner: owner, request: request, operation: logging.ThoughtCreate, item: item, err: err}
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if result, ok := msg.(ThoughtCountResult); ok {
		return m.receiveThoughtCount(result)
	}
	if result, ok := msg.(BrowseThoughtsResult); ok {
		return m.receiveBrowseThoughts(result)
	}
	switch msg.(type) {
	case listfilter.Reply, list.FilterMatchesMsg:
		cmd := m.filter.Update(&m.list, msg)
		return m, cmd
	}
	if result, ok := msg.(Result); ok {
		if result.owner != m.owner || result.request != m.request {
			return m, nil
		}
		m.loading = false
		m.err = result.err
		if result.err != nil {
			if category, emit := failure.Classify(result.operation, result.err); emit {
				m.logger.Failure(result.operation, category)
			}
			m.errMessage = diagnostics.ThoughtMessage(result.err, "Could not load thoughts.")
			if result.operation == logging.ThoughtCreate {
				m.errMessage = diagnostics.ThoughtMessage(result.err, "Could not save the thought.")
				return m, m.input.Focus()
			}
			return m, nil
		}
		if result.operation == logging.ThoughtList {
			rows := make([]list.Item, 0, len(result.items)+1)
			rows = append(rows, row{kind: rowCreate, unassigned: m.subjectID == nil})
			for _, item := range result.items {
				rows = append(rows, row{kind: rowRecord, item: item})
			}
			m.stale = false
			cmd := m.filter.SetItems(&m.list, rows)
			return m, cmd
		}
		m.selected = result.item
		m.screen = detail
		m.viewport.SetContent(result.item.Thought)
		m.viewport.GotoTop()
		if result.operation == logging.ThoughtCreate {
			m.input.Reset()
			m.stale = true
			m.logger.Mutation(logging.ThoughtCreated, result.item.ThoughtID)
			id := m.subjectID
			return m, func() tea.Msg { return ChangedMsg{SubjectID: id} }
		}
		return m, nil
	}
	if key, ok := msg.(tea.KeyPressMsg); ok {
		if m.browsingThoughtsView && m.screen == browse {
			return m.updateBrowseThoughtsView(msg)
		}
		if m.screen == detail && key.String() == "q" {
			return m, tea.Quit
		}
		if m.loading {
			return m, nil
		}
		switch m.screen {
		case create:
			switch key.String() {
			case "esc":
				m.request++
				m.input.Blur()
				m.input.Reset()
				m.err = nil
				m.screen = browse
				return m, nil
			case "ctrl+s":
				m.inputWarning = ""
				m.input.Blur()
				cmd := m.createThought(m.input.Value())
				return m, cmd
			}
		case detail:
			switch key.String() {
			case "esc":
				m.screen = browse
				m.err = nil
				if m.stale {
					if m.browsingThoughtsView {
						cmd := m.reloadBrowseThoughtsView()
						return m, cmd
					}
					cmd := m.listThoughts()
					return m, cmd
				}
				return m, nil
			case "r":
				cmd := m.getThought(m.selected.ThoughtID)
				return m, cmd
			}
		case browse:
			if !m.list.SettingFilter() {
				switch key.String() {
				case "r":
					cmd := m.listThoughts()
					return m, cmd
				case "enter":
					item, ok := m.list.SelectedItem().(row)
					if !ok {
						return m, nil
					}
					m.err = nil
					switch item.kind {
					case rowCreate:
						m.request++
						m.inputWarning = ""
						m.screen = create
						return m, m.input.Focus()
					case rowRecord:
						cmd := m.getThought(item.item.ThoughtID)
						return m, cmd
					}
				}
			}
		}
	}
	var cmd tea.Cmd
	switch m.screen {
	case browse:
		cmd = m.filter.Update(&m.list, msg)
	case create:
		return m.updateInput(msg)
	case detail:
		m.viewport, cmd = m.viewport.Update(msg)
	}
	return m, cmd
}

func (m Model) View() string {
	status := ""
	if m.loading {
		status = "Loading…\n"
	} else if m.err != nil {
		status = m.errMessage + "\n"
	}
	switch m.screen {
	case create:
		if m.inputWarning != "" {
			status = m.inputWarning + "\n"
		}
		return "Create thought\n" + status + m.input.View() + "\nCtrl+S: save • Enter: newline • Esc: cancel"
	case detail:
		return fmt.Sprintf("Thought %d • %s\n%s%s\n↑/↓: scroll • PgUp/PgDn: page • Esc: thoughts • r: reload • q: quit", m.selected.ThoughtID, displaytime.Format(m.selected.ObservedAt, "Jan 2, 2006 3:04:05 PM MST"), status, m.viewport.View())
	default:
		if m.browsingThoughtsView {
			return m.renderBrowseThoughtsView(status)
		}
		count := 0
		for _, item := range m.list.VisibleItems() {
			if row, ok := item.(row); ok && row.kind == rowRecord {
				count++
			}
		}
		label := "thoughts"
		if count == 1 {
			label = "thought"
		}
		return fmt.Sprintf("%s\n%d %s\nr: refresh", strings.TrimSuffix(status+m.list.View(), "\n"), count, label)
	}
}
