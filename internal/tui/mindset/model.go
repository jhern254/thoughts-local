// Package mindset owns the historical mindset panel above the home timeline.
package mindset

import (
	"context"
	"image/color"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/failure"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
)

const rotationInterval = 15 * time.Second

type Reader interface {
	ListMindsetsInRange(context.Context, string, time.Time, time.Time) ([]data.ThoughtSummaryView, error)
}

type OpenThoughtMsg struct {
	Thought data.ThoughtSummaryView
	owner   *int
	request uint64
}
type loadedMsg struct {
	owner   *int
	request uint64
	items   []data.ThoughtSummaryView
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
	items                     []data.ThoughtSummaryView
	index, offset             int
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
		rows:   5,
	}
}
func (m Model) AcceptsOpen(msg OpenThoughtMsg) bool {
	return m.focused && !m.loading && len(m.items) > 0 && msg.owner == m.owner && msg.request == m.request && msg.Thought.ThoughtID == m.items[m.index].ThoughtID
}

func (m Model) Day() time.Time { return m.day }
func (m Model) Focused() bool  { return m.focused }
func (m Model) Expanded() bool { return m.focused }
func (m Model) Height() int {
	if !m.focused || len(m.items) == 0 || m.err != nil || m.loading {
		return 1
	}
	return min(m.rows, len(m.items))
}
func (m *Model) Resize(width, rows int) {
	m.width = max(1, width)
	m.rows = max(1, min(5, rows))
	m.keepVisible()
}

func (m *Model) Load(day time.Time) tea.Cmd {
	day = displaytime.Day(day)
	if m.cancelRead != nil {
		m.cancelRead()
	}
	m.stopTimer()
	if !m.day.Equal(day) {
		m.items = nil
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
		m.items = msg.items
		m.index = min(m.index, max(0, len(m.items)-1))
		for i, item := range m.items {
			if item.ThoughtID == selected {
				m.index = i
				break
			}
		}
		m.keepVisible()
		return m, m.startTimer()
	case tickMsg:
		if msg.owner != m.owner || msg.generation != m.generation || !m.visible || m.focused || len(m.items) < 2 {
			return m, nil
		}
		m.index = (m.index + 1) % len(m.items)
		return m, m.startTimer()
	case tea.KeyPressMsg:
		if !m.focused {
			return m, nil
		}
		switch msg.String() {
		case "r":
			return m, m.Load(m.day)
		case "left", "q":
			return m, nil
		case "enter", "right":
			if m.loading || m.err != nil || len(m.items) == 0 {
				return m, nil
			}
			item := m.items[m.index]
			return m, func() tea.Msg { return OpenThoughtMsg{Thought: item, owner: m.owner, request: m.request} }
		}
		if m.loading || len(m.items) == 0 {
			return m, nil
		}
		step := 0
		switch msg.String() {
		case "up", "k":
			step = -1
		case "down", "j":
			step = 1
		case "pgup":
			step = -m.rows
		case "pgdown":
			step = m.rows
		case "home":
			m.index = 0
		case "end":
			m.index = len(m.items) - 1
		}
		m.index = (m.index + step) % len(m.items)
		if m.index < 0 {
			m.index += len(m.items)
		}
		m.keepVisible()
	}
	return m, nil
}
func (m *Model) keepVisible() {
	if m.index < m.offset {
		m.offset = m.index
	}
	if m.index >= m.offset+m.rows {
		m.offset = m.index - m.rows + 1
	}
	m.offset = max(0, min(m.offset, len(m.items)-m.rows))
}
func (m Model) View() string {
	heading := lipgloss.NewStyle().Background(color.White).Foreground(color.Black).Padding(0, 1).Render("Mindset")
	styles := list.NewDefaultItemStyles(true)
	status := ""
	switch {
	case m.loading:
		status = "Loading mindsets…"
	case m.err != nil:
		status = "Could not load mindsets. r: retry"
	case len(m.items) == 0:
		if m.focused {
			return ansi.Truncate(heading+styles.SelectedTitle.PaddingLeft(0).Render("..."), m.width, "")
		}
		status = "..."
	case !m.focused:
		status = previewText(m.items[m.index].Preview)
	}
	if status != "" || !m.focused {
		return ansi.Truncate(heading+" "+status, m.width, "…")
	}
	last := min(len(m.items), m.offset+m.rows)
	rows := make([]string, 0, last-m.offset)
	indent := strings.Repeat(" ", ansi.StringWidth(heading))
	pickerWidth := max(0, m.width-ansi.StringWidth(heading))
	for i := m.offset; i < last; i++ {
		style := styles.NormalTitle.PaddingLeft(1)
		if m.focused && i == m.index {
			style = styles.SelectedTitle.PaddingLeft(0)
		}
		text := previewText(m.items[i].Preview)
		prefix := indent
		if i == m.offset {
			prefix = heading
		}
		item := style.Render(ansi.Truncate(text, max(0, pickerWidth-style.GetHorizontalFrameSize()), "…"))
		rows = append(rows, ansi.Truncate(prefix+item, m.width, ""))
	}
	return strings.Join(rows, "\n")
}

func previewText(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
}
