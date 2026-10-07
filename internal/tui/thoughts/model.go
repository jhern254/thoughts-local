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
	BrowseView(context.Context, string, data.ThoughtSummaryViewRequest) (data.ThoughtSummaryViewResult, error)
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
	ctx     context.Context
	userID  string
	service Service
	logger  logging.Logger

	// owner separates model instances. request owns list/get/create and cursor
	// replies; statsRequest lets a full-scope statistics refresh independently.
	owner           *int
	request         uint64
	statsRequest    uint64
	cancelRead      context.CancelFunc
	cancelStats     context.CancelFunc
	cancelClipboard context.CancelFunc

	// screen selects list, editor, or detail behavior. selected survives detail
	// rendering, while the list or bounded browse state owns row selection.
	screen              screen
	subjectID           *int64
	list                list.Model
	input               textarea.Model
	viewport            viewport.Model
	voice               voiceDraft
	selected            *data.Thought
	selectedSubjectName string

	loading    bool
	stale      bool
	err        error
	errMessage string

	inputWarning string
	// filter owns asynchronous Bubbles matches for the current list revision.
	filter listfilter.Scope
	// browsingThoughtsView switches from subject rows to the bounded summary browser.
	browsingThoughtsView bool
	browseThoughts       browseThoughtsState
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

// Result carries one asynchronous service result with its request ownership.
// operation describes logging/classification; it never selects the service call.
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
	if m.VoiceOpen() {
		m.resizeVoice(width, height)
	}
}

