// Package daynav owns the bounded day-activity overview above the Events rail.
package daynav

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/render"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
)

const (
	defaultPlotRows = 5
	maxPlotRows     = 12
	minPlotSize     = 50
	maxPlotSize     = 300
)

type Reader interface {
	ActivityStart(context.Context, string, time.Time) (time.Time, error)
	ActivityCounts(context.Context, string, []time.Time, time.Time) ([]data.ActivityCount, error)
}
type loadedMsg struct {
	owner              *int
	request            uint64
	first, from, until time.Time
	boundaries         []time.Time
	counts             []data.ActivityCount
	err                error
}

func Owns(msg tea.Msg) bool { _, ok := msg.(loadedMsg); return ok }

type Model struct {
	ctx                          context.Context
	user                         string
	reader                       Reader
	now                          func() time.Time
	owner                        *int
	cancel                       context.CancelFunc
	request                      uint64
	first, selected, from, until time.Time
	// The accepted range stays visible during reads. Invalidation during a read
	// queues one refresh; request ownership rejects responses after Close/zoom.
	pending, visible, focused, dirty bool
	width, height, rows, span        int
	series                           [3]bool
	tuning                           bool
	compact                          bool
	boundsDirty                      bool
	calendarDay                      time.Time
	boost, size                      int
	outline                          bool
	values                           []render.DayActivity
	plot                             string
	err                              error
}

func New(ctx context.Context, user string, reader Reader) Model {
	return Model{
		ctx:    ctx,
		user:   user,
		reader: reader,
		now:    time.Now,
		owner:  new(int),
		dirty:  true,
		width:  80,
		height: 24,
		rows:   1,
		series: [3]bool{true, true, true},
		boost:  1,
		size:   100,
	}
}
func (m *Model) Close() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.request++
	m.pending = false
}
func (m Model) Focused() bool { return m.focused }
func (m Model) Tuning() bool  { return m.tuning }
func (m Model) Height() int {
	if m.compact {
		return max(1, m.rows)
	}
	return m.rows + 1
}
func (m *Model) Invalidate() { m.dirty, m.boundsDirty = true, true }
func (m *Model) SetFocused(focused bool) {
	if m.focused == focused {
		return
	}
	m.focused = focused
	m.tuning = false
	m.rebuild()
}
func (m *Model) Resize(width, height int) {
	oldWidth, oldRows := m.width, m.rows
	m.width, m.height = max(1, width), height
	m.compact = height < 26
	m.rows = m.plotRows()
	if m.width != oldWidth {
		m.dirty = true
	}
	if m.width != oldWidth || m.rows != oldRows {
		m.rebuild()
	}
}

