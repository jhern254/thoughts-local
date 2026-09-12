// Package events owns the daily home timeline and Start/End forms.
package events

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/diagnostics"
	"github.com/jhern254/go-thoughts/internal/failure"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/timeline"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
)

type Service interface {
	List(context.Context, string, time.Time, time.Time) ([]data.Event, error)
	Create(context.Context, string, string, time.Time) (*data.Event, error)
	End(context.Context, string, int64, int64, time.Time) (*data.Event, error)
}
type TimelineView interface {
	thoughts.TimelineReader
	OpenThoughtsView(context.Context, string, int64) (timeline.ThoughtScope, error)
	LatestThought(context.Context, string, int64) (*data.ThoughtSummary, error)
	ThoughtCounts(context.Context, string, time.Time, time.Time) ([]data.EventThoughtCount, error)
}

type Tick struct {
	owner   *int
	session uint64
	at      time.Time
}
type Listed struct {
	owner   *int
	request uint64
	items   []data.Event
	err     error
}
type Counts struct {
	owner   *int
	request uint64
	items   []data.EventThoughtCount
	err     error
}
type Latest struct {
	owner   *int
	request uint64
	item    *data.ThoughtSummary
	err     error
}
type Opened struct {
	owner   *int
	request uint64
	scope   timeline.ThoughtScope
	err     error
}
type Saved struct {
	owner   *int
	request uint64
	item    *data.Event
	ending  bool
	err     error
}

type Model struct {
	ctx                                  context.Context
	userID                               string
	service                              Service
	timelineView                         TimelineView
	logger                               logging.Logger
	owner                                *int
	now                                  func() time.Time
	clock, day                           time.Time
	active, following                    bool
	blurred                              bool
	session, request, expansion, save    uint64
	items                                []data.Event
	index, offset, width, height         int
	loading, countPending, latestPending bool
	err, countErr, latestErr             error
	message                              string
	counts                               []data.EventThoughtCount
	latest                               *data.ThoughtSummary
	expanded                             int64
	inside, opening                      bool
	picker                               thoughts.Model
	form                                 eventForm
}

func New(ctx context.Context, userID string, service Service, view TimelineView, thoughtService thoughts.Service, logger logging.Logger) Model {
	now := time.Now()
	return Model{ctx: ctx, userID: userID, service: service, timelineView: view, logger: logger, owner: new(int), now: time.Now, clock: now, day: displaytime.Day(now), following: true, width: 80, height: 20, picker: thoughts.New(ctx, userID, thoughtService, logger)}
}

