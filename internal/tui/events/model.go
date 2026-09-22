// Package events owns the daily home timeline and Start/End forms.
package events

import (
	"context"
	"slices"
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
	Create(context.Context, string, string, time.Time, *int64) (*data.Event, error)
	End(context.Context, string, int64, int64, time.Time) (*data.Event, error)
}
type TimelineView interface {
	thoughts.TimelineReader
	OpenThoughtsView(context.Context, string, int64) (timeline.ThoughtScope, error)
	LatestThought(context.Context, string, int64) (*data.ThoughtSummaryView, error)
	ThoughtCounts(context.Context, string, time.Time, time.Time) ([]data.EventThoughtCountView, error)
}

// Asynchronous replies carry the model identity and the generation of the state
// that requested them. Update accepts a reply only while both still match.
type tickMsg struct {
	owner   *int
	session uint64
	at      time.Time
}
type listedMsg struct {
	owner   *int
	request uint64
	items   []data.Event
	err     error
}
type loadDelayedMsg struct {
	owner   *int
	request uint64
}
type countsMsg struct {
	owner   *int
	request uint64
	items   []data.EventThoughtCountView
	err     error
}
type latestMsg struct {
	owner   *int
	request uint64
	item    *data.ThoughtSummaryView
	err     error
}
type openedMsg struct {
	owner   *int
	request uint64
	scope   timeline.ThoughtScope
	err     error
}
type savedMsg struct {
	owner   *int
	request uint64
	item    *data.Event
	ending  bool
	err     error
}

// Owns identifies Events-only messages. Instance and generation checks still
// happen in Update; shared Thought replies keep their existing root routing.
func Owns(msg tea.Msg) bool {
	_, ok := msg.(message)
	return ok
}

type message interface{ eventMessage() }

func (tickMsg) eventMessage()        {}
func (listedMsg) eventMessage()      {}
func (countsMsg) eventMessage()      {}
func (latestMsg) eventMessage()      {}
func (loadDelayedMsg) eventMessage() {}
func (openedMsg) eventMessage()      {}
func (savedMsg) eventMessage()       {}

type Model struct {
	ctx           context.Context
	userID        string
	service       Service
	subjectReader SubjectReader
	timelineView  TimelineView
	logger        logging.Logger
	now           func() time.Time

	// Clock, expansion, and form lifetimes are independent of a day load.
	owner                    *int
	session, expansion, save uint64

	// Day and expansion failures share the screen's safe status message.
	err     error
	message string

	// day and items are the accepted timeline. load.pendingDay is not presented as the
	// current day until its matching listedMsg reply succeeds.
	clock, day time.Time
	items      []data.Event
	counts     []data.EventThoughtCountView
	latest     *data.ThoughtSummaryView

	distributions distributionSettings
	position      timelinePosition
	load          dayLoad
	width, height int

	// expanded delegates keys and rendering to picker without replacing the day.
	expanded int64
	opening  bool
	picker   thoughts.Model
	form     eventForm

	// blurred preserves selection while the root entity strip owns keyboard focus.
	active  bool
	blurred bool
}

func New(ctx context.Context, userID string, service Service, view TimelineView, thoughtService thoughts.Service, subjects SubjectReader, logger logging.Logger) Model {
	now := time.Now()
	return Model{
		ctx:           ctx,
		userID:        userID,
		service:       service,
		subjectReader: subjects,
		timelineView:  view,
		logger:        logger,
		now:           time.Now,

		owner:         new(int),
		clock:         now,
		day:           displaytime.Day(now),
		distributions: defaultDistributionSettings(),
		position:      timelinePosition{followNow: true},
		width:         80,
		height:        20,

		picker: thoughts.New(ctx, userID, thoughtService, logger),
	}
}

