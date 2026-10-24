package tui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/visual"
)

const videoPreparationRows = 3

const (
	trimFocusSource = iota
	trimFocusEnd
	trimFocusQuality
	trimFocusSpeed
	trimFocusPreview
	trimFocusDone
	trimFocusCancel
	trimFocusCount
)

func trimSeconds(milliseconds int64) string { return fmt.Sprintf("%.2f", float64(milliseconds)/1000) }

func (m Model) videoTrimEndLimit() int64 {
	return min(m.options.sourceDurationMS, m.options.trim.StartMS+visual.MaxVideoDuration*1000)
}

func (m *Model) activateVideoTrim() {
	if m.options.request == "discardVideo" {
		return
	}
	switch m.options.trimFocus {
	case trimFocusSource:
		m.options.trimTimecode = textinput.New()
		m.options.trimTimecode.Prompt = "Source / Start: "
		m.options.trimTimecode.CharLimit = 12
		m.options.trimTimecode.SetValue(trimTimecode(m.options.trim.StartMS))
		m.options.trimTimecode.Focus()
		m.options.trimTimecodeEditing = true
	case trimFocusQuality:
		m.cycleVideoQuality()
	case trimFocusPreview:
		if !m.options.trim.Valid(m.options.sourceDurationMS) {
			m.options.status = "Choose 1–30 seconds within the source."
			return
		}
		m.options.status = "Preparing video…"
		m.requestOptions("prepareLoop")
	case trimFocusDone:
		if !m.videoTrimDoneEnabled() {
			return
		}
		m.options.trimming = false
		m.options.focus = optionsFocusTrim
		m.requestOptions("")
	case trimFocusCancel:
		m.options.status = "Cancelling upload…"
		m.requestOptions("discardVideo")
	}
}

func (m Model) updateVideoTrimInput(message tea.Msg) (tea.Model, tea.Cmd) {
	if m.options.trimTimecodeEditing && m.options.request == "" {
		if key, ok := message.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "esc":
				m.options.trimTimecodeEditing = false
				m.options.status = ""
				return m, nil
			case "enter":
				position, ok := parseTrimTimecode(m.options.trimTimecode.Value())
				if !ok {
					m.options.status = "Use HH:MM:SS with optional milliseconds."
					return m, nil
				}
				m.setTrimStart(position)
				m.options.trimTimecodeEditing = false
				m.options.status = ""
				return m, nil
			}
		}
		var command tea.Cmd
		m.options.trimTimecode, command = m.options.trimTimecode.Update(message)
		return m, command
	}
	switch msg := message.(type) {
	case tea.KeyPressMsg:
		key := msg.String()
		if key == "esc" || key == "q" {
			m.options.trimFocus = trimFocusCancel
			m.activateVideoTrim()
			return m, nil
		}
		if m.options.request != "" {
			if m.options.request == "prepareLoop" {
				switch key {
				case "tab", "shift+tab", "up", "down":
					m.options.trimFocus = trimFocusCancel
				case "enter", "space":
					if m.options.trimFocus == trimFocusCancel {
						m.activateVideoTrim()
					}
				}
			}
			return m, nil
		}
		switch key {
		case "tab", "down":
			m.moveVideoTrimFocus(1)
		case "shift+tab", "up":
			m.moveVideoTrimFocus(-1)
		case "enter", "space":
			m.activateVideoTrim()
		case "left", "right", "shift+left", "shift+right", "home", "end", "pgup", "pgdown":
			if m.options.trimFocus == trimFocusQuality {
				m.setVideoQuality(key)
				return m, nil
			}
			if m.options.trimFocus == trimFocusSpeed {
				if strings.Contains(key, "left") {
					m.options.speedPercent = max(25, m.options.speedPercent-5)
				}
				if strings.Contains(key, "right") {
					m.options.speedPercent = min(100, m.options.speedPercent+5)
				}
				return m, nil
			}
			if m.options.trimFocus != trimFocusSource && m.options.trimFocus != trimFocusEnd {
				return m, nil
			}
			value := m.options.trim.StartMS
			lower, upper := int64(0), max(int64(0), m.options.sourceDurationMS-1000)
			if m.options.trimFocus == trimFocusEnd {
				value = m.options.trim.EndMS
				lower = m.options.trim.StartMS + 1000
				upper = m.videoTrimEndLimit()
			}
			step := int64(100)
			if strings.HasPrefix(key, "shift+") {
				step = 1000
			}
			switch key {
			case "home":
				value = lower
			case "end":
				value = upper
			case "pgup":
				value -= 60000
			case "pgdown":
				value += 60000
			default:
				if strings.HasSuffix(key, "left") {
					value -= step
				} else {
					value += step
				}
			}
			value = max(lower, min(upper, value))
			if m.options.trimFocus == trimFocusSource {
				m.setTrimStart(value)
			} else {
				m.options.trim.EndMS = value
			}

		}
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			m.clickVideoTrim(msg.X, msg.Y, true)
		}
	case tea.MouseMotionMsg:
		if msg.Button == tea.MouseLeft && m.options.request == "" {
			m.clickVideoTrim(msg.X, msg.Y, false)
		}
	}
	return m, nil
}

