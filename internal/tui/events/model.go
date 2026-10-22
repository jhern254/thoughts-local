// Package events owns the daily home timeline and Start/End forms.
package events

import (
	"context"
	"errors"
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
	Create(context.Context, string, string, time.Time, *int64) (*data.Event, error)
	End(context.Context, string, int64, int64, time.Time) (*data.Event, error)
}
type TimelineView interface {
	SubjectTimeInRange(context.Context, string, time.Time, time.Time, time.Time) ([]data.SubjectTimeView, error)
	ListEntriesInRange(context.Context, string, time.Time, time.Time) ([]timeline.Entry, error)
	OpenThoughtsOutsideEventsView(context.Context, string, time.Time) (timeline.ThoughtScope, error)
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
type entriesListedMsg struct {
	owner   *int
	request uint64
	entries []timeline.Entry
	err     error
}
type loadDelayedMsg struct {
	owner   *int
	request uint64
}
type eventCountsMsg struct {
	owner   *int
	request uint64
	counts  []data.EventThoughtCountView
	err     error
}
type latestEventThoughtMsg struct {
	owner   *int
	request uint64
	thought *data.ThoughtSummaryView
	err     error
}
type entryOpenedMsg struct {
	owner      *int
	request    uint64
	scope      timeline.ThoughtScope
	err        error
	subject    *data.Subject
	subjectErr error
}
type eventSavedMsg struct {
	owner   *int
	request uint64
	event   *data.Event
	ending  bool
	err     error
}

// Owns identifies messages owned by the Events screen. Instance and generation
// checks still happen in Update; shared Thought replies keep their root routing.
func Owns(msg tea.Msg) bool {
	_, ok := msg.(message)
	return ok
}

type message interface{ eventMessage() }

func (tickMsg) eventMessage()               {}
func (entriesListedMsg) eventMessage()      {}
func (eventCountsMsg) eventMessage()        {}
func (latestEventThoughtMsg) eventMessage() {}
func (loadDelayedMsg) eventMessage()        {}
func (entryOpenedMsg) eventMessage()        {}
func (eventSavedMsg) eventMessage()         {}

type Model struct {
	ctx           context.Context
	userID        string
	service       Service
	subjectReader SubjectReader
	timelineView  TimelineView
	logger        logging.Logger
	now           func() time.Time

	clockCtx        context.Context
	cancelClock     context.CancelFunc
	cancelExpansion context.CancelFunc

	// Clock, expansion, and form lifetimes are independent of a day load.
	owner                    *int
	session, expansion, save uint64

	// Day and expansion failures share the screen's safe status message.
	err     error
	message string

	// day and entries are the accepted chronological timeline. load.pendingDay is not
	// presented as the current day until its matching entriesListedMsg reply succeeds.
	clock, day  time.Time
	entries     []timeline.Entry
	eventCounts []data.EventThoughtCountView
	latest      *data.ThoughtSummaryView

	distributions distributionSettings
	position      timelinePosition
	subjectTime   subjectTimeState
	load          dayLoad
	width, height int

	// expanded delegates keys and rendering to picker without replacing the day.
	expanded timeline.EntryKey
	opening  bool
	// Retain the last badge during revalidation; rendering checks its subject ID.
	expandedSubject    *data.Subject
	expandedSubjectErr error
	picker             thoughts.Model
	form               eventForm

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
		subjectTime:   subjectTimeState{distributions: defaultSubjectTimeDistributionSettings()},
		position:      timelinePosition{target: targetBoundary, followNow: true},
		width:         80,
		height:        20,

		picker: thoughts.New(ctx, userID, thoughtService, logger),
	}
}