func (m Model) plotRows() int {
	if m.height < 16 {
		return 0
	}
	base := defaultPlotRows
	switch {
	case m.height < 26:
		base = 1
	case m.height < 28:
		base = 2
	case m.height < 36:
		base = 4
	}
	// Reserve the existing header/footer, Mindset's minimum row, separators,
	// and 14 usable Events rows. Twelve plot rows bound even dramatic sizing.
	reserved := 6 + 1 + 2 + 14
	if !m.compact {
		reserved++
	} // The quiet row also hosts controls/read status.
	available := max(1, min(maxPlotRows, m.height-reserved))
	return min(available, max(1, (base*m.size+50)/100))
}
func (m *Model) Sync(day time.Time, visible, refresh bool) tea.Cmd {
	m.selected = displaytime.Day(day)
	if !visible {
		if m.visible {
			if m.pending {
				m.dirty = true
			}
			m.Close()
		}
		m.visible = false
		return nil
	}
	m.visible = true
	if refresh {
		m.Invalidate()
	}
	today := displaytime.Day(m.now())
	if !today.Equal(m.calendarDay) {
		m.calendarDay = today
		m.Invalidate()
	}
	outside := m.from.IsZero() || m.selected.Before(m.from) || !m.selected.Before(m.until)
	if m.pending {
		return nil
	}
	if m.dirty || (outside && m.err == nil) {
		return m.load(m.boundsDirty)
	}
	return nil
}
func window(first, selected, today, previousFrom, previousUntil time.Time, span int) (time.Time, time.Time) {
	earliest := first
	if earliest.IsZero() || selected.Before(earliest) {
		earliest = selected
	}
	end := today.AddDate(0, 0, 1)
	if span == 0 || span >= data.CalendarDays(earliest, end) {
		return earliest, end
	}
	if !previousFrom.IsZero() && data.CalendarDays(previousFrom, previousUntil) == span {
		if selected.Before(previousFrom) {
			return selected, selected.AddDate(0, 0, span)
		}
		if !selected.Before(previousUntil) {
			return selected.AddDate(0, 0, 1-span), selected.AddDate(0, 0, 1)
		}
		return previousFrom, previousUntil
	}
	from := selected.AddDate(0, 0, -span/2)
	if from.Before(earliest) {
		from = earliest
	}
	if from.AddDate(0, 0, span).After(end) {
		from = end.AddDate(0, 0, -span)
	}
	return from, from.AddDate(0, 0, span)
}
func (m *Model) load(refreshBounds bool) tea.Cmd {
	// Zoom may replace an in-flight read, but must not discard a mutation's
	// request to include newly recorded history in the available range.
	refreshBounds = refreshBounds || m.boundsDirty
	m.Close()
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	m.pending = true
	m.dirty, m.boundsDirty = false, false
	owner, request, reader, user := m.owner, m.request, m.reader, m.user
	first, selected, oldFrom, oldUntil, span := m.first, m.selected, m.from, m.until, m.span
	// Each Braille cell has two horizontal dots. Read enough bounded samples
	// to use that detail instead of averaging it away before drawing.
	columns := min(data.MaxActivityBuckets, m.width*2)
	cutoff := m.now().Truncate(time.Second).Add(time.Second)
	today := displaytime.Day(cutoff.Add(-time.Second))
	return func() tea.Msg {
		result := loadedMsg{owner: owner, request: request}
		if first.IsZero() || refreshBounds {
			start, err := reader.ActivityStart(ctx, user, cutoff)
			if err != nil {
				result.err = err
				return result
			}
			first = today
			if !start.IsZero() {
				first = displaytime.Day(start)
			}
		}
		result.first = first
		result.from, result.until = window(first, selected, today, oldFrom, oldUntil, span)
		days := data.CalendarDays(result.from, result.until)
		count := min(columns, max(1, days))
		result.boundaries = make([]time.Time, count+1)
		for i := range result.boundaries {
			result.boundaries[i] = result.from.AddDate(0, 0, days*i/count)
		}
		result.counts, result.err = reader.ActivityCounts(ctx, user, result.boundaries, cutoff)
		if ctx.Err() != nil {
			return nil
		}
		return result
	}
}
func (m Model) Update(msg tea.Msg) Model {
	result, ok := msg.(loadedMsg)
	if !ok || result.owner != m.owner || result.request != m.request || !m.pending || !m.visible {
		return m
	}
	m.pending = false
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.err = result.err
	if result.err != nil {
		return m
	}
	if len(result.counts)+1 != len(result.boundaries) {
		m.err = fmt.Errorf("invalid activity result")
		return m
	}
	m.first, m.from, m.until = result.first, result.from, result.until
	m.values = make([]render.DayActivity, len(result.counts))
	for i, count := range result.counts {
		days := float64(data.CalendarDays(result.boundaries[i], result.boundaries[i+1]))
		m.values[i] = render.DayActivity{Events: float64(count.EventDays) / days, Thoughts: float64(count.Thoughts) / days}
	}
	m.rebuild()
	return m
}

func (m Model) boostPreset() (string, float64) {
	names := [...]string{"low", "normal", "medium", "strong", "extra"}
	// Stronger boost adds contrast: quiet days recede and summits stay sharp.
	exponents := [...]float64{.8, render.DayActivityDefaultExponent, 1.4, 1.8, 2.4}
	return names[m.boost], exponents[m.boost]
}

