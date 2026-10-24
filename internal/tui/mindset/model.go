// Package mindset owns the historical mindset panel above the home timeline.
package mindset

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/failure"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
)

const rotationInterval = 15 * time.Second

type Reader interface {
	ListMindsetsInRange(context.Context, string, time.Time, time.Time) ([]data.MindsetThought, error)
}

type OpenThoughtMsg struct {
	Thought data.MindsetThought
	owner   *int
	request uint64
}
type loadedMsg struct {
	owner   *int
	request uint64
	items   []data.MindsetThought
	err     error
}
type tickMsg struct {
	owner      *int
	generation uint64
}

func Owns(msg tea.Msg) bool {
	switch msg.(type) {
	case loadedMsg, tickMsg:
		return true
	}
	return false
}

type Model struct {
	ctx                       context.Context
	userID                    string
	reader                    Reader
	logger                    logging.Logger
	owner                     *int
	cancelRead, cancelTimer   context.CancelFunc
	request, generation       uint64
	day                       time.Time
	items                     []data.MindsetThought
	index, offset             int
	layout                    panelLayout
	width, rows               int
	visible, focused, loading bool
	err                       error
}

func New(ctx context.Context, userID string, reader Reader, logger logging.Logger) Model {
	return Model{
		ctx:    ctx,
		userID: userID,
		reader: reader,
		logger: logger,
		owner:  new(int),
		width:  80,
		rows:   12,
	}
}
func (m Model) AcceptsOpen(msg OpenThoughtMsg) bool {
	return m.focused && !m.loading && len(m.items) > 0 && msg.owner == m.owner && msg.request == m.request && msg.Thought.ThoughtID == m.items[m.index].ThoughtID
}

func (m Model) Day() time.Time { return m.day }
func (m Model) Focused() bool  { return m.focused }
func (m Model) Expanded() bool { return m.focused }
func (m Model) Height() int {
	if len(m.layout.lines) == 0 || m.err != nil || m.loading {
		return 1
	}
	if !m.focused {
		return min(m.rows, m.layout.ends[m.index]-m.layout.starts[m.index])
	}
	return min(m.rows, len(m.layout.lines))
}
func (m *Model) Resize(width, rows int) {
	width, rows = max(1, width), max(1, min(12, rows))
	if width == m.width && rows == m.rows {
		return
	}
	anchor := 0
	if len(m.layout.lines) > 0 {
		line := max(m.layout.starts[m.index], m.offset)
		anchor = m.layout.lines[line].source
	}
	m.width, m.rows = width, rows
	m.reflow(anchor)
}

