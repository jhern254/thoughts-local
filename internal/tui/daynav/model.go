// Package daynav owns the bounded day-activity overview above the Events rail.
package daynav

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/render"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
)

const (
	initialMonthSpan = -1 // Resolve once at the accepted date, then keep the window stable.
	maxPlotRows      = 4
	edgeBuckets      = 2 * render.DayActivityContextBuckets
	defaultPlotSize  = 100
	minPlotSize      = 50
	maxPlotSize      = 300
)

const (
	boostLow = iota
	boostNormal
	boostMedium
	boostStrong
	boostExtra
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
	bucketDays         int
	reference          float64
	err                error
}

type loadDelayedMsg struct {
	owner   *int
	request uint64
}

func Owns(msg tea.Msg) bool {
	switch msg.(type) {
	case loadedMsg, loadDelayedMsg:
		return true
	default:
		return false
	}
}

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
	showStatus                       bool
	width, height, rows, span        int
	markerAtRight                    bool // One-day windows share both dates; preserve the entry side.
	highlighted                      bool
	series                           [3]bool
	tuning                           bool
	boundsDirty                      bool
	refreshingHistory                bool
	calendarDay                      time.Time
	boost, size                      int
	outline                          bool
	values                           []render.DayActivity
	// Scale belongs to the historical snapshot and bucket resolution, not the
	// moving viewport. Only refresh/invalidation or a new resolution replaces it.
	reference                  float64
	bucketDays, firstBucketDay int
	plot                       string
	err                        error
}

func New(ctx context.Context, user string, reader Reader) Model {
	return Model{
		ctx:    ctx,
		user:   user,
		reader: reader,

		now:   time.Now,
		owner: new(int),

		dirty:  true,
		width:  80,
		height: 24,
		rows:   1,
		span:   initialMonthSpan,

		markerAtRight: true,
		series:        [3]bool{true, true, true},
		boost:         boostNormal,
		size:          defaultPlotSize,
	}
}
func (m *Model) Close() {
	// Retiring a read must not retire a still-unfulfilled history refresh.
	if m.refreshingHistory {
		m.boundsDirty = true
	}
	m.refreshingHistory = false
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.request++
	m.pending, m.showStatus = false, false
}
func (m Model) Focused() bool { return m.focused }
func (m Model) Tuning() bool  { return m.tuning }
func (m Model) Height() int   { return m.rows + 1 }
func (m *Model) Invalidate()  { m.dirty, m.boundsDirty = true, true }
func (m *Model) SetFocused(focused bool) {
	if m.focused == focused {
		return
	}
	m.focused = focused
	m.tuning = false
}

// SetHighlighted changes only the palette. Keyboard ownership stays with
// SetFocused; other home panels can keep the mountains colored too.
func (m *Model) SetHighlighted(highlighted bool) {
	if m.highlighted == highlighted {
		return
	}
	m.highlighted = highlighted
	m.rebuild()
}
func (m *Model) Resize(width, height int) {
	oldWidth, oldRows := m.width, m.rows
	m.width, m.height = max(1, width), height
	m.rows = m.plotRows()
	if m.width != oldWidth {
		m.dirty = true
	}
	if m.width != oldWidth || m.rows != oldRows || m.plot == "" {
		m.rebuild()
	}
}