func (m Model) videoTrimDoneEnabled() bool {
	return m.options.loopReady && m.options.request == ""
}

func (m *Model) moveVideoTrimFocus(direction int) {
	m.options.trimFocus = (m.options.trimFocus + direction + trimFocusCount) % trimFocusCount
	if m.options.trimFocus == trimFocusDone && !m.videoTrimDoneEnabled() {
		m.options.trimFocus = (m.options.trimFocus + direction + trimFocusCount) % trimFocusCount
	}
}

func videoTrimControlsTop(preview PreviewRect) int {
	if preview.Height > 0 {
		return preview.Y + preview.Height + 2
	}
	return 4
}

func (m Model) videoPreparationRect() PreviewRect {
	if !m.options.trimming {
		return PreviewRect{}
	}
	_, preview := m.optionsLayout()
	return PreviewRect{X: preview.X, Y: videoTrimControlsTop(preview) + 6, Width: preview.Width, Height: videoPreparationRows}
}

func (m *Model) clickVideoTrim(x, y int, activate bool) {
	width, rect := m.optionsLayout()
	left := rect.X - 1
	if x < left || x >= left+width {
		return
	}
	row := y - videoTrimControlsTop(rect)
	if row == 1 && m.options.request == "" {
		m.options.trimFocus = trimFocusSource
		m.setTrimStart(int64(max(0, min(width-1, x-left))) * m.options.sourceDurationMS / int64(max(1, width-1)))
		return
	}
	if !activate {
		return
	}
	var focus int
	switch row {
	case 0:
		focus = trimFocusSource
	case 2:
		focus = trimFocusEnd
	case 4:
		focus = trimFocusQuality
	case 5:
		focus = trimFocusSpeed
	case 6 + videoPreparationRows:
		focus = trimFocusPreview
	case 7 + videoPreparationRows:
		focus = trimFocusDone
	case 8 + videoPreparationRows:
		focus = trimFocusCancel
	default:
		return
	}
	if m.options.request != "" && focus != trimFocusCancel {
		return
	}
	if focus == trimFocusDone && !m.videoTrimDoneEnabled() {
		return
	}
	m.options.trimFocus = focus
	if focus >= trimFocusPreview {
		m.activateVideoTrim()
	}
}