func (m *Model) Open() tea.Cmd {
	if m.cancelClock != nil {
		m.cancelClock()
	}
	m.clockCtx, m.cancelClock = context.WithCancel(m.ctx)
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
	if m.cancelClock != nil {
		m.cancelClock()
	}
	m.form.stopReads()
	m.distributions.open = false
	m.load.invalidate()
	m.subjectTime.stop()
	m.active = false
	// Invalidate replies that can outlive this screen opening.
	m.session++
	m.stopExpansion()
	m.expansion++
	m.picker.Reset()
	m.expanded = timeline.EntryKey{}
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
func (m Model) CanLeave() bool {
	return m.TimelineVisible() && !m.picker.SavingMindset()
}
func (m Model) TimelineVisible() bool { return !m.form.open && !m.picker.ShowingDetail() }
func (m Model) Day() time.Time        { return m.day }
func (m Model) DayReady() bool        { return m.active && !m.load.entriesPending }
func (m Model) FormOpen() bool        { return m.form.open }

// AtTopLevel excludes the nested views and controls owned by Events.
func (m Model) AtTopLevel() bool {
	return m.CanLeave() && m.expanded.IsZero() && !m.distributions.open && m.position.target != targetMetrics
}
func (m *Model) Resize(width, height int) {
	m.load.retainedBody = ""
	m.width, m.height = max(1, width), max(1, height)
	m.clampSubjectTime()
	m.picker.Resize(max(1, width-12), max(1, height-2))
	// Leave calendar context around the expanded box, not just room for the picker.
	m.picker.ResizeTimelineEntryView(m.cardWidth(), max(1, (height-14)/3))
	if m.form.open {
		m.resizeForm()
	}
	m.anchor()
}
func (m Model) tick() tea.Cmd {
	owner, session := m.owner, m.session
	ctx := m.clockCtx
	if ctx == nil {
		ctx = m.ctx
	}
	delay := time.Minute - m.clock.Sub(m.clock.Truncate(time.Minute))
	return func() tea.Msg {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil
		case at := <-timer.C:
			return tickMsg{owner, session, at}
		}
	}
}

// loadDay keeps the displayed date with its cards until the requested list arrives.
func (m *Model) loadDay(day time.Time) tea.Cmd {
	// Hold only the visible body during this read, not data for other days.
	// Rapid keys and obsolete replies must not repeatedly format unchanged cards.
	if !m.load.entriesPending || m.load.retainedBody == "" {
		m.load.retainedBody = m.timelineBody()
	}
	feedback := m.load.begin(m.ctx, day, m.owner)
	m.stopExpansion()
	m.expansion++
	m.opening = false
	m.err = nil
	m.message = ""
	owner, request, ctx, user, view := m.owner, m.load.generation, m.load.ctx, m.userID, m.timelineView
	return tea.Batch(func() tea.Msg {
		if err := ctx.Err(); err != nil {
			return entriesListedMsg{owner: owner, request: request, err: err}
		}
		entries, err := view.ListEntriesInRange(ctx, user, day, day.AddDate(0, 0, 1))
		return entriesListedMsg{owner, request, entries, err}
	}, func() tea.Msg {
		if err := ctx.Err(); err != nil {
			return eventCountsMsg{owner: owner, request: request, err: err}
		}
		counts, err := view.ThoughtCounts(ctx, user, day, day.AddDate(0, 0, 1))
		return eventCountsMsg{owner, request, counts, err}
	}, feedback, m.loadSubjectTime(day))
}
func (m *Model) stopExpansion() {
	if m.cancelExpansion != nil {
		m.cancelExpansion()
	}
}