func (m Model) plotRows() int {
	// The marker is separate. Reserve 6 header/footer rows, a Mindset row,
	// its separator, the marker, and 14 Events rows before growing mountains.
	return max(1, min(maxPlotRows, m.height-23))
}
func (m *Model) Sync(day time.Time, visible, refresh bool) tea.Cmd {
	selected := displaytime.Day(day)
	if !m.selected.IsZero() && !selected.Equal(m.selected) {
		m.markerAtRight = selected.After(m.selected)
	}
	m.selected = selected
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
	if span == initialMonthSpan {
		// Clamp the day number for shorter months instead of AddDate's overflow
		// (March 31 minus a month would otherwise normalize into March).
		anchor := minDate(selected, today)
		month := time.Date(anchor.Year(), anchor.Month(), 1, 0, 0, 0, 0, anchor.Location())
		last := month.AddDate(0, 0, -1)
		from := time.Date(last.Year(), last.Month(), min(anchor.Day(), last.Day()), 0, 0, 0, 0, anchor.Location())
		return from, anchor.AddDate(0, 0, 1)
	}
	earliest := first
	if earliest.IsZero() || selected.Before(earliest) {
		earliest = selected
	}
	end := today.AddDate(0, 0, 1)
	if span == 0 {
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
	if from.AddDate(0, 0, span).After(end) {
		from = end.AddDate(0, 0, -span)
	}
	return from, from.AddDate(0, 0, span)
}
func (m *Model) load(refreshBounds bool) tea.Cmd {
	// Zoom may replace an in-flight read, but must not discard a mutation's
	// request to include newly recorded history in the available range.
	refreshBounds = refreshBounds || m.boundsDirty || m.refreshingHistory
	m.Close()
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	m.pending = true
	m.refreshingHistory = refreshBounds
	m.dirty, m.boundsDirty = false, false
	owner, request, reader, user := m.owner, m.request, m.reader, m.user
	first, selected, oldFrom, oldUntil, span := m.first, m.selected, m.from, m.until, m.span
	// Each Braille cell has two horizontal dots. Read enough bounded samples
	// to use that detail instead of averaging it away before drawing.
	// One extra visible bucket may straddle the viewport's final boundary.
	columns := min(data.MaxActivityBuckets-edgeBuckets-1, m.width*2)
	reference, referenceDays := m.reference, m.bucketDays
	cutoff := m.now().Truncate(time.Second).Add(time.Second)
	today := displaytime.Day(cutoff.Add(-time.Second))
	return tea.Batch(func() tea.Msg {
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
		result.bucketDays = max(1, (days+columns-1)/columns)
		if refreshBounds || referenceDays != result.bucketDays {
			var err error
			reference, err = readHistoryReference(ctx, reader, user, first, today.AddDate(0, 0, 1), result.bucketDays, cutoff)
			if ctx.Err() != nil {
				return nil
			}
			if err != nil {
				result.err = err
				return result
			}
		}
		result.reference = reference
		from := alignedBucketStart(result.from, result.bucketDays)
		until := alignedBucketStart(result.until.AddDate(0, 0, -1), result.bucketDays).AddDate(0, 0, result.bucketDays)
		// Whole buckets stay anchored to civil dates across pans, including the
		// neighboring samples that continue each finite mountain shoulder.
		count := data.CalendarDays(from, until) / result.bucketDays
		result.boundaries = make([]time.Time, count+edgeBuckets+1)
		for i := range result.boundaries {
			result.boundaries[i] = from.AddDate(0, 0, (i-render.DayActivityContextBuckets)*result.bucketDays)
		}
		result.counts, result.err = reader.ActivityCounts(ctx, user, result.boundaries, cutoff)
		if ctx.Err() != nil {
			return nil
		}
		return result
	}, func() tea.Msg {
		// Match Events: delay only feedback, never the read or its publication.
		if ctx.Err() != nil {
			return nil
		}
		timer := time.NewTimer(150 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			return loadDelayedMsg{owner: owner, request: request}
		}
	})
}
func (m Model) Update(msg tea.Msg) Model {
	if delayed, ok := msg.(loadDelayedMsg); ok {
		if delayed.owner == m.owner && delayed.request == m.request && m.pending && m.visible {
			m.showStatus = true
		}
		return m
	}
	result, ok := msg.(loadedMsg)
	if !ok || result.owner != m.owner || result.request != m.request || !m.pending || !m.visible {
		return m
	}
	m.pending, m.showStatus = false, false
	m.refreshingHistory = false
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.err = result.err
	if result.err != nil {
		return m
	}
	if len(result.counts) < edgeBuckets+1 || len(result.counts)+1 != len(result.boundaries) {
		m.err = fmt.Errorf("invalid activity result")
		return m
	}
	m.first, m.from, m.until = result.first, result.from, result.until
	if m.span == initialMonthSpan {
		m.span = data.CalendarDays(m.from, m.until)
	}

	projected := make([]render.DayActivity, len(result.counts))
	for i, count := range result.counts {
		days := float64(data.CalendarDays(result.boundaries[i], result.boundaries[i+1]))
		projected[i] = render.DayActivity{Events: float64(count.EventDays) / days, Thoughts: float64(count.Thoughts) / days}
	}
	m.values = projected
	m.reference, m.bucketDays = result.reference, result.bucketDays
	m.firstBucketDay = calendarDayIndex(result.boundaries[0])
	m.rows = m.plotRows()
	m.rebuild()
	return m
}

func (m Model) boostPreset() (string, float64) {
	names := [...]string{"low", "normal", "medium", "strong", "extra"}
	// Stronger boost adds contrast: quiet days recede and summits stay sharp.
	exponents := [...]float64{.5, render.DayActivityDefaultExponent, 1.1, 1.8, 2.4}
	return names[m.boost], exponents[m.boost]
}

func (m *Model) Key(key string) tea.Cmd {
	if m.tuning {
		switch key {
		case "left", "h":
			m.boost = max(boostLow, m.boost-1)
		case "right", "l":
			m.boost = min(boostExtra, m.boost+1)
		case "up", "k":
			m.size = min(maxPlotSize, m.size+5)
		case "down", "j":
			m.size = max(minPlotSize, m.size-5)
		case "f":
			m.outline = !m.outline
		case "0":
			m.boost, m.size, m.outline = boostNormal, defaultPlotSize, false
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
		today := displaytime.Day(m.now())
		days := data.CalendarDays(m.from, m.until)
		if m.pending {
			// Repeated keys continue from the requested range, including a reset
			// whose month has not yet replaced the accepted viewport.
			from, until := window(m.first, m.selected, today, m.from, m.until, m.span)
			days = data.CalendarDays(from, until)
		}
		all := data.CalendarDays(minDate(m.first, m.selected), today.AddDate(0, 0, 1))
		switch key {
		case "+", "=":
			m.span = max(1, min(days-1, days*4/5))
		case "-":
			// The default month may include empty days. Zooming out must not
			// collapse it to a shorter history range.
			from, until := window(m.first, m.selected, today, m.from, m.until, initialMonthSpan)
			monthDays := data.CalendarDays(from, until)
			m.span = min(max(all, monthDays), max(days+1, (days*5+3)/4))
			if all >= monthDays && m.span >= all {
				m.span = 0
			}
		case "0":
			m.span = initialMonthSpan
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
	_, exponent := m.boostPreset()
	cells := render.RenderDayActivity(m.width, m.rows, m.values, render.DayActivityOptions{
		FirstDay:       calendarDayIndex(m.from),
		Days:           data.CalendarDays(m.from, m.until),
		SampleFirstDay: m.firstBucketDay,
		SampleDays:     m.bucketDays,
		Reference:      m.reference,
		Total:          visible[0],
		Events:         visible[1],
		Thoughts:       visible[2],
		Outline:        m.outline,
		Baseline:       true,
		Exponent:       exponent,
		// Size is relative to the normal mountain profile; the canvas stays fixed.
		Size: float64(m.size) / 100,
	})
	styles := [...]lipgloss.Style{lipgloss.NewStyle().Foreground(lipgloss.Color("240")), lipgloss.NewStyle().Foreground(lipgloss.Color("37")), lipgloss.NewStyle().Foreground(lipgloss.Color("179"))}
	if !m.highlighted {
		// Keep the unfocused graph neutral while retaining three visible bands.
		styles[render.DayThoughts] = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
		styles[render.DayEvents] = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))
	}
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
	// The accepted date lives in its own quiet row. Moving the block leaves
	// cached mountain geometry intact and uses the Events title highlight.
	marker := min(max(0, m.Marker()), max(0, m.width-1))
	block := lipgloss.NewStyle().Background(lipgloss.Color("62")).Render(" ")
	return m.plot + "\n" + strings.Repeat(" ", marker) + block + strings.Repeat(" ", m.width-marker-1)
}

// Status belongs beside the Events heading, with the existing footer as a
// narrow-window fallback. It never takes space away from the mountains.
func (m Model) Status() string {
	if m.tuning {
		name, _ := m.boostPreset()
		mode := "filled"
		if m.outline {
			mode = "outline"
		}
		return fmt.Sprintf("%s boost · %d%% · %s", name, m.size, mode)
	}
	if m.err != nil {
		return "Activity unavailable · r retry"
	}
	if m.pending && m.showStatus {
		return "Loading activity…"
	}
	return ""
}
func (m Model) Help() string {
	if m.tuning {
		return "←/→ boost · ↑/↓ size · f mode · 0 reset · Esc done"
	}
	if m.width < 60 {
		return "←/→ day · ↓ back · +/- zoom · 0 reset · d style"
	}
	return "←/→ day · +/- zoom · 0 reset zoom · d style · r refresh · ↓/Esc events"
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

// Marker pins edge dates to the border; interior dates share the curve grid.
func (m Model) Marker() int {
	if m.from.IsZero() {
		return 0
	}
	days := max(1, data.CalendarDays(m.from, m.until))
	if days == 1 {
		if m.markerAtRight {
			return m.width - 1
		}
		return 0
	}
	// A pending pan already clamps outside dates to the border. Keep the same
	// position after that date becomes the accepted window's first/last day.
	if !m.selected.After(m.from) {
		return 0
	}
	if !m.selected.Before(m.until.AddDate(0, 0, -1)) {
		return m.width - 1
	}
	return render.DayActivityDayColumn(m.width, days, calendarDayIndex(m.from), calendarDayIndex(m.selected))
}

func calendarDayIndex(day time.Time) int {
	return data.CalendarDays(time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC), day)
}