func (m *Model) Key(key string) tea.Cmd {
	if m.tuning {
		switch key {
		case "left", "h":
			m.boost = max(0, m.boost-1)
		case "right", "l":
			m.boost = min(4, m.boost+1)
		case "up", "k":
			m.size = min(maxPlotSize, m.size+5)
		case "down", "j":
			m.size = max(minPlotSize, m.size-5)
		case "f":
			m.outline = !m.outline
		case "0":
			m.boost, m.size, m.outline = 1, 100, false
		case "q", "esc", "enter", "d":
			m.tuning = false
		}
		m.rows = m.plotRows()
		m.rebuild()
		return nil
	}
	switch key {
	case "1", "2", "3":
		i := int(key[0] - '1')
		m.series[i] = !m.series[i]
		m.rebuild()
	case "d":
		m.tuning = true
	case "+", "=", "-", "0":
		if m.first.IsZero() {
			return nil
		}
		days := data.CalendarDays(m.from, m.until)
		if m.pending && m.span > 0 {
			days = m.span
		}
		all := data.CalendarDays(minDate(m.first, m.selected), displaytime.Day(m.now()).AddDate(0, 0, 1))
		switch key {
		case "+", "=":
			m.span = max(1, min(days-1, days*4/5))
		case "-":
			m.span = max(days+1, (days*5+3)/4)
			if m.span >= all {
				m.span = 0
			}
		case "0":
			m.span = 0
		}
		return m.load(false)
	case "r":
		return m.load(true)
	}
	return nil
}
func minDate(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
func (m *Model) rebuild() {
	visible := m.series
	if !m.focused {
		visible = [3]bool{true, false, false}
	}
	_, exponent := m.boostPreset()
	cells := render.RenderDayActivity(m.width, m.rows, m.values, render.DayActivityOptions{
		Total:    visible[0],
		Events:   visible[1],
		Thoughts: visible[2],
		Outline:  m.outline,
		Exponent: exponent,
		Size:     float64(m.size) / 100,
	})
	styles := [...]lipgloss.Style{lipgloss.NewStyle().Foreground(lipgloss.Color("240")), lipgloss.NewStyle().Foreground(lipgloss.Color("37")), lipgloss.NewStyle().Foreground(lipgloss.Color("179"))}
	var out strings.Builder
	for y, row := range cells {
		if y > 0 {
			out.WriteByte('\n')
		}
		for start := 0; start < len(row); {
			end := start + 1
			for end < len(row) && row[end].Series == row[start].Series {
				end++
			}
			glyphs := make([]rune, end-start)
			for i := range glyphs {
				glyphs[i] = row[start+i].Glyph
			}
			out.WriteString(styles[row[start].Series].Render(string(glyphs)))
			start = end
		}
	}
	m.plot = out.String()
}
func (m Model) View() string {
	// Keep space above the plot without shifting the timeline. Only explicit
	// controls and exceptional status use this row.
	line := ""
	if m.err != nil {
		line = "Activity unavailable · r retry"
	} else if m.pending {
		line = "Loading activity…"
	}
	if m.tuning {
		name, _ := m.boostPreset()
		mode := "filled"
		if m.outline {
			mode = "outline"
		}
		line = fmt.Sprintf("%s boost · %d%% · %s", name, m.size, mode)
	}
	line = ansi.Truncate(line, m.width, "…")
	if m.rows > 0 {
		if m.compact && !m.tuning && m.err == nil {
			return m.plot
		}
		if m.compact {
			return line
		}
		return line + "\n" + m.plot
	}
	return line
}
func (m Model) Help() string {
	if m.tuning {
		return "←/→ boost · ↑/↓ size · f mode · 0 reset · Esc done"
	}
	if m.width < 60 {
		return "←/→ day · ↓ back · +/- zoom · d style"
	}
	return "←/→ day · +/- zoom · 0 all · d style · r refresh · ↓/Esc events"
}
func (m Model) Legend() string {
	names := [...]string{"1 Total", "2 Events", "3 Thoughts"}
	colors := [...]string{"240", "37", "179"}
	for i := range names {
		if !m.series[i] {
			names[i] += " off"
		}
		names[i] = lipgloss.NewStyle().Foreground(lipgloss.Color(colors[i])).Render(names[i])
	}
	return strings.Join(names[:], " · ")
}

// Marker is the accepted date's column, independent of coarse bucket resolution.
func (m Model) Marker() int {
	if m.from.IsZero() {
		return 0
	}
	days := max(1, data.CalendarDays(m.from, m.until))
	return min(m.width-1, max(0, (2*data.CalendarDays(m.from, m.selected)+1)*m.width/(2*days)))
}