func (m *Model) Reset() {
	m.stopVoice()
	// Invalidate both result streams before replacing their visible state.
	if m.cancelRead != nil {
		m.cancelRead()
	}
	if m.cancelStats != nil {
		m.cancelStats()
	}
	m.stopClipboard()
	m.statsRequest++
	m.browsingThoughtsView = false
	m.browseThoughts = browseThoughtsState{width: m.browseThoughts.width, height: m.browseThoughts.height}
	m.filter.Invalidate()
	m.request++
	m.subjectID = nil
	m.selected = nil
	m.selectedSubjectName = ""
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

// Creating lets the parent equip and render the shared thought editor.
func (m Model) Creating() bool { return m.screen == create }

// ShowingDetail lets the parent render the shared detail without list context.
func (m Model) ShowingDetail() bool { return m.screen == detail }

// beginRead replaces only the list/detail/page request, never a count or write.
func (m *Model) beginRead(ctx context.Context) context.Context {
	if m.cancelRead != nil {
		m.cancelRead()
	}
	ctx, m.cancelRead = context.WithCancel(ctx)
	return ctx
}

func (m *Model) listThoughts() tea.Cmd {
	m.request++
	m.loading = true
	m.err = nil
	owner := m.owner
	request, ctx, userID, subjectID, service := m.request, m.beginRead(m.ctx), m.userID, m.subjectID, m.service
	return func() tea.Msg {
		if ctx.Err() != nil {
			return nil
		}
		var items []data.Thought
		var err error
		if subjectID == nil {
			items, err = service.ListUnassigned(ctx, userID)
		} else {
			items, err = service.List(ctx, userID, *subjectID)
		}
		if ctx.Err() != nil {
			return nil
		}
		return Result{owner: owner, request: request, operation: logging.ThoughtList, items: items, err: err}
	}
}

func (m *Model) getThought(id int64) tea.Cmd {
	m.request++
	m.loading = true
	m.err = nil
	owner := m.owner
	request, ctx, userID, service := m.request, m.beginRead(m.ctx), m.userID, m.service
	return func() tea.Msg {
		if ctx.Err() != nil {
			return nil
		}
		item, err := service.Get(ctx, userID, id)
		if ctx.Err() != nil {
			return nil
		}
		return Result{owner: owner, request: request, operation: logging.ThoughtGet, item: item, err: err}
	}
}

func (m *Model) createThought(body string) tea.Cmd {
	m.stopClipboard()
	m.request++
	m.loading = true
	m.err = nil
	if m.voice.cancelRead != nil {
		m.voice.cancelRead()
	}
	owner := m.owner
	request, ctx, userID, subjectID, service := m.request, m.ctx, m.userID, m.subjectID, m.service
	return func() tea.Msg {
		if ctx.Err() != nil {
			return nil
		}
		item, err := service.Create(ctx, userID, body, subjectID, time.Time{})
		if ctx.Err() != nil {
			return nil
		}
		return Result{owner: owner, request: request, operation: logging.ThoughtCreate, item: item, err: err}
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if m.ctx.Err() != nil {
		return m, nil
	}
	if action, ok := msg.(VoiceAction); ok {
		return m.updateVoiceAction(action)
	}
	if reply, ok := msg.(voiceSubjectsLoaded); ok {
		return m.acceptVoiceSubjects(reply)
	}
	if m.VoiceOpen() && m.screen == create && m.voice.suspended {
		return m, nil
	}
	if result, ok := msg.(ThoughtBrowseStatsResult); ok {
		return m.receiveThoughtBrowseStats(result)
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
		// Ownership also rejects results already queued before cancellation.
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
			m.selectedSubjectName = m.voice.subjectName
			id := m.subjectID
			if m.VoiceOpen() {
				m.subjectID = m.voice.originalSubjectID
			}
			m.stopVoice()
			m.stale = true
			m.logger.Mutation(logging.ThoughtCreated, result.item.ThoughtID)
			return m, func() tea.Msg { return ChangedMsg{SubjectID: id} }
		}
		return m, nil
	}
	if key, ok := msg.(tea.KeyPressMsg); ok {
		if m.browsingThoughtsView && m.screen == browse {
			return m.updateBrowseThoughtsView(msg)
		}
		if key.String() == "q" && m.screen != create && !m.list.SettingFilter() {
			key = tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
			msg = key
		}
		if m.screen == detail && key.String() == "esc" {
			if m.cancelRead != nil {
				m.cancelRead()
			}
			m.request++
			m.loading = false
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
		}
		if m.loading {
			return m, nil
		}
		switch m.screen {
		case create:
			if m.VoiceOpen() && key.String() == "esc" {
				m.subjectID = m.voice.originalSubjectID
				m.stopVoice()
			}
			if m.VoiceOpen() && key.String() == "f8" {
				action := "start"
				if m.voiceLocked() {
					action = "stop"
				}
				return m.updateVoiceAction(VoiceAction{Action: action, DraftID: m.voice.draftID, RecordingID: m.voice.recordingID})
			}
			if m.voiceLocked() {
				return m, nil
			}
			if m.VoiceOpen() && (key.String() == "tab" || key.String() == "shift+tab") {
				m.stopClipboard()
				m.voice.subjectFocused = !m.voice.subjectFocused
				m.resizeVoice(m.voice.width, m.voice.height)
				if m.voice.subjectFocused {
					m.input.Blur()
					return m, m.voice.query.Focus()
				}
				m.voice.query.Blur()
				return m, m.input.Focus()
			}
			switch key.String() {
			case "esc":
				m.stopClipboard()
				m.request++
				m.input.Blur()
				m.input.Reset()
				m.err = nil
				m.screen = browse
				return m, nil
			case "ctrl+s":
				if m.VoiceOpen() && m.voice.query.Value() != "" && m.subjectID == nil {
					m.voice.message = "Select a subject, create one, or clear the optional field."
					m.voice.subjectFocused = true
					m.resizeVoice(m.voice.width, m.voice.height)
					m.input.Blur()
					return m, m.voice.query.Focus()
				}
				m.inputWarning = ""
				m.input.Blur()
				cmd := m.createThought(m.input.Value())
				return m, cmd
			}
		case detail:
			switch key.String() {
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
		if m.voiceLocked() {
			return m, nil
		}
		if m.VoiceOpen() && m.voice.subjectFocused {
			return m.updateVoicePicker(msg)
		}
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
		return m.editorView(status)
	case detail:
		return fmt.Sprintf("Thought %d • %s\n%s%s\n↑/↓: scroll • PgUp/PgDn: page • Q/Esc: thoughts • r: reload", m.selected.ThoughtID, displaytime.Format(m.selected.ObservedAt, "Jan 2, 2006 3:04:05 PM MST"), status, m.viewport.View())
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
