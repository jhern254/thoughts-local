package events

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/render"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
)

func duration(start, end time.Time, ongoing bool) string {
	minutes := int64(max(0, end.Sub(start)) / time.Minute)
	if minutes == 0 {
		if ongoing {
			return "<1m"
		}
		return "0m"
	}
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	if minutes < 1440 {
		if minutes%60 == 0 {
			return fmt.Sprintf("%dh", minutes/60)
		}
		return fmt.Sprintf("%dh %dm", minutes/60, minutes%60)
	}
	return fmt.Sprintf("%dd %dh", minutes/1440, (minutes%1440)/60)
}
func singleLine(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, value)
}
func (m *Model) count(id int64) string {
	pending := m.load.countsPending && !m.load.eventsPending && m.load.pendingDay.Equal(m.day)
	if m.load.countErr != nil {
		return "Thought count unavailable"
	}
	for _, item := range m.counts {
		if item.EventID == id {
			label := "1 thought"
			if item.Count != 1 {
				label = fmt.Sprintf("%d thoughts", item.Count)
			}
			if pending && m.load.showStatus {
				label += " · refreshing…"
			}
			return label
		}
	}
	if pending {
		if m.load.showStatus {
			return "Counting thoughts…"
		}
		return "" // Reserve the count row without inventing a value or flashing text.
	}
	return "Thought count unavailable"
}
func (m *Model) cardContent(item data.Event) []string {
	label := "Event"
	if item.ActivityType != nil {
		label = singleLine(*item.ActivityType)
	}
	end := m.clock
	if item.EndedAt != nil {
		end = *item.EndedAt
	}
	heading := list.DefaultStyles(true).Title.Render(label) + " · " + duration(item.StartedAt, end, item.EndedAt == nil)
	startLayout, endLayout := "03:04", "03:04 PM"
	crossDay := !displaytime.Day(item.StartedAt).Equal(displaytime.Day(end))
	differentZone := displaytime.Format(item.StartedAt, "MST") != displaytime.Format(end, "MST")
	if crossDay || displaytime.Format(item.StartedAt, "PM") != displaytime.Format(end, "PM") || (item.EventID == m.expanded && differentZone) {
		startLayout = "03:04 PM"
	}
	if crossDay {
		startLayout, endLayout = "Jan 2 "+startLayout, "Jan 2 "+endLayout
	}
	if item.EventID == m.expanded {
		endLayout += " MST"
		if differentZone {
			startLayout += " MST"
		}
	}
	interval := ""
	if item.EndedAt == nil {
		interval = "Started at " + displaytime.Format(item.StartedAt, endLayout)
	} else {
		interval = displaytime.Format(item.StartedAt, startLayout) + " - " + displaytime.Format(end, endLayout)
	}
	if item.StartedAt.Before(m.day) {
		interval = "← " + interval
	}
	if end.After(m.day.AddDate(0, 0, 1)) {
		interval += " →"
	}
	width := m.cardWidth()
	heading += " · " + interval
	if item.EndedAt == nil {
		heading += " · ongoing"
	}
	content := ansi.Truncate(heading, width, "…") + "\n"
	if item.EventID == m.expanded {
		content += "\n"
		if m.opening {
			content += "Loading thoughts…"
		} else if m.err != nil {
			content += m.message
		} else {
			content += m.picker.View()
		}
	} else {
		content += m.count(item.EventID)
		if item.EndedAt == nil {
			content += "\n"
			switch {
			case m.load.latestPending:
				content += "Loading latest thought…"
			case m.load.latestErr != nil:
				content += "Latest thought unavailable"
			case m.latest != nil:
				content += m.picker.TimelinePreview(*m.latest, width)
			default:
				content += "No thoughts yet"
			}
		}
	}
	// Lip Gloss Width includes the border. Clip each logical row before
	// wrapping so narrow cards cannot split a two-line thought preview.
	lines := strings.Split(content, "\n")
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "…")
	}
	return lines
}

func (m *Model) paintCard(content []string, selected bool) []string {
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Width(m.cardWidth() + 2)
	if selected && !m.blurred {
		style = style.BorderForeground(lipgloss.Color("62"))
	}
	return strings.Split(style.Render(strings.Join(content, "\n")), "\n")
}

// Clip only the calendar geometry, not the event's actual timestamps or count.
func (m *Model) interval(item data.Event) (time.Time, time.Time) {
	start := item.StartedAt
	if start.Before(m.day) {
		start = m.day
	}
	end := m.clock
	if item.EndedAt != nil {
		end = *item.EndedAt
	}
	if boundary := m.day.AddDate(0, 0, 1); end.After(boundary) {
		end = boundary
	}
	if end.Before(start) {
		end = start
	}
	return start, end
}

