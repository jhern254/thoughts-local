package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/visual"
)

const (
	trimFocusSource = iota
	trimFocusStart
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
	switch m.options.trimFocus {
	case trimFocusStart:
		m.options.trim.StartMS = m.options.sourcePositionMS
		m.options.trim.EndMS = min(m.options.trim.EndMS, m.videoTrimEndLimit())
	case trimFocusEnd:
		m.options.trim.EndMS = min(m.options.sourcePositionMS, m.videoTrimEndLimit())
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
		if !m.options.loopReady {
			m.options.status = "Preview a loop before Done."
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
			m.options.trimFocus = (m.options.trimFocus + 1) % trimFocusCount
		case "shift+tab", "up":
			m.options.trimFocus = (m.options.trimFocus + trimFocusCount - 1) % trimFocusCount
		case "enter", "space":
			m.activateVideoTrim()
		case "left", "right", "shift+left", "shift+right", "home", "end":
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
			var value *int64
			switch m.options.trimFocus {
			case trimFocusSource:
				value = &m.options.sourcePositionMS
			case trimFocusStart:
				value = &m.options.trim.StartMS
			case trimFocusEnd:
				value = &m.options.trim.EndMS
			}
			if value == &m.options.sourcePositionMS {
				m.options.sourceFrame = true
			}
			if value == nil {
				return m, nil
			}
			step := int64(100)
			if strings.HasPrefix(key, "shift+") {
				step = 1000
			}
			switch key {
			case "home":
				*value = 0
			case "end":
				*value = m.options.sourceDurationMS
			default:
				if strings.HasSuffix(key, "left") {
					*value = max(0, *value-step)
				} else {
					*value = min(m.options.sourceDurationMS, *value+step)
				}
			}
			if m.options.trimFocus == trimFocusStart || m.options.trimFocus == trimFocusEnd {
				m.options.trim.EndMS = min(m.options.trim.EndMS, m.videoTrimEndLimit())
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

func (m *Model) clickVideoTrim(x, y int, activate bool) {
	width, rect := m.optionsLayout()
	left := rect.X - 1
	if x < left || x >= left+width {
		return
	}
	top := 5
	if rect.Height > 0 {
		top = rect.Y + rect.Height + 2
	}
	if m.options.request != "" && (!activate || y != top+8) {
		return
	}
	if y == top+1 {
		m.options.trimFocus = trimFocusSource
		m.options.sourceFrame = true
		m.options.sourcePositionMS = int64(max(0, min(width-1, x-left))) * m.options.sourceDurationMS / int64(max(1, width-1))
	} else if activate && y >= top+2 && y <= top+8 {
		m.options.trimFocus = trimFocusStart + y - (top + 2)
		if m.options.trimFocus >= trimFocusPreview {
			m.activateVideoTrim()
		}
	}
}

func (m Model) viewVideoTrim() tea.View {
	width, rect := m.optionsLayout()
	selected := m.subjects.list.Styles.Title
	plain := lipgloss.NewStyle().Padding(0, 1)
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#999999"))
	label := func(focus int, text string) string {
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
	} else {
		lines = append(lines, "")
	}
	peak := int(m.options.sourcePositionMS * int64(width-1) / max(1, m.options.sourceDurationMS))
	rail := strings.Repeat("─", peak) + "●" + strings.Repeat("─", width-peak-1)
	lines = append(lines,
		label(trimFocusSource, "Source: "+trimSeconds(m.options.sourcePositionMS)+" / "+trimSeconds(m.options.sourceDurationMS)+"s"), muted.Render(rail),
		label(trimFocusStart, "Start: "+trimSeconds(m.options.trim.StartMS)+"s"),
		label(trimFocusEnd, "End: "+trimSeconds(m.options.trim.EndMS)+"s (max "+trimSeconds(m.videoTrimEndLimit())+"s)"),
		label(trimFocusQuality, "Video quality: "+m.options.videoQuality.Description()),
		label(trimFocusSpeed, fmt.Sprintf("Playback speed: %d%%", m.options.speedPercent)),
		label(trimFocusPreview, "Preview loop"), label(trimFocusDone, "Done — use previewed loop"), label(trimFocusCancel, "Cancel upload"))
	if m.options.loopReady {
		lines = append(lines, m.acceptedVideoLabel(), m.acceptedVideoRangeLabel(), m.draftVideoQualityLabel())
	} else {
		lines = append(lines, "No prepared loop yet.")
	}
	lines = append(lines, muted.Render(m.options.status))
	if m.options.request == "prepareLoop" {
		lines = append(lines, muted.Render("Tab: Cancel · Esc: cancel upload"), muted.Render("Cut and quality controls resume when ready."))
	} else {
		lines = append(lines, muted.Render("←/→: 0.1s · Shift: 1s · ↑/↓: focus"), muted.Render("Enter on Start/End: use source position"))
	}
	for i, line := range lines {
		lines[i] = strings.Repeat(" ", rect.X-1) + ansi.Truncate(line, width, "…")
	}
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}
