package events

import (
	"context"
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/render"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
)

type subjectTimeMsg struct {
	owner                *int
	generation, revision uint64
	day                  time.Time
	items                []data.SubjectTimeView
	err                  error
}

func (subjectTimeMsg) eventMessage() {}

type subjectTimeDelayedMsg struct {
	owner                *int
	generation, revision uint64
}

func (subjectTimeDelayedMsg) eventMessage() {}

// Subject-time reads share the day owner's context, but have their own revision
// so a minute refresh cannot apply an older ongoing cutoff.
type subjectTimeState struct {
	cancel              context.CancelFunc
	cancelFeedback      context.CancelFunc
	revision            uint64
	pending, showStatus bool
	result              subjectTimeMsg
	itemsDay            time.Time
	items               []data.SubjectTimeView
	err                 error
	offset              int
}

func (s *subjectTimeState) stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.revision++
}

func (m *Model) loadSubjectTime(day time.Time) tea.Cmd {
	s := &m.subjectTime
	s.stop()
	ctx, cancel := context.WithCancel(m.load.ctx)
	feedbackCtx, cancelFeedback := context.WithCancel(ctx)
	s.cancel, s.cancelFeedback = cancel, cancelFeedback
	s.pending, s.showStatus = true, false
	owner, generation, revision := m.owner, m.load.generation, s.revision
	user, view, cutoff := m.userID, m.timelineView, m.clock
	return tea.Batch(func() tea.Msg {
		if ctx.Err() != nil {
			return nil
		}
		items, err := view.SubjectTimeInRange(ctx, user, day, day.AddDate(0, 0, 1), cutoff)
		if ctx.Err() != nil {
			return nil
		}
		return subjectTimeMsg{owner, generation, revision, day, items, err}
	}, func() tea.Msg {
		timer := time.NewTimer(150 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-feedbackCtx.Done():
			return nil
		case <-timer.C:
			return subjectTimeDelayedMsg{owner, generation, revision}
		}
	})
}

func (m *Model) applySubjectTime() {
	s := &m.subjectTime
	if !s.itemsDay.Equal(m.day) {
		s.items, s.err, s.offset = nil, nil, 0
		s.itemsDay = m.day
	}
	if s.pending || !s.result.day.Equal(m.day) {
		return
	}
	s.err = s.result.err
	if s.err != nil {
		return
	}
	var anchor int64
	if s.offset < len(s.items) {
		anchor = s.items[s.offset].SubjectID
	}
	s.items = s.result.items
	for i, item := range s.items {
		if item.SubjectID == anchor {
			s.offset = i
			break
		}
	}
	m.clampSubjectTime()
}

func (m *Model) acceptSubjectTime(reply subjectTimeMsg) {
	s := &m.subjectTime
	if reply.owner != m.owner || !m.load.owns(reply.generation) || reply.revision != s.revision || !s.pending {
		return
	}
	s.cancelFeedback()
	s.pending, s.showStatus = false, false
	s.result = reply
	m.fail(logging.SubjectTime, reply.err)
	m.applySubjectTime()
}

func (m *Model) clampSubjectTime() {
	capacity := max(1, m.bodyHeight()-6)
	last := 0
	if len(m.subjectTime.items) > capacity {
		last = len(m.subjectTime.items) - max(1, capacity-1)
	}
	m.subjectTime.offset = max(0, min(m.subjectTime.offset, last))
}

func (m *Model) boundaryLine() string {
	label := "..."
	if m.day.Equal(displaytime.Day(m.clock)) {
		label = "── Now · " + displaytime.Format(m.clock, "03:04 PM") + " ──"
	}
	return strings.Repeat(" ", m.cardColumn()) + label
}

