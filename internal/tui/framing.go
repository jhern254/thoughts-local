package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"strings"
)

const (
	framingFocusPreview = iota
	framingFocusReset
	framingFocusDone
	framingFocusCancel
	framingFocusCount
)

func (m *Model) toggleBackgroundFit() {
	if m.options.framing.Fit == "fill" {
		m.options.framing.Fit = "fit"
	} else {
		m.options.framing.Fit = "fill"
	}
}

func (m *Model) finishFraming(cancel bool) {
	if cancel {
		m.options.framing = m.options.framingBeforeEdit
	}
	m.options.editing = false
	m.requestOptions("") // Retire drag messages from the previous framing view.
}

func (m *Model) activateFraming() {
	switch m.options.framingFocus {
	case framingFocusPreview, framingFocusDone:
		m.finishFraming(false)
	case framingFocusReset:
		m.options.framing = data.DefaultBackgroundFraming()
	case framingFocusCancel:
		m.finishFraming(true)
	}
}

func (m Model) updateFramingInput(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "q":
			m.finishFraming(true)
		case "tab":
			m.options.framingFocus = (m.options.framingFocus + 1) % framingFocusCount
		case "shift+tab":
			m.options.framingFocus = (m.options.framingFocus + framingFocusCount - 1) % framingFocusCount
		case "enter", "space":
			m.activateFraming()
		case "+", "=":
			m.options.framing.Zoom = min(300, m.options.framing.Zoom+5)
		case "-":
			m.options.framing.Zoom = max(100, m.options.framing.Zoom-5)
		case "left", "right", "up", "down":
			if m.options.framingFocus == framingFocusPreview {
				// Arrow direction moves the image, so right/down reduce the overflow offset.
				switch msg.String() {
				case "left":
					m.options.framing.PositionX = min(10000, m.options.framing.PositionX+100)
				case "right":
					m.options.framing.PositionX = max(0, m.options.framing.PositionX-100)
				case "up":
					m.options.framing.PositionY = min(10000, m.options.framing.PositionY+100)
				case "down":
					m.options.framing.PositionY = max(0, m.options.framing.PositionY-100)
				}
			} else if msg.String() == "up" {
				m.options.framingFocus = max(framingFocusPreview, m.options.framingFocus-1)
			} else if msg.String() == "down" {
				m.options.framingFocus = min(framingFocusCancel, m.options.framingFocus+1)
			}
		}
	case tea.MouseClickMsg:
		width, rect := m.optionsLayout()
		firstAction := 5
		if rect.Height > 0 {
			firstAction = rect.Y + rect.Height + 2
		}
		if msg.Button == tea.MouseLeft && msg.X >= rect.X-1 && msg.X < rect.X-1+width && msg.Y >= firstAction && msg.Y <= firstAction+2 {
			m.options.framingFocus = framingFocusReset + msg.Y - firstAction
			m.activateFraming()
		}
	}
	return m, nil
}

func (m Model) viewFraming() tea.View {
	width, rect := m.optionsLayout()
	selected := m.subjects.list.Styles.Title
	plain := lipgloss.NewStyle().Padding(0, 1)
	label := func(focus int, text string) string {
		if m.options.framingFocus == focus {
			return selected.Render(text)
		}
		return plain.Render(text)
	}
	lines := []string{selected.Render("Adjust framing"), label(framingFocusPreview, fmt.Sprintf("Preview · Zoom: %d%%", m.options.framing.Zoom)), ""}
	if rect.Height > 0 {
		lines = append(lines, "╭"+strings.Repeat("─", rect.Width)+"╮")
		for range rect.Height {
			lines = append(lines, "│"+strings.Repeat(" ", rect.Width)+"│")
		}
		lines = append(lines, "╰"+strings.Repeat("─", rect.Width)+"╯")
	} else {
		lines = append(lines, "Preview needs a taller window.")
	}
	lines = append(lines, "", label(framingFocusReset, "Reset framing"), label(framingFocusDone, "Done"), label(framingFocusCancel, "Cancel"), "", "Drag/↑↓←→: move · +/−: zoom", "Tab: focus · Enter: accept", "Esc/Q: cancel")
	for i, line := range lines {
		lines[i] = strings.Repeat(" ", rect.X-1) + ansi.Truncate(line, width, "…")
	}
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}
