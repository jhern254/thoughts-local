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
func (m Model) count(id int64) string {
	if m.countPending {
		return "Counting thoughts…"
	}
	if m.countErr != nil {
		return "Thought count unavailable"
	}
	for _, item := range m.counts {
		if item.EventID == id {
			if item.Count == 1 {
				return "1 thought"
			}
			return fmt.Sprintf("%d thoughts", item.Count)
		}
	}
	return "Thought count unavailable"
}
func (m Model) card(item data.Event, selected bool) string {
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
	width := max(1, m.width-12)
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
			case m.latestPending:
				content += "Loading latest thought…"
			case m.latestErr != nil:
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
	content = strings.Join(lines, "\n")
	start, finish := m.interval(item)
	// Keep every whole-hour marker readable, without duration-based padding.
	height := len(m.hoursBetween(start, finish)) + 2
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Width(width + 2).Height(height)
	if selected && !m.blurred {
		style = style.BorderForeground(lipgloss.Color("62"))
	}
	return style.Render(content)
}

// Clip only the calendar geometry, not the event's actual timestamps or count.
func (m Model) interval(item data.Event) (time.Time, time.Time) {
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

func (m Model) hourHeight() int { return max(4, (m.bodyHeight()+4)/5) }

func (m Model) hoursBetween(start, end time.Time) []time.Time {
	var hours []time.Time
	for hour := m.day; hour.Before(end); hour = hour.Add(time.Hour) {
		if hour.After(start) {
			hours = append(hours, hour)
		}
	}
	return hours
}

type railEntry struct {
	at       time.Time
	priority int
	id       int64
	text     string
	now      bool
	end      time.Time
	hideEnd  bool
}

// layout returns actual rendered row positions, so expansion and the live
// marker share the same chronological rail without assuming fixed card heights.
func (m Model) layout() ([]string, map[int64]int, int) {
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
		entries = append(entries, railEntry{at: at, end: end, hideEnd: hideEnd, priority: 1, id: item.EventID, text: m.card(item, i == m.index)})
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
	positions := make(map[int64]int)
	nowLine := -1
	hourHeight := m.hourHeight()
	previousTop := 0
	var previousTime time.Time
	for i, entry := range entries {
		// Hours within an event are drawn alongside its box below.
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
			card := strings.Split(entry.text, "\n")
			labels := make([]string, len(card))
			labels[0] = displaytime.Format(entry.at, "03:04 PM")
			if !entry.hideEnd {
				labels[len(card)-1] = displaytime.Format(entry.end, "03:04 PM")
			}
			hours := m.hoursBetween(entry.at, entry.end)
			previousRow := 0
			for i, hour := range hours {
				fraction := float64(hour.Sub(entry.at)) / float64(entry.end.Sub(entry.at))
				// Reserve a distinct row for each remaining marker, including
				// repeated local hours across a daylight-saving transition.
				row := min(len(card)-1-(len(hours)-i), max(previousRow+1, int(math.Round(fraction*float64(len(card)-1)))))
				labels[row] = displaytime.Format(hour, "03:04 PM")
				previousRow = row
			}
			for row, text := range card {
				lines = append(lines, fmt.Sprintf("%-10s%s", labels[row], text))
			}
			previousTop, previousTime = len(lines)-1, entry.end
		}
	}
	if m.day.Before(displaytime.Day(m.clock)) {
		lines = append(lines, "          ...")
	}
	return lines, positions, nowLine
}
func (m *Model) clampOffset(lineCount int) {
	// Keep the chosen card at the top even near the end of the day; unused
	// viewport rows belong below it, not before it as unrelated earlier hours.
	m.offset = min(max(0, m.offset), max(0, lineCount-1))
}
func (m Model) bodyHeight() int { return max(1, m.height-4) }
func (m *Model) revealSelected() {
	lines, positions, _ := m.layout()
	if len(m.items) > 0 {
		top := positions[m.items[m.index].EventID]
		if top < m.offset || top >= m.offset+m.bodyHeight()-3 || m.expanded != 0 {
			m.offset = top
		}
	}
	m.clampOffset(len(lines))
}
func (m *Model) anchor() {
	if m.day.IsZero() {
		return
	}
	if !m.following && m.expanded != 0 {
		m.revealSelected()
		return
	}
	lines, _, now := m.layout()
	if m.following && now >= 0 {
		m.offset = max(0, now-m.bodyHeight()+2)
	}
	m.clampOffset(len(lines))
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
	lines, _, _ := m.layout()
	offset := min(max(0, m.offset), max(0, len(lines)-1))
	visible := append([]string{}, lines[offset:min(len(lines), offset+m.bodyHeight())]...)
	for len(visible) < m.bodyHeight() {
		visible = append(visible, "")
	}
	status := m.message
	if m.loading {
		status = "Loading events…"
	} else if len(m.items) == 0 && status == "" {
		status = "No events on this day. n: start event"
	}
	if m.expanded != 0 {
		status = "←: collapse • →: open • ↑/↓: thoughts • PgUp/PgDn: scroll"
	}
	for i := range visible {
		visible[i] = ansi.Truncate(visible[i], m.width, "")
	}
	help := "Home: first • End: now • ←/→: day • r: refresh • n: start • e: end • q: quit"
	if m.day.Equal(displaytime.Day(m.clock)) {
		help = "← day / → open • Home/End: first/now • r: refresh • n: start • e: end • q: quit"
	}
	if m.expanded != 0 {
		help = "r: refresh event • q: quit"
	}
	return heading + "\n\n" + strings.Join(visible, "\n") + "\n" + ansi.Truncate(status, m.width, "…") + "\n" + ansi.Truncate(help, m.width, "…")
}