func (m *Model) hourHeight() int { return max(4, (m.bodyHeight()+4)/5) }

type railEntry struct {
	at       time.Time
	priority int
	id       int64
	text     string
	content  []string
	selected bool
	now      bool
	end      time.Time
	hideEnd  bool
}

// timelineLayout is a per-call measurement, never retained on Model. Card rows
// are clipped before measuring so borders cannot wrap them into additional rows.
type timelineLayout struct {
	lines             []string // Rail and boundary labels; card interiors are painted later.
	cards             []timelineCard
	positions         map[int64]int
	curves            []render.Distribution
	selectedCurve     int
	nowLine, curveEnd int
}

type timelineCard struct {
	top      int
	content  []string
	selected bool
}

// measureTimeline preserves the chronological rail without painting card borders
// or distributions. Positioning and viewport painting consume the same row count.
func (m *Model) measureTimeline() timelineLayout {
	entries := []railEntry{}
	until := m.day.AddDate(0, 0, 1)
	for hour := m.day; hour.Before(until) && !hour.After(m.clock); hour = hour.Add(time.Hour) {
		entries = append(entries, railEntry{at: hour, text: displaytime.Format(hour, "03:04 PM")})
	}
	for i, item := range m.items {
		at, end := m.interval(item)
		hideEnd := at.Equal(end) || (end.Equal(m.clock) && m.clock.Before(until))
		if i+1 < len(m.items) && m.items[i+1].StartedAt.Equal(end) {
			hideEnd = true
		}
		entries = append(entries, railEntry{
			at:       at,
			priority: 1,
			id:       item.EventID,
			content:  m.cardContent(item),
			selected: i == m.position.eventIndex,
			end:      end,
			hideEnd:  hideEnd,
		})
	}
	if !m.clock.Before(m.day) && m.clock.Before(until) {
		entries = append(entries, railEntry{at: m.clock, priority: 2, now: true, text: "── Now · " + displaytime.Format(m.clock, "03:04 PM") + " ──"})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].at.Equal(entries[j].at) {
			return entries[i].priority < entries[j].priority
		}
		return entries[i].at.Before(entries[j].at)
	})
	lines := []string{}
	var cards []timelineCard
	positions := make(map[int64]int)
	var curves []render.Distribution
	selectedCurve := -1
	nowLine := -1
	hourHeight := m.hourHeight()
	previousTop := 0
	var previousTime time.Time
	for i, entry := range entries {
		// Covered hours are omitted; cards show only their boundaries.
		if entry.id == 0 && !entry.now && !previousTime.IsZero() && !entry.at.After(previousTime) {
			continue
		}
		// An event starting on the hour owns that label's row, not a row below it.
		if entry.id == 0 && !entry.now && i+1 < len(entries) && entries[i+1].id != 0 && entry.at.Equal(entries[i+1].at) {
			continue
		}
		if !previousTime.IsZero() {
			gap := int(math.Ceil(entry.at.Sub(previousTime).Hours() * float64(hourHeight)))
			for len(lines) < previousTop+gap {
				lines = append(lines, "          │")
			}
		}
		if entry.id != 0 && len(lines) == previousTop+1 && !previousTime.IsZero() {
			// A small visual separator is not an elapsed-time gap.
			lines = append(lines, "")
		}
		previousTop, previousTime = len(lines), entry.at
		if entry.id != 0 {
			positions[entry.id] = len(lines)
		}
		if entry.now {
			nowLine = len(lines)
		}
		if entry.now {
			lines = append(lines, "          "+entry.text)
		} else if entry.id == 0 {
			lines = append(lines, fmt.Sprintf("%-10s│", entry.text))
		} else {
			height := len(entry.content) + 2
			cards = append(cards, timelineCard{top: len(lines), content: entry.content, selected: entry.selected})
			if m.cardColumn() != 10 && entry.id != m.expanded && m.load.countErr == nil {
				for _, count := range m.counts {
					if count.EventID != entry.id {
						continue
					}
					if entry.id == m.items[m.position.eventIndex].EventID {
						selectedCurve = len(curves)
					}
					// Counts affect the renderer's shape, never the card height.
					center := float64(len(lines)*4) + float64(height*4-1)/2
					curves = append(curves, render.Distribution{CenterY: center, Count: count.Count})
					break
				}
			}
			labels := make([]string, height)
			labels[0] = displaytime.Format(entry.at, "03:04 PM")
			if !entry.hideEnd {
				labels[height-1] = displaytime.Format(entry.end, "03:04 PM")
			}
			for _, label := range labels {
				lines = append(lines, fmt.Sprintf("%-10s", label))
			}
			previousTop, previousTime = len(lines)-1, entry.end
		}
	}
	end := len(lines)
	if nowLine >= 0 {
		end = nowLine
	}
	if m.day.Before(displaytime.Day(m.clock)) {
		end = len(lines)
		lines = append(lines, "          ...")
	}
	return timelineLayout{
		lines:         lines,
		cards:         cards,
		positions:     positions,
		curves:        curves,
		selectedCurve: selectedCurve,
		nowLine:       nowLine,
		curveEnd:      end,
	}
}