func (m Model) viewVideoTrim() tea.View {
	width, rect := m.optionsLayout()
	selected := m.subjects.list.Styles.Title
	plain := lipgloss.NewStyle().Padding(0, 1)
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#999999"))
	label := func(focus int, text string) string {
		if focus == trimFocusDone && !m.videoTrimDoneEnabled() {
			return muted.Padding(0, 1).Render(text)
		}
		if focus == m.options.trimFocus {
			return selected.Render(text)
		}
		return plain.Render(text)
	}
	heading := "Visual · Prepared loop"
	if m.options.sourceFrame {
		heading = "Visual · Source frame"
	}
	lines := []string{selected.Render("Options"), "", heading, ""}
	if rect.Height > 0 {
		lines = append(lines, muted.Render("╭"+strings.Repeat("─", rect.Width)+"╮"))
		for range rect.Height {
			lines = append(lines, muted.Render("│")+strings.Repeat(" ", rect.Width)+muted.Render("│"))
		}
		lines = append(lines, muted.Render("╰"+strings.Repeat("─", rect.Width)+"╯"), "")
	}
	peak := int(min(m.options.trim.StartMS, m.options.sourceDurationMS) * int64(width-1) / max(1, m.options.sourceDurationMS))
	rail := strings.Repeat("─", peak) + "●" + strings.Repeat("─", width-peak-1)
	lines = append(lines,
		label(trimFocusSource, "Source / Start: "+trimTimecode(m.options.trim.StartMS)+" / "+trimTimecode(m.options.sourceDurationMS)), muted.Render(rail),
		label(trimFocusEnd, "End: "+trimTimecode(m.options.trim.EndMS)+" (max "+trimTimecode(m.videoTrimEndLimit())+")"),
		plain.Render(fmt.Sprintf("Selected: %.1f seconds", float64(m.options.trim.EndMS-m.options.trim.StartMS)/1000)),
		label(trimFocusQuality, "Video quality: "+m.options.videoQuality.Description()),
		label(trimFocusSpeed, fmt.Sprintf("Playback speed: %d%%", m.options.speedPercent)))
	for range videoPreparationRows {
		lines = append(lines, "")
	}
	lines = append(lines, label(trimFocusPreview, "Preview loop"), label(trimFocusDone, "Done — use previewed loop"), label(trimFocusCancel, "Cancel upload"), "")
	if m.options.loopReady {
		lines = append(lines, m.acceptedVideoLabel(), m.acceptedVideoRangeLabel(), m.draftVideoQualityLabel())
	} else {
		lines = append(lines, "No prepared loop yet.")
	}
	if m.options.trimTimecodeEditing {
		for i, line := range lines {
			if strings.Contains(ansi.Strip(line), "Source / Start:") {
				lines[i] = selected.Render(m.options.trimTimecode.View())
				break
			}
		}
	}
	lines = append(lines, "", muted.Render(m.options.status))
	if m.options.request == "prepareLoop" {
		lines = append(lines, muted.Render("Tab: Cancel · Esc: cancel upload"), muted.Render("Cut and quality controls resume when ready."))
	} else if m.options.trimTimecodeEditing {
		lines = append(lines, muted.Render("Enter: jump · Esc: back"), muted.Render("HH:MM:SS or HH:MM:SS.mmm"))
	} else {
		lines = append(lines, muted.Render("←/→: 0.1s · Shift: 1s · ↑/↓: focus"), muted.Render("PgUp/Dn: 60s · Home/End · Enter: timecode"))
	}
	for i, line := range lines {
		lines[i] = strings.Repeat(" ", rect.X-1) + ansi.Truncate(line, width, "…")
	}
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

// Start is the sole source-preview cursor. Accepted encoded loops own their
// separate playback position and survive all unpreviewed draft edits.
func (m *Model) setTrimStart(positionMS int64) {
	m.options.trim.StartMS = max(int64(0), min(positionMS, m.options.sourceDurationMS-1000))
	m.options.trim.EndMS = m.videoTrimEndLimit()
	m.options.sourceFrame = true
}
func trimTimecode(milliseconds int64) string {
	return fmt.Sprintf("%02d:%02d:%02d.%03d", milliseconds/3600000, (milliseconds/60000)%60, (milliseconds/1000)%60, milliseconds%1000)
}
func parseTrimTimecode(value string) (int64, bool) {
	parts := strings.Split(value, ":")
	if len(parts) != 3 || len(parts[0]) != 2 || len(parts[1]) != 2 {
		return 0, false
	}
	seconds := strings.Split(parts[2], ".")
	if len(seconds) > 2 || len(seconds[0]) != 2 {
		return 0, false
	}
	digits := []string{parts[0], parts[1], seconds[0]}
	fraction := "000"
	if len(seconds) == 2 {
		if len(seconds[1]) < 1 || len(seconds[1]) > 3 {
			return 0, false
		}
		fraction = seconds[1] + strings.Repeat("0", 3-len(seconds[1]))
		digits = append(digits, seconds[1])
	}
	for _, part := range digits {
		for _, digit := range part {
			if digit < '0' || digit > '9' {
				return 0, false
			}
		}
	}
	hours, _ := strconv.ParseInt(parts[0], 10, 64)
	minutes, _ := strconv.ParseInt(parts[1], 10, 64)
	secs, _ := strconv.ParseInt(seconds[0], 10, 64)
	ms, _ := strconv.ParseInt(fraction, 10, 64)
	position := hours*3600000 + minutes*60000 + secs*1000 + ms
	return position, minutes < 60 && secs < 60 && position <= visual.MaxVideoSourceDuration*1000
}
