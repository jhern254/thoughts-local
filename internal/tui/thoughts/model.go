// Package thoughts owns the thought list, editor, and detail inside a subject.
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
	"github.com/jhern254/go-thoughts/internal/tui/listfilter"
)

type Service interface {
	List(context.Context, string, int64) ([]data.Thought, error)
	Get(context.Context, string, int64) (*data.Thought, error)
	Create(context.Context, string, string, *int64, time.Time) (*data.Thought, error)
}

// ChangedMsg invalidates the parent subject counts after successful creation.
type ChangedMsg struct{ SubjectID int64 }

type screen uint8

const (
	browse screen = iota
	create
	detail
)

type Model struct {
	ctx        context.Context
	userID     string
	service    Service
	logger     logging.Logger
	subjectID  int64
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

	inputWarning string
	filter       listfilter.Scope
}

type row struct{ item data.Thought }

func (r row) Title() string {
	if r.item.ThoughtID == 0 {
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
	if r.item.ThoughtID == 0 {
		return "Add a thought to this subject"
	}
	return r.item.ObservedAt.UTC().Format("Jan 2, 2006 15:04 UTC")
}
func (r row) FilterValue() string { return r.Title() }

// Result carries one asynchronous service result with its request context.
// Only the component constructs and interprets these messages.
type Result struct {
	request   uint64
	operation logging.Operation
	items     []data.Thought
	item      *data.Thought
	err       error
}

func New(ctx context.Context, userID string, service Service, logger logging.Logger) Model {
	items := list.New([]list.Item{row{}}, list.NewDefaultDelegate(), 80, 14)
	items.Title = "Thoughts"
	items.SetStatusBarItemName("thought", "thoughts")
	items.DisableQuitKeybindings()
	input := textarea.New()
	input.Placeholder = "What is on your mind?"
	input.CharLimit = 0
	input.MaxHeight = 0
	input.MaxWidth = 0
	view := viewport.New()
	view.SoftWrap = true
	m := Model{ctx: ctx, userID: userID, service: service, logger: logger, list: items, input: input, viewport: view}
	m.Resize(80, 14)
	return m
}

func (m *Model) Resize(width, height int) {
	m.list.SetSize(max(1, width), max(1, height-3))
	m.input.SetWidth(max(1, width-2))
	m.input.SetHeight(max(1, height-5))
	m.viewport.SetWidth(max(1, width))
	m.viewport.SetHeight(max(1, height-5))
}

func (m *Model) Reset() {
	m.filter.Invalidate()
	m.request++
	m.subjectID = 0
	m.selected = nil
	m.screen = browse
	m.loading = false
	m.stale = false
	m.err = nil
	m.input.Blur()
	m.input.Reset()
	m.inputWarning = ""
	m.list.ResetFilter()
	m.list.SetItems([]list.Item{row{}})
	m.list.Select(0)
}

func (m *Model) Open(subjectID int64) tea.Cmd {
	m.Reset()
	m.subjectID = subjectID
	return m.load(logging.ThoughtList, 0, "")
}

// Browsing allows the parent to handle subject shortcuts, but not editor input
// or keys used to set/clear a thought-list filter.
func (m Model) Browsing() bool {
	return m.screen == browse && !m.list.SettingFilter() && !m.list.IsFiltered()
}

func (m *Model) load(operation logging.Operation, id int64, body string) tea.Cmd {
	m.request++
	m.loading = true
	m.err = nil
	request, ctx, userID, subjectID, service := m.request, m.ctx, m.userID, m.subjectID, m.service
	return func() tea.Msg {
		result := Result{request: request, operation: operation}
		switch operation {
		case logging.ThoughtList:
			result.items, result.err = service.List(ctx, userID, subjectID)
		case logging.ThoughtGet:
			result.item, result.err = service.Get(ctx, userID, id)
		case logging.ThoughtCreate:
			result.item, result.err = service.Create(ctx, userID, body, &subjectID, time.Time{})
		}
		return result
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg.(type) {
	case listfilter.Reply, list.FilterMatchesMsg:
		cmd := m.filter.Update(&m.list, msg)
		return m, cmd
	}
	if result, ok := msg.(Result); ok {
		if result.request != m.request {
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
			rows = append(rows, row{})
			for _, item := range result.items {
				rows = append(rows, row{item: item})
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
				cmd := m.load(logging.ThoughtCreate, 0, m.input.Value())
				return m, cmd
			}
		case detail:
			switch key.String() {
			case "esc":
				m.screen = browse
				m.err = nil
				if m.stale {
					cmd := m.load(logging.ThoughtList, 0, "")
					return m, cmd
				}
				return m, nil
			case "r":
				cmd := m.load(logging.ThoughtGet, m.selected.ThoughtID, "")
				return m, cmd
			}
		case browse:
			if !m.list.SettingFilter() {
				switch key.String() {
				case "r":
					cmd := m.load(logging.ThoughtList, 0, "")
					return m, cmd
				case "enter":
					item, ok := m.list.SelectedItem().(row)
					if !ok {
						return m, nil
					}
					m.err = nil
					if item.item.ThoughtID == 0 {
						m.request++
						m.inputWarning = ""
						m.screen = create
						return m, m.input.Focus()
					}
					cmd := m.load(logging.ThoughtGet, item.item.ThoughtID, "")
					return m, cmd
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
		return fmt.Sprintf("Thought %d • %s\n%s%s\n↑/↓: scroll • PgUp/PgDn: page • Esc: thoughts • r: reload • q: quit", m.selected.ThoughtID, m.selected.ObservedAt.UTC().Format(time.RFC3339), status, m.viewport.View())
	default:
		return strings.TrimSuffix(status+m.list.View(), "\n") + "\nr: refresh"
	}
}