func subjectTimeDuration(seconds int64) string {
	if seconds < 60 {
		return "<1m"
	}
	hours, minutes := seconds/3600, seconds%3600/60
	if hours == 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

func subjectTimeColor(id int64) color.Color {
	palette := [...]string{"#FF4B24", "#FFB000", "#B6FF00", "#00E5FF", "#B16CFF"}
	index := id % int64(len(palette))
	if index < 0 {
		index += int64(len(palette))
	}
	return lipgloss.Color(palette[index])
}

func (m *Model) subjectTimePanel(windowed bool) []string {
	s := &m.subjectTime
	heading := "Subject time"
	if m.day.Equal(displaytime.Day(m.clock)) {
		heading += " today"
	}
	content := []string{ansi.Truncate(heading, m.cardWidth(), "…")}
	var bars []render.Bar
	var barSubjects []int64
	start, end := 0, len(s.items)
	if windowed {
		m.clampSubjectTime()
		start = s.offset
		capacity := max(1, m.bodyHeight()-6)
		if start > 0 {
			content = append(content, "...")
			capacity--
		}
		if len(s.items)-start > capacity {
			capacity--
		}
		end = min(len(s.items), start+max(1, capacity))
	}
	if len(s.items) == 0 {
		label := "No subject time recorded."
		switch {
		case s.err != nil:
			label = "Subject time unavailable"
		case s.pending && s.showStatus:
			label = "Loading subject time…"
		case s.pending && !s.result.day.Equal(m.day):
			label = ""
		}
		content = append(content, ansi.Truncate(label, m.cardWidth(), "…"))
	}
	for i := start; i < end; i++ {
		item := s.items[i]
		spent := subjectTimeDuration(item.TimeSpentSeconds)
		badge := list.DefaultStyles(true).Title.Render(singleLine(item.SubjectName))
		badge = ansi.Truncate(badge, max(0, m.cardWidth()-ansi.StringWidth(spent)-1), "…")
		content = append(content, badge+strings.Repeat(" ", max(1, m.cardWidth()-ansi.StringWidth(badge)-ansi.StringWidth(spent)))+spent)
		bars = append(bars, render.Bar{Row: len(content), Value: item.TimeSpentSeconds})
		barSubjects = append(barSubjects, item.SubjectID)
	}
	if windowed && end < len(s.items) {
		content = append(content, "...")
	}
	panel := m.paintCard(content, m.position.target == targetMetrics)
	var cells [][]render.Cell
	if m.width >= distributionMinWidth && len(s.items) > 0 {
		_, exponent := m.distributions.boostPreset()
		cells = render.RenderBars(distributionWidth, len(panel), bars, render.BarOptions{
			Mode:      m.distributions.mode,
			Reference: s.items[0].TimeSpentSeconds,
			Exponent:  render.ThoughtLengthExponent(exponent),
			Size:      float64(m.distributions.size) / 100,
		})
	}
	for y := range panel {
		prefix := timelineLabelPadding
		if m.width >= distributionMinWidth {
			var lane strings.Builder
			for x := 0; x < distributionWidth; x++ {
				if y >= len(cells) || cells[y][x].Glyph == ' ' {
					lane.WriteByte(' ')
					continue
				}
				cell := cells[y][x]
				color := lipgloss.Color("240")
				if cell.CurveIndex >= 0 {
					color = subjectTimeColor(barSubjects[cell.CurveIndex])
				}
				lane.WriteString(lipgloss.NewStyle().Foreground(color).Render(string(cell.Glyph)))
			}
			prefix += distributionGutter + lane.String() + distributionGutter
		}
		panel[y] = prefix + panel[y]
	}
	return panel
}

func (m *Model) boundaryBody() string {
	height := m.bodyHeight()
	var contextRows []string
	if len(m.entries) > 0 {
		layout := m.measureTimeline()
		last := layout.cards[len(layout.cards)-1]
		end := last.top + len(last.content) + 2
		available := max(0, height-6)
		if len(last.content)+2 > available && available >= 4 {
			card := m.paintCard(last.content[:available-2], last.selected)
			for _, row := range card {
				contextRows = append(contextRows, strings.Repeat(" ", m.cardColumn())+row)
			}
		} else {
			contextRows = m.paintTimeline(layout, max(0, end-available), min(available, end))
		}
	}
	rows := make([]string, max(0, height-6-len(contextRows)))
	rows = append(rows, contextRows...)
	rows = append(rows, m.boundaryLine(), "", "")
	rows = append(rows, m.subjectTimePanel(false)[:3]...)
	return strings.Join(rows, "\n")
}

func (m *Model) subjectTimeBody() string {
	rows := []string{m.boundaryLine(), "", ""}
	rows = append(rows, m.subjectTimePanel(true)...)
	for len(rows) < m.bodyHeight() {
		rows = append(rows, "")
	}
	return strings.Join(rows[:min(len(rows), m.bodyHeight())], "\n")
}