func (m *Model) openEntry(key timeline.EntryKey) tea.Cmd {
	if m.expanded != key {
		m.picker.Reset()
	}
	m.stopExpansion()
	m.expansion++
	m.expanded = key
	m.position.target = targetEntry
	m.opening = true
	m.expandedSubjectErr = nil
	m.position.followNow = false
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancelExpansion = cancel
	owner, request, user, view := m.owner, m.expansion, m.userID, m.timelineView
	subjects := m.subjectReader
	return func() tea.Msg {
		if ctx.Err() != nil {
			return nil
		}
		var scope timeline.ThoughtScope
		var err error
		if key.EventID != 0 {
			scope, err = view.OpenThoughtsView(ctx, user, key.EventID)
		} else {
			scope, err = view.OpenThoughtsOutsideEventsView(ctx, user, key.ObservedAt)
		}
		if ctx.Err() != nil {
			return nil
		}
		result := entryOpenedMsg{
			owner:   owner,
			request: request,
			scope:   scope,
			err:     err,
		}
		if err == nil && scope.Event().SubjectID != nil {
			result.subject, result.subjectErr = subjects.Get(ctx, user, *scope.Event().SubjectID)
		}
		if ctx.Err() != nil {
			return nil
		}
		return result
	}
}
func (m *Model) fail(operation logging.Operation, err error) {
	if category, emit := failure.Classify(operation, err); emit {
		m.logger.Failure(operation, category)
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.active || m.ctx.Err() != nil {
		return m, nil
	}
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "ctrl+c" {
		return m, nil
	}
	switch reply := msg.(type) {
	case subjectTimeMsg:
		m.acceptSubjectTime(reply)
		return m, nil
	case subjectTimeDelayedMsg:
		if reply.owner == m.owner && m.load.owns(reply.generation) && reply.revision == m.subjectTime.revision && m.subjectTime.pending {
			m.subjectTime.showStatus = true
		}
		return m, nil
	case subjectsLoaded:
		m.acceptSubjects(reply)
		return m, nil
	case tickMsg:
		if reply.owner != m.owner || reply.session != m.session {
			return m, nil
		}
		m.clock = reply.at
		m.load.retainedBody = ""
		var reload tea.Cmd
		if m.position.followNow && !m.day.Equal(displaytime.Day(m.clock)) && (!m.load.entriesPending || !m.load.pendingDay.Equal(displaytime.Day(m.clock))) {
			reload = m.loadDay(displaytime.Day(m.clock))
		}
		var subjectRead tea.Cmd
		if !m.load.entriesPending {
			subjectRead = m.loadSubjectTime(m.day)
		}
		m.anchor()
		return m, tea.Batch(reload, subjectRead, m.tick())
	case loadDelayedMsg:
		if reply.owner == m.owner {
			m.load.delayFeedback(reply.request)
		}
		return m, nil
	case entriesListedMsg:
		// The current model instance and newest day request exclusively own this reply.
		if reply.owner != m.owner || !m.load.owns(reply.request) || !m.load.entriesPending {
			return m, nil
		}
		m.load.finishEntries()
		m.err = reply.err
		m.fail(logging.TimelineList, reply.err)
		if reply.err != nil {
			m.message = "Could not load timeline for " + displaytime.Format(m.load.pendingDay, "January 2, 2006") + "."
			return m, nil
		}
		if !m.day.Equal(m.load.pendingDay) {
			m.day = m.load.pendingDay
			m.position.entryIndex, m.position.topLine, m.expanded = 0, 0, timeline.EntryKey{}
			m.position.target = targetEntry
			if m.position.followNow {
				m.position.target = targetBoundary
			}
			m.entries = nil
			m.picker.Reset()
		}
		if m.load.countsPending {
			// Only retain successful counts for unchanged displayed intervals.
			sameIntervals := slices.EqualFunc(m.entries, reply.entries, func(a, b timeline.Entry) bool {
				if a.Event == nil || b.Event == nil {
					return a.Key() == b.Key()
				}
				left, right := a.Event, b.Event
				sameEnd := left.EndedAt == nil && right.EndedAt == nil
				if left.EndedAt != nil && right.EndedAt != nil {
					sameEnd = left.EndedAt.Equal(*right.EndedAt)
				}
				return left.EventID == right.EventID && left.StartedAt.Equal(right.StartedAt) && sameEnd
			})
			if !sameIntervals || m.load.countErr != nil {
				m.eventCounts = nil
			}
			m.load.countErr = nil
		} else {
			m.eventCounts, m.load.countErr = m.load.pendingCounts.counts, m.load.pendingCounts.err
		}
		selected := timeline.EntryKey{}
		if len(m.entries) > 0 {
			selected = m.entries[m.position.entryIndex].Key()
		}
		m.entries = reply.entries
		m.applySubjectTime()
		m.position.clampSelection(len(m.entries))
		var latest, open tea.Cmd
		if m.position.followNow && len(m.entries) > 0 {
			m.position.entryIndex = len(m.entries) - 1
		}
		foundExpanded := false
		m.latest = nil
		m.load.latestErr = nil
		for i, entry := range m.entries {
			if !m.position.followNow && entry.Key() == selected {
				m.position.entryIndex = i
			}
			if entry.Key() == m.expanded {
				foundExpanded = true
				open = m.openEntry(entry.Key())
			}
			if entry.Event != nil && entry.Event.EndedAt == nil {
				if m.position.followNow {
					m.position.entryIndex = i
				}
				m.load.latestPending = true
				owner, request, ctx, user, view, id := m.owner, m.load.generation, m.load.ctx, m.userID, m.timelineView, entry.Event.EventID
				latest = func() tea.Msg {
					if err := ctx.Err(); err != nil {
						return latestEventThoughtMsg{owner: owner, request: request, err: err}
					}
					thought, err := view.LatestThought(ctx, user, id)
					return latestEventThoughtMsg{owner, request, thought, err}
				}
			}
		}
		if !foundExpanded {
			m.expanded = timeline.EntryKey{}
			m.picker.Reset()
		}
		m.anchor()
		if selected.IsZero() && !m.position.followNow && len(m.entries) > 0 {
			layout := m.measureTimeline()
			m.position.topLine = layout.positions[m.entries[0].Key()]
			m.position.clamp(len(layout.lines))
		}
		return m, tea.Batch(latest, open)
	case eventCountsMsg:
		// Counts share the day-request generation but may arrive before its entry list.
		if reply.owner != m.owner || !m.load.owns(reply.request) {
			return m, nil
		}
		m.load.finishCounts(reply)
		m.fail(logging.EventThoughtCount, reply.err)
		if m.load.entriesPending || !m.load.pendingDay.Equal(m.day) {
			return m, nil
		}
		m.eventCounts, m.load.countErr = reply.counts, reply.err
		m.anchor()
		return m, nil
	case latestEventThoughtMsg:
		// The latest preview inherits the accepted list's generation.
		if reply.owner != m.owner || !m.load.owns(reply.request) {
			return m, nil
		}
		m.load.latestPending = false
		m.latest, m.load.latestErr = reply.thought, reply.err
		m.fail(logging.EventThoughtList, reply.err)
		m.anchor()
		return m, nil
	case entryOpenedMsg:
		// Expansion has its own generation so changing/collapsing cards invalidates it.
		if reply.owner != m.owner || reply.request != m.expansion {
			return m, nil
		}
		m.opening = false
		m.expandedSubject, m.expandedSubjectErr = reply.subject, reply.subjectErr
		m.fail(logging.SubjectGet, reply.subjectErr)
		m.load.retainedBody = ""
		m.err = reply.err
		m.fail(logging.TimelineEntryOpen, reply.err)
		if reply.err != nil {
			m.message = "Could not open timeline entry."
			if errors.Is(diagnostics.SingleError(reply.err), data.ErrRecordNotFound) {
				m.message = "The requested timeline entry was not found."
			}
			return m, nil
		}
		if reply.scope.Key().EventID != 0 {
			event := reply.scope.Event()
			for i := range m.entries {
				if m.entries[i].Key() == reply.scope.Key() {
					m.entries[i].Event = &event
				}
			}
		}
		cmd := m.picker.OpenTimelineEntryView(reply.scope, m.timelineView)
		m.anchor()
		return m, cmd
	case eventSavedMsg:
		// Save ownership prevents an abandoned or replaced form from handling a reply.
		if reply.owner != m.owner || reply.request != m.save {
			return m, nil
		}
		m.form.saving = false
		m.form.err = reply.err
		op := logging.EventCreate
		if reply.ending {
			op = logging.EventEnd
		}
		m.fail(op, reply.err)
		if reply.err != nil {
			m.form.message = diagnostics.EventMessage(reply.err, "Could not save event.")
			cmd := m.focusForm()
			return m, cmd
		}
		mutation := logging.EventCreated
		if reply.ending {
			mutation = logging.EventEnded
		}
		m.logger.Mutation(mutation, reply.event.EventID)
		m.form.stopReads()
		m.form = eventForm{}
		day := m.day
		if !reply.ending {
			m.clock = m.now()
			day = displaytime.Day(m.clock)
			m.position.followNow = true
			m.position.target = targetBoundary
		}
		cmd := m.loadDay(day)
		return m, cmd
	case thoughts.Result, thoughts.BrowseThoughtsResult, thoughts.ThoughtBrowseStatsResult, thoughts.MindsetResult, thoughts.MindsetChanged:
		m.load.retainedBody = ""
		if change, ok := msg.(thoughts.MindsetChanged); ok && m.latest != nil && m.latest.ThoughtID == change.ThoughtID {
			m.latest.IsMindset = change.Active
		}
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
	if m.picker.SavingMindset() {
		return m, nil
	}
	if key.String() == "q" {
		key = tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
		msg = key
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
	if m.position.target == targetMetrics {
		m.load.retainedBody = ""
		switch key.String() {
		case "down", "j":
			m.subjectTime.offset++
			m.clampSubjectTime()
			return m, nil
		case "up", "k":
			if m.subjectTime.offset == 0 {
				m.position.target = targetBoundary
				m.position.followNow = m.day.Equal(displaytime.Day(m.clock))
			} else {
				m.subjectTime.offset--
				m.clampSubjectTime()
			}
			return m, nil
		case "pgdown":
			m.subjectTime.offset += max(1, m.bodyHeight()-8)
			m.clampSubjectTime()
			return m, nil
		case "pgup":
			m.subjectTime.offset -= max(1, m.bodyHeight()-8)
			m.clampSubjectTime()
			return m, nil
		case "esc":
			m.position.target = targetBoundary
			m.position.followNow = m.day.Equal(displaytime.Day(m.clock))
			return m, nil
		case "end", "home", "r":
		default:
			return m, nil
		}
	}
	if !m.expanded.IsZero() {
		if key.String() == "esc" || key.String() == "left" || key.String() == "h" {
			m.stopExpansion()
			m.expansion++
			m.expanded = timeline.EntryKey{}
			m.opening = false
			m.picker.Reset()
			m.revealSelected()
			return m, nil
		}
		if key.String() == "r" {
			cmd := m.openEntry(m.expanded)
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
	if m.load.entriesPending {
		day = m.load.pendingDay
	}
	navigation := key.String()
	if (navigation == "right" || navigation == "l") && day.Equal(displaytime.Day(m.now())) {
		if m.load.entriesPending && !day.Equal(m.day) {
			return m, nil
		}
		navigation = "enter"
	}
	// Do not act on the previous day's cards while navigation is pending.
	if m.load.entriesPending && !m.load.pendingDay.Equal(m.day) {
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
	if m.position.target == targetBoundary && len(m.entries) > 0 {
		m.position.entryIndex = len(m.entries) - 1
	}
	switch navigation {
	case "end":
		m.position.followNow = true
		m.clock = m.now()
		day := displaytime.Day(m.clock)
		var cmd tea.Cmd
		if !day.Equal(m.day) || m.load.entriesPending {
			cmd = m.loadDay(day)
		}
		m.position.target = targetBoundary
		m.position.entryIndex = max(0, len(m.entries)-1)
		m.anchor()
		return m, cmd
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
		if m.position.target == targetBoundary {
			if delta > 0 {
				m.position.target = targetMetrics
				m.position.followNow = false
				m.subjectTime.offset = 0
				return m, nil
			}
			if len(m.entries) == 0 {
				return m, nil
			}
			m.position.target = targetEntry
			m.position.entryIndex = len(m.entries) - 1
			m.position.followNow = false
			m.revealSelected()
			return m, nil
		}
		if delta > 0 && (len(m.entries) == 0 || m.position.entryIndex == len(m.entries)-1) {
			m.position.target = targetBoundary
			m.position.followNow = m.day.Equal(displaytime.Day(m.clock))
			m.anchor()
			return m, nil
		}
		m.position.moveEntry(delta, len(m.entries))
		m.revealSelected()
	case "pgup", "pgdown":
		if m.position.target == targetBoundary {
			m.position.topLine = max(0, m.measureTimeline().nowLine-m.bodyHeight()+2)
		}
		m.position.target = targetEntry
		direction := 1
		if key.String() == "pgup" {
			direction = -1
		}
		layout := m.measureTimeline()
		m.position.scrollPage(direction, m.height, len(layout.lines))
	case "enter":
		if len(m.entries) > 0 {
			cmd := m.openEntry(m.entries[m.position.entryIndex].Key())
			m.revealSelected()
			return m, cmd
		}
	case "esc":
		m.stopExpansion()
		m.expansion++
		m.expanded = timeline.EntryKey{}
		m.opening = false
		m.picker.Reset()
		layout := m.measureTimeline()
		m.position.clamp(len(layout.lines))
	case "n":
		cmd := m.startForm(false)
		return m, cmd
	case "e":
		if len(m.entries) > 0 && m.entries[m.position.entryIndex].Event != nil && m.entries[m.position.entryIndex].Event.EndedAt == nil {
			cmd := m.startForm(true)
			return m, cmd
		}
	}
	return m, nil
}