func (m *Model) Open() tea.Cmd {
	m.active = true
	m.session++
	m.clock = m.now()
	day := m.day
	if day.IsZero() || m.position.followNow {
		day = displaytime.Day(m.clock)
	}
	return tea.Batch(m.loadDay(day), m.tick())
}
func (m *Model) Close() {
	m.distributions.open = false
	m.load.invalidate()
	m.active = false
	// Invalidate replies that can outlive this screen opening.
	m.session++
	m.expansion++
	m.picker.Reset()
	m.expanded = 0
}
func (m *Model) SetFocused(focused bool) {
	blurred := !focused
	if blurred {
		m.distributions.open = false
		m.position.followNow = false
	}
	if m.blurred != blurred {
		m.load.retainedBody = ""
	}
	m.blurred = blurred
	m.picker.SetFocused(focused)
}
func (m Model) CanLeave() bool { return !m.form.open && !m.picker.ShowingDetail() }
func (m Model) FormOpen() bool { return m.form.open }
func (m *Model) Resize(width, height int) {
	m.load.retainedBody = ""
	m.width, m.height = max(1, width), max(1, height)
	m.picker.Resize(max(1, width-12), max(1, height-2))
	// Leave calendar context around the expanded box, not just room for the picker.
	m.picker.ResizeEventView(m.cardWidth(), max(1, (height-14)/3))
	if m.form.open {
		m.resizeForm()
	}
	m.anchor()
}
func (m Model) tick() tea.Cmd {
	owner, session := m.owner, m.session
	delay := time.Minute - m.clock.Sub(m.clock.Truncate(time.Minute))
	return tea.Tick(delay, func(at time.Time) tea.Msg { return tickMsg{owner, session, at} })
}