func (m *Model) Load(day time.Time) tea.Cmd {
	day = displaytime.Day(day)
	if m.cancelRead != nil {
		m.cancelRead()
	}
	m.stopTimer()
	if !m.day.Equal(day) {
		m.items = nil
		m.layout = panelLayout{}
		m.index = 0
		m.offset = 0
	}
	m.day = day
	m.loading = true
	m.err = nil
	m.request++
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancelRead = cancel
	owner, request, reader, user := m.owner, m.request, m.reader, m.userID
	until := day.AddDate(0, 0, 1)
	return func() tea.Msg {
		if ctx.Err() != nil {
			return nil
		}
		items, err := reader.ListMindsetsInRange(ctx, user, day, until)
		if ctx.Err() != nil {
			return nil
		}
		return loadedMsg{owner: owner, request: request, items: items, err: err}
	}
}
func (m *Model) stopTimer() {
	if m.cancelTimer != nil {
		m.cancelTimer()
		m.cancelTimer = nil
	}
	m.generation++
}
func (m *Model) startTimer() tea.Cmd {
	m.stopTimer()
	if !m.visible || m.focused || m.loading || m.err != nil || len(m.items) < 2 {
		return nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancelTimer = cancel
	owner, generation := m.owner, m.generation
	return func() tea.Msg {
		timer := time.NewTimer(rotationInterval)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			return tickMsg{owner: owner, generation: generation}
		}
	}
}
func (m Model) SetVisible(visible bool) (Model, tea.Cmd) {
	if m.visible == visible {
		return m, nil
	}
	m.visible = visible
	return m, m.startTimer()
}
func (m Model) SetFocused(focused bool) (Model, tea.Cmd) {
	if m.focused == focused {
		return m, nil
	}
	m.focused = focused
	m.keepVisible()
	return m, m.startTimer()
}
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if m.ctx.Err() != nil {
		return m, nil
	}
	switch msg := msg.(type) {
	case loadedMsg:
		if msg.owner != m.owner || msg.request != m.request {
			return m, nil
		}
		m.loading = false
		m.err = msg.err
		if msg.err != nil {
			if category, emit := failure.Classify(logging.ThoughtMindsetList, msg.err); emit {
				m.logger.Failure(logging.ThoughtMindsetList, category)
			}
			return m, nil
		}
		selected := int64(0)
		if len(m.items) > 0 {
			selected = m.items[m.index].ThoughtID
		}
		anchor := 0
		if len(m.layout.lines) > 0 {
			anchor = m.layout.lines[max(m.layout.starts[m.index], m.offset)].source
		}
		m.items = msg.items
		m.index = min(m.index, max(0, len(m.items)-1))
		retained := false
		for i, item := range m.items {
			if item.ThoughtID == selected {
				m.index = i
				retained = true
				break
			}
		}
		if !retained {
			anchor = 0
		}
		m.reflow(anchor)
		return m, m.startTimer()
	case tickMsg:
		if msg.owner != m.owner || msg.generation != m.generation || !m.visible || m.focused || len(m.items) < 2 {
			return m, nil
		}
		m.index = (m.index + 1) % len(m.items)
		m.offset = m.layout.starts[m.index]
		return m, m.startTimer()
	case tea.KeyPressMsg:
		if !m.focused {
			return m, nil
		}
		switch msg.String() {
		case "r":
			return m, m.Load(m.day)
		case "left", "right", "q":
			return m, nil
		case "enter":
			if m.loading || m.err != nil || len(m.items) == 0 {
				return m, nil
			}
			item := m.items[m.index]
			return m, func() tea.Msg { return OpenThoughtMsg{Thought: item, owner: m.owner, request: m.request} }
		}
		if m.loading || m.err != nil || len(m.items) == 0 {
			return m, nil
		}

		switch msg.String() {
		case "up", "k":
			m.moveReading(-1)
		case "down", "j":
			m.moveReading(1)
		case "pgup", "pgdown":
			direction := 1
			if msg.String() == "pgup" {
				direction = -1
			}
			for range m.rows {
				m.moveReading(direction)
			}
		case "home":
			m.index, m.offset = 0, 0
		case "end":
			m.index = len(m.items) - 1
			m.offset = max(0, len(m.layout.lines)-m.rows)
		}
	}
	return m, nil
}

func (m *Model) moveReading(direction int) {
	start, end := m.layout.starts[m.index], m.layout.ends[m.index]
	if direction > 0 && end > m.offset+m.rows {
		m.offset++
		return
	}
	if direction < 0 && start < m.offset {
		m.offset--
		return
	}
	m.index = (m.index + direction + len(m.items)) % len(m.items)
	if m.layout.ends[m.index]-m.layout.starts[m.index] > m.rows {
		m.offset = m.layout.starts[m.index]
		if direction < 0 {
			m.offset = m.layout.ends[m.index] - m.rows
		}
	}
	m.keepVisible()
}

func (m *Model) keepVisible() {
	if len(m.layout.lines) == 0 {
		m.offset = 0
		return
	}
	start, end := m.layout.starts[m.index], m.layout.ends[m.index]
	if end-start > m.rows {
		m.offset = max(start, min(m.offset, end-m.rows))
	} else {
		if start < m.offset {
			m.offset = start
		}
		if end > m.offset+m.rows {
			m.offset = end - m.rows
		}
	}
	m.offset = max(0, min(m.offset, len(m.layout.lines)-m.rows))
}