// paintTimeline renders only cards intersecting the requested rows. A partially
// visible card is painted with its normal border, then cropped at the viewport.
func (m *Model) paintTimeline(layout timelineLayout, offset, height int) []string {
	end := min(len(layout.lines), offset+height)
	lines := append([]string{}, layout.lines...)
	for _, card := range layout.cards {
		bottom := card.top + len(card.content) + 2
		if bottom <= offset || card.top >= end {
			continue
		}
		painted := m.paintCard(card.content, card.selected)
		for row := max(offset, card.top); row < min(end, bottom); row++ {
			lines[row] += painted[row-card.top]
		}
	}
	lines = m.addDistributions(lines, layout.curves, layout.selectedCurve, layout.curveEnd)
	return lines[offset:end]
}

func (m *Model) bodyHeight() int { return max(1, m.height-4) }
func (m *Model) revealSelected() {
	layout := m.measureTimeline()
	if len(m.items) > 0 {
		top := layout.positions[m.items[m.position.eventIndex].EventID]
		m.position.showSelected(top, m.bodyHeight(), m.expanded != 0)
	}
	m.position.clamp(len(layout.lines))
}
func (m *Model) anchor() {
	if m.day.IsZero() {
		return
	}
	if !m.position.followNow && m.expanded != 0 {
		m.revealSelected()
		return
	}
	layout := m.measureTimeline()
	m.position.showNow(layout.nowLine, m.bodyHeight())
	m.position.clamp(len(layout.lines))
}

func (m *Model) timelineBody() string {
	layout := m.measureTimeline()
	position := m.position
	position.clamp(len(layout.lines))
	offset := position.topLine
	visible := m.paintTimeline(layout, offset, m.bodyHeight())
	for len(visible) < m.bodyHeight() {
		visible = append(visible, "")
	}
	for i := range visible {
		visible[i] = ansi.Truncate(visible[i], m.width, "")
	}
	return strings.Join(visible, "\n")
}

func (m Model) View() string {
	styles := list.DefaultStyles(true)
	heading := styles.Title.Render("Events") + " " + displaytime.Format(m.day, "January 2, 2006")
	if m.width < 24 || m.height < 14 {
		return heading + "\nResize terminal to view events."
	}
	if m.form.open {
		return heading + "\n" + m.formView()
	}
	if m.picker.ShowingDetail() {
		return styles.Title.Render(m.picker.SelectedSubjectName()) + "\n\n" + m.picker.View()
	}
	body := m.load.retainedBody
	if !m.load.eventsPending || body == "" {
		body = m.timelineBody()
	}
	status := m.message
	if status == "" {
		status = "d: curve settings"
	}
	if m.load.eventsPending && m.load.showStatus {
		status = "Loading events for " + displaytime.Format(m.load.pendingDay, "January 2, 2006") + "…"
	} else if !m.load.eventsPending && len(m.items) == 0 && m.message == "" {
		status = "No events on this day. n: start event • d: curves"
	}
	if m.expanded != 0 {
		status = "←: collapse • →: open • ↑/↓: thoughts • PgUp/PgDn: scroll"
	}
	help := "Home: first • End: now • ←/→: day • r: refresh • n: start • e: end • q: quit"
	if m.day.Equal(displaytime.Day(m.clock)) {
		help = "← day / → open • Home/End: first/now • r: refresh • n: start • e: end • q: quit"
	}
	if m.expanded != 0 {
		help = "r: refresh event • d: curves • q: quit"
	}
	if m.distributions.open {
		status = m.distributions.label()
		if m.width < 60 {
			status = "Curves hidden below 60 columns"
		}
		if m.load.eventsPending && m.load.showStatus {
			status = "Loading… " + status
		}
		help = "←/→ boost  ↑/↓ size  f mode  [0 Reset to default]  Esc done"
		if m.width < 60 {
			help = "f mode · [0 Reset to default] · Esc done"
		}
	}
	return heading + "\n\n" + body + "\n" + ansi.Truncate(status, m.width, "…") + "\n" + ansi.Truncate(help, m.width, "…")
}