// loadDay keeps the displayed date with its cards until the requested list arrives.
func (m *Model) loadDay(day time.Time) tea.Cmd {
	// Hold only the visible body during this read, not data for other days.
	// Rapid keys and obsolete replies must not repeatedly format unchanged cards.
	if !m.load.eventsPending || m.load.retainedBody == "" {
		m.load.retainedBody = m.timelineBody()
	}
	feedback := m.load.begin(m.ctx, day, m.owner)
	m.expansion++
	m.opening = false
	m.err = nil
	m.message = ""
	owner, request, ctx, user, service, view := m.owner, m.load.generation, m.load.ctx, m.userID, m.service, m.timelineView
	return tea.Batch(func() tea.Msg {
		if err := ctx.Err(); err != nil {
			return listedMsg{owner: owner, request: request, err: err}
		}
		items, err := service.List(ctx, user, day, day.AddDate(0, 0, 1))
		return listedMsg{owner, request, items, err}
	}, func() tea.Msg {
		if err := ctx.Err(); err != nil {
			return countsMsg{owner: owner, request: request, err: err}
		}
		items, err := view.ThoughtCounts(ctx, user, day, day.AddDate(0, 0, 1))
		return countsMsg{owner, request, items, err}
	}, feedback)
}
func (m *Model) openEvent(id int64) tea.Cmd {
	if m.expanded != id {
		m.picker.Reset()
	}
	m.expansion++
	m.expanded = id
	m.opening = true
	m.position.followNow = false
	owner, request, ctx, user, view := m.owner, m.expansion, m.ctx, m.userID, m.timelineView
	return func() tea.Msg {
		scope, err := view.OpenThoughtsView(ctx, user, id)
		return openedMsg{owner, request, scope, err}
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
	case subjectsLoaded:
		m.acceptSubjects(result)
		return m, nil
	case tickMsg:
		if result.owner != m.owner || result.session != m.session {
			return m, nil
		}
		m.clock = result.at
		m.load.retainedBody = ""
		var reload tea.Cmd
		if m.position.followNow && !m.day.Equal(displaytime.Day(m.clock)) && (!m.load.eventsPending || !m.load.pendingDay.Equal(displaytime.Day(m.clock))) {
			reload = m.loadDay(displaytime.Day(m.clock))
		}
		m.anchor()
		return m, tea.Batch(reload, m.tick())
	case loadDelayedMsg:
		if result.owner == m.owner {
			m.load.delayFeedback(result.request)
		}
		return m, nil
	case listedMsg:
		// The current model instance and newest day request exclusively own this reply.
		if result.owner != m.owner || !m.load.owns(result.request) || !m.load.eventsPending {
			return m, nil
		}
		m.load.finishEvents()
		m.err = result.err
		m.fail(logging.EventList, result.err)
		if result.err != nil {
			m.message = "Could not load events for " + displaytime.Format(m.load.pendingDay, "January 2, 2006") + "."
			return m, nil
		}
		if !m.day.Equal(m.load.pendingDay) {
			m.day = m.load.pendingDay
			m.position.eventIndex, m.position.topLine, m.expanded = 0, 0, 0
			m.items = nil
			m.picker.Reset()
		}
		if m.load.countsPending {
			// Only retain successful counts for unchanged displayed intervals.
			sameIntervals := slices.EqualFunc(m.items, result.items, func(a, b data.Event) bool {
				sameEnd := a.EndedAt == nil && b.EndedAt == nil
				if a.EndedAt != nil && b.EndedAt != nil {
					sameEnd = a.EndedAt.Equal(*b.EndedAt)
				}
				return a.EventID == b.EventID && a.StartedAt.Equal(b.StartedAt) && sameEnd
			})
			if !sameIntervals || m.load.countErr != nil {
				m.counts = nil
			}
			m.load.countErr = nil
		} else {
			m.counts, m.load.countErr = m.load.pendingCounts.items, m.load.pendingCounts.err
		}
		selected := int64(0)
		if len(m.items) > 0 {
			selected = m.items[m.position.eventIndex].EventID
		}
		m.items = result.items
		m.position.clampSelection(len(m.items))
		var latest, open tea.Cmd
		foundExpanded := false
		m.latest = nil
		m.load.latestErr = nil
		for i, item := range m.items {
			if item.EventID == selected {
				m.position.eventIndex = i
			}
			if item.EventID == m.expanded {
				foundExpanded = true
				open = m.openEvent(item.EventID)
			}
			if item.EndedAt == nil {
				if m.position.followNow {
					m.position.eventIndex = i
				}
				m.load.latestPending = true
				owner, request, ctx, user, view, id := m.owner, m.load.generation, m.load.ctx, m.userID, m.timelineView, item.EventID
				latest = func() tea.Msg {
					if err := ctx.Err(); err != nil {
						return latestMsg{owner: owner, request: request, err: err}
					}
					item, err := view.LatestThought(ctx, user, id)
					return latestMsg{owner, request, item, err}
				}
			}
		}
		if !foundExpanded {
			m.expanded = 0
			m.picker.Reset()
		}
		m.anchor()
		if selected == 0 && !m.position.followNow && len(m.items) > 0 {
			layout := m.measureTimeline()
			m.position.topLine = layout.positions[m.items[0].EventID]
			m.position.clamp(len(layout.lines))
		}
		return m, tea.Batch(latest, open)
	case countsMsg:
		// Counts share the day-request generation but may arrive before its event list.
		if result.owner != m.owner || !m.load.owns(result.request) {
			return m, nil
		}
		m.load.finishCounts(result)
		m.fail(logging.EventThoughtCount, result.err)
		if m.load.eventsPending || !m.load.pendingDay.Equal(m.day) {
			return m, nil
		}
		m.counts, m.load.countErr = result.items, result.err
		m.anchor()
		return m, nil
	case latestMsg:
		// The latest preview inherits the accepted list's generation.
		if result.owner != m.owner || !m.load.owns(result.request) {
			return m, nil
		}
		m.load.latestPending = false
		m.latest, m.load.latestErr = result.item, result.err
		m.fail(logging.EventThoughtList, result.err)
		m.anchor()
		return m, nil
	case openedMsg:
		// Expansion has its own generation so changing/collapsing cards invalidates it.
		if result.owner != m.owner || result.request != m.expansion {
			return m, nil
		}
		m.opening = false
		m.load.retainedBody = ""
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
	case savedMsg:
		// Save ownership prevents an abandoned or replaced form from handling a reply.
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
		day := m.day
		if !result.ending {
			m.clock = m.now()
			day = displaytime.Day(m.clock)
			m.position.followNow = true
		}
		cmd := m.loadDay(day)
		return m, cmd
	case thoughts.Result, thoughts.BrowseThoughtsResult, thoughts.ThoughtCountResult:
		m.load.retainedBody = ""
		var cmd tea.Cmd
		m.picker, cmd = m.picker.Update(msg)
		if !m.picker.ShowingDetail() {
			m.anchor()
		}
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
	if m.distributions.open {
		m.tuneDistributions(key.String())
		return m, nil
	}
	if key.String() == "d" {
		m.distributions.open = true
		m.load.retainedBody = ""
		return m, nil
	}
	if m.expanded != 0 {
		if key.String() == "esc" || key.String() == "left" || key.String() == "h" {
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
		if key.String() == "right" || key.String() == "l" {
			msg = tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
		}
		var cmd tea.Cmd
		m.picker, cmd = m.picker.Update(msg)
		return m, cmd
	}
	day := m.day
	if m.load.eventsPending {
		day = m.load.pendingDay
	}
	navigation := key.String()
	if (navigation == "right" || navigation == "l") && day.Equal(displaytime.Day(m.now())) {
		if m.load.eventsPending && !day.Equal(m.day) {
			return m, nil
		}
		navigation = "enter"
	}
	// Do not act on the previous day's cards while navigation is pending.
	if m.load.eventsPending && !m.load.pendingDay.Equal(m.day) {
		switch navigation {
		case "left", "right", "h", "l", "end", "r", "n":
		default:
			return m, nil
		}
	}
	switch navigation {
	case "left", "right", "h", "l", "r":
	default:
		m.load.retainedBody = ""
	}
	switch navigation {
	case "end":
		m.position.followNow = true
		m.clock = m.now()
		day := displaytime.Day(m.clock)
		if !day.Equal(m.day) || m.load.eventsPending {
			cmd := m.loadDay(day)
			return m, cmd
		}
		m.anchor()
	case "home":
		m.position.first()
		m.revealSelected()
	case "left", "right", "h", "l":
		delta := 1
		if key.String() == "left" || key.String() == "h" {
			delta = -1
		}
		day := day.AddDate(0, 0, delta)
		if day.After(displaytime.Day(m.now())) {
			return m, nil
		}
		m.position.followNow = false
		cmd := m.loadDay(day)
		return m, cmd
	case "r":
		cmd := m.loadDay(day)
		return m, cmd
	case "up", "k", "down", "j":
		delta := 1
		if key.String() == "up" || key.String() == "k" {
			delta = -1
		}
		m.position.moveEvent(delta, len(m.items))
		m.revealSelected()
	case "pgup", "pgdown":
		direction := 1
		if key.String() == "pgup" {
			direction = -1
		}
		layout := m.measureTimeline()
		m.position.scrollPage(direction, m.height, len(layout.lines))
	case "enter":
		if len(m.items) > 0 {
			cmd := m.openEvent(m.items[m.position.eventIndex].EventID)
			return m, cmd
		}
	case "esc":
		m.expansion++
		m.expanded = 0
		m.opening = false
		m.picker.Reset()
		layout := m.measureTimeline()
		m.position.clamp(len(layout.lines))
	case "n":
		cmd := m.startForm(false)
		return m, cmd
	case "e":
		if len(m.items) > 0 && m.items[m.position.eventIndex].EndedAt == nil {
			cmd := m.startForm(true)
			return m, cmd
		}
	}
	return m, nil
}