func (m *Model) Open() tea.Cmd {
	m.active = true
	m.session++
	m.clock = m.now()
	if m.day.IsZero() || m.following {
		m.day = displaytime.Day(m.clock)
	}
	return tea.Batch(m.reload(), m.tick())
}
func (m *Model) Close() {
	m.active = false
	m.session++
	m.request++
	m.expansion++
	m.picker.Reset()
	m.expanded = 0
	m.inside = false
}
func (m *Model) Pause() { m.following = false }
func (m *Model) SetFocused(focused bool) {
	m.blurred = !focused
	m.picker.SetFocused(focused)
}
func (m Model) CanLeave() bool { return !m.form.open && !m.picker.ShowingDetail() }
func (m Model) FormOpen() bool { return m.form.open }
func (m *Model) Resize(width, height int) {
	m.width, m.height = max(1, width), max(1, height)
	m.picker.Resize(max(1, width-12), max(1, height-2))
	// Leave calendar context around the expanded box, not just room for the picker.
	m.picker.ResizeEventView(max(1, width-12), max(1, (height-14)/3))
	if m.form.open {
		m.resizeForm()
	}
	m.anchor()
}
func (m Model) tick() tea.Cmd {
	owner, session := m.owner, m.session
	delay := time.Minute - m.clock.Sub(m.clock.Truncate(time.Minute))
	return tea.Tick(delay, func(at time.Time) tea.Msg { return Tick{owner, session, at} })
}
func (m *Model) reload() tea.Cmd {
	m.request++
	m.expansion++
	m.opening = false
	m.loading, m.countPending, m.latestPending = true, true, false
	m.err, m.countErr, m.latestErr = nil, nil, nil
	m.message = ""
	owner, request, ctx, user, service, view, day := m.owner, m.request, m.ctx, m.userID, m.service, m.timelineView, m.day
	return tea.Batch(func() tea.Msg {
		items, err := service.List(ctx, user, day, day.AddDate(0, 0, 1))
		return Listed{owner, request, items, err}
	}, func() tea.Msg {
		items, err := view.ThoughtCounts(ctx, user, day, day.AddDate(0, 0, 1))
		return Counts{owner, request, items, err}
	})
}
func (m *Model) openEvent(id int64) tea.Cmd {
	if m.expanded != id {
		m.picker.Reset()
	}
	m.expansion++
	m.expanded = id
	m.inside = true
	m.opening = true
	m.following = false
	owner, request, ctx, user, view := m.owner, m.expansion, m.ctx, m.userID, m.timelineView
	return func() tea.Msg {
		scope, err := view.OpenThoughtsView(ctx, user, id)
		return Opened{owner, request, scope, err}
	}
}
func (m *Model) fail(operation logging.Operation, err error) {
	if category, emit := failure.Classify(operation, err); emit {
		m.logger.Failure(operation, category)
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.active {
		return m, nil
	}
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "ctrl+c" {
		return m, tea.Quit
	}
	switch result := msg.(type) {
	case Tick:
		if result.owner != m.owner || result.session != m.session {
			return m, nil
		}
		m.clock = result.at
		var reload tea.Cmd
		if m.following && !m.day.Equal(displaytime.Day(m.clock)) {
			m.day = displaytime.Day(m.clock)
			m.items = nil
			m.index = 0
			reload = m.reload()
		}
		m.anchor()
		return m, tea.Batch(reload, m.tick())
	case Listed:
		if result.owner != m.owner || result.request != m.request {
			return m, nil
		}
		m.loading = false
		m.err = result.err
		m.fail(logging.EventList, result.err)
		if result.err != nil {
			m.message = "Could not load events."
			return m, nil
		}
		selected := int64(0)
		if len(m.items) > 0 {
			selected = m.items[m.index].EventID
		}
		m.items = result.items
		m.index = min(m.index, max(0, len(m.items)-1))
		var latest, open tea.Cmd
		foundExpanded := false
		m.latest = nil
		for i, item := range m.items {
			if item.EventID == selected {
				m.index = i
			}
			if item.EventID == m.expanded {
				foundExpanded = true
				inside := m.inside
				open = m.openEvent(item.EventID)
				m.inside = inside
			}
			if item.EndedAt == nil {
				if m.following {
					m.index = i
				}
				m.latestPending = true
				owner, request, ctx, user, view, id := m.owner, m.request, m.ctx, m.userID, m.timelineView, item.EventID
				latest = func() tea.Msg {
					item, err := view.LatestThought(ctx, user, id)
					return Latest{owner, request, item, err}
				}
			}
		}
		if !foundExpanded {
			m.expanded = 0
			m.inside = false
			m.picker.Reset()
		}
		m.anchor()
		return m, tea.Batch(latest, open)
	case Counts:
		if result.owner != m.owner || result.request != m.request {
			return m, nil
		}
		m.countPending = false
		m.counts, m.countErr = result.items, result.err
		m.fail(logging.EventThoughtCount, result.err)
		m.anchor()
		return m, nil
	case Latest:
		if result.owner != m.owner || result.request != m.request {
			return m, nil
		}
		m.latestPending = false
		m.latest, m.latestErr = result.item, result.err
		m.fail(logging.EventThoughtList, result.err)
		m.anchor()
		return m, nil
	case Opened:
		if result.owner != m.owner || result.request != m.expansion {
			return m, nil
		}
		m.opening = false
		m.err = result.err
		m.fail(logging.EventGet, result.err)
		if result.err != nil {
			m.message = diagnostics.EventMessage(result.err, "Could not open event.")
			return m, nil
		}
		item := result.scope.Event()
		for i := range m.items {
			if m.items[i].EventID == item.EventID {
				m.items[i] = item
			}
		}
		cmd := m.picker.OpenEventView(result.scope, m.timelineView)
		m.anchor()
		return m, cmd
	case Saved:
		if result.owner != m.owner || result.request != m.save {
			return m, nil
		}
		m.form.saving = false
		m.form.err = result.err
		op := logging.EventCreate
		if result.ending {
			op = logging.EventEnd
		}
		m.fail(op, result.err)
		if result.err != nil {
			m.form.message = diagnostics.EventMessage(result.err, "Could not save event.")
			cmd := m.focusForm()
			return m, cmd
		}
		mutation := logging.EventCreated
		if result.ending {
			mutation = logging.EventEnded
		}
		m.logger.Mutation(mutation, result.item.EventID)
		m.form = eventForm{}
		cmd := m.reload()
		return m, cmd
	case thoughts.Result, thoughts.BrowseThoughtsResult, thoughts.ThoughtCountResult:
		var cmd tea.Cmd
		m.picker, cmd = m.picker.Update(msg)
		return m, cmd
	}
	if m.form.open {
		cmd := m.updateForm(msg)
		return m, cmd
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	if key.String() == "q" || key.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.picker.ShowingDetail() {
		var cmd tea.Cmd
		m.picker, cmd = m.picker.Update(msg)
		return m, cmd
	}
	if m.inside {
		if key.String() == "esc" {
			m.inside = false
			m.expansion++
			m.expanded = 0
			m.opening = false
			m.picker.Reset()
			m.revealSelected()
			return m, nil
		}
		if key.String() == "r" {
			cmd := m.openEvent(m.expanded)
			return m, cmd
		}
		var cmd tea.Cmd
		m.picker, cmd = m.picker.Update(msg)
		return m, cmd
	}
	switch key.String() {
	case "end":
		m.following = true
		m.clock = m.now()
		day := displaytime.Day(m.clock)
		if !day.Equal(m.day) {
			m.day = day
			m.items = nil
			m.index = 0
			cmd := m.reload()
			return m, cmd
		}
		m.anchor()
	case "home":
		m.following = false
		m.index = 0
		m.offset = 0
		m.revealSelected()
	case "[", "]":
		m.following = false
		delta := 1
		if key.String() == "[" {
			delta = -1
		}
		m.day = m.day.AddDate(0, 0, delta)
		m.offset = 0
		m.index = 0
		m.items = nil
		m.expanded = 0
		m.picker.Reset()
		cmd := m.reload()
		return m, cmd
	case "r":
		cmd := m.reload()
		return m, cmd
	case "up", "k", "down", "j":
		m.following = false
		delta := 1
		if key.String() == "up" || key.String() == "k" {
			delta = -1
		}
		m.index = min(max(0, m.index+delta), max(0, len(m.items)-1))
		m.revealSelected()
	case "pgup", "pgdown":
		m.following = false
		delta := m.height - 2
		if key.String() == "pgup" {
			delta = -delta
		}
		m.offset += delta
		m.clampOffset()
	case "enter":
		if len(m.items) > 0 {
			id := m.items[m.index].EventID
			if id == m.expanded {
				m.inside = true
				return m, nil
			}
			cmd := m.openEvent(id)
			return m, cmd
		}
	case "esc":
		m.expansion++
		m.expanded = 0
		m.opening = false
		m.picker.Reset()
		m.clampOffset()
	case "n":
		cmd := m.startForm(false)
		return m, cmd
	case "e":
		if len(m.items) > 0 && m.items[m.index].EndedAt == nil {
			cmd := m.startForm(true)
			return m, cmd
		}
	}
	return m, nil
}
