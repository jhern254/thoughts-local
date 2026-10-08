package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type BrowserOptionsEnabledMsg struct{}

// BrowserOptionsMsg carries only bounded visual control metadata. Images
// stay in the browser and travel through the existing validated HTTP routes.
type BrowserOptionsMsg struct {
	Action   string `json:"action"`
	ID       uint64 `json:"id"`
	Darkness int    `json:"darkness"`
}

type PreviewRect struct{ X, Y, Width, Height int }

// BrowserOptionsState exposes the reserved terminal cells, not pixel geometry.
// ID changes for each browser operation so late results cannot affect a new page.
type BrowserOptionsState struct {
	Open     bool        `json:"open"`
	ID       uint64      `json:"id"`
	Request  string      `json:"request"`
	Darkness int         `json:"darkness"`
	Preview  PreviewRect `json:"preview"`
}

const (
	optionsFocusChooseImage = iota
	optionsFocusDarkness
	optionsFocusApply
	optionsFocusRemoveBackground
	optionsFocusBack
	optionsFocusCount
)

type optionsState struct {
	open     bool
	id       uint64
	request  string
	darkness int
	focus    int
	status   string
}

func (m *Model) requestOptions(action string) {
	m.options.id++
	m.options.request = action
}

func (m Model) OptionsState() BrowserOptionsState {
	_, rect := m.optionsLayout()
	return BrowserOptionsState{
		Open:     m.options.open,
		ID:       m.options.id,
		Request:  m.options.request,
		Darkness: m.options.darkness,
		Preview:  rect,
	}
}

func (m Model) updateBrowserOptions(msg BrowserOptionsMsg) (tea.Model, tea.Cmd) {
	if !m.browserOptionsEnabled || m.exitPromptOpen {
		return m, nil
	}
	if msg.Action == "open" {
		if !m.options.open {
			m.options.open = true
			m.options.focus = optionsFocusChooseImage
			m.options.darkness = 70
			m.options.status = "Loading visual…"
			m.requestOptions("load")
		}
		return m, nil
	}
	if !m.options.open || msg.ID != m.options.id || m.options.request == "" {
		return m, nil
	}
	switch msg.Action {
	case "loaded":
		if m.options.request != "load" {
			return m, nil
		}
		m.options.darkness = max(0, min(95, msg.Darkness))
		m.options.status = "JPEG / PNG · 16 MiB · 8192px per side · 24MP"
	case "selected":
		if m.options.request != "choose" {
			return m, nil
		}
		m.options.status = "Preview only. Apply to save."
	case "cancelled":
		if m.options.request != "choose" {
			return m, nil
		}
		m.options.status = ""
	case "saved", "warning":
		if m.options.request != "apply" && m.options.request != "remove" {
			return m, nil
		}
		m.options.darkness = max(0, min(95, msg.Darkness))
		if msg.Action == "saved" {
			m.options.open = false
		} else {
			m.options.status = "Saved. Previous image file could not be removed."
		}
	case "failed":
		m.options.status = "Could not load or apply. Check image limits and retry."
	default:
		return m, nil
	}
	m.options.request = ""
	return m, nil
}

func (m Model) updateOptionsInput(message tea.Msg) (tea.Model, tea.Cmd) {
	busy := m.options.request != ""
	switch msg := message.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc":
			if m.options.request != "apply" && m.options.request != "remove" {
				m.options.open = false
				m.requestOptions("")
			}
		case "tab", "down":
			if !busy {
				m.options.focus = (m.options.focus + 1) % optionsFocusCount
			}
		case "shift+tab", "up":
			if !busy {
				m.options.focus = (m.options.focus + optionsFocusCount - 1) % optionsFocusCount
			}
		case "left", "right", "home", "end":
			if !busy && m.options.focus == optionsFocusDarkness {
				switch msg.String() {
				case "left":
					m.options.darkness = max(0, m.options.darkness-5)
				case "right":
					m.options.darkness = min(95, m.options.darkness+5)
				case "home":
					m.options.darkness = 0
				case "end":
					m.options.darkness = 95
				}
			}
		case "enter", "space":
			if !busy {
				m.activateOption()
			}
		}
	case tea.MouseClickMsg:
		if !busy && msg.Button == tea.MouseLeft {
			m.clickOption(msg.X, msg.Y, true)
		}
	case tea.MouseMotionMsg:
		if !busy && msg.Button == tea.MouseLeft {
			m.clickOption(msg.X, msg.Y, false)
		}
	}
	return m, nil
}

func (m *Model) activateOption() {
	switch m.options.focus {
	case optionsFocusChooseImage:
		m.options.status = "Choose a JPEG or PNG…"
		m.requestOptions("choose")
	case optionsFocusApply:
		m.options.status = "Saving…"
		m.requestOptions("apply")
	case optionsFocusRemoveBackground:
		m.options.status = "Removing…"
		m.requestOptions("remove")
	case optionsFocusBack:
		m.options.open = false
		m.requestOptions("")
	}
}

// The preview shrinks before the controls. Very short terminals omit it entirely.
func (m Model) optionsLayout() (int, PreviewRect) {
	width := max(10, min(60, m.width-4))
	x := max(0, (m.width-width)/2)
	height := max(0, min(8, m.height-21))
	return width, PreviewRect{X: x + 1, Y: 8, Width: width - 2, Height: height}
}

func (m *Model) clickOption(x, y int, activate bool) {
	width, rect := m.optionsLayout()
	left := rect.X - 1
	if x < left || x >= left+width {
		return
	}
	shift := 0
	if rect.Height > 0 {
		shift = rect.Height + 2
	}
	switch {
	case y == 5 && activate:
		m.options.focus = optionsFocusChooseImage
		m.activateOption()
	case y == 7+shift && activate:
		m.options.focus = optionsFocusDarkness
	case y == 8+shift:
		m.options.focus = optionsFocusDarkness
		m.options.darkness = max(0, min(95, (x-left)*95/max(1, width-1)))
	case y >= 10+shift && y <= 12+shift && activate:
		m.options.focus = optionsFocusApply + y - (10 + shift)
		m.activateOption()
	}
}

func (m Model) viewOptions() tea.View {
	width, rect := m.optionsLayout()
	selected := m.subjects.list.Styles.Title
	plain := lipgloss.NewStyle().Padding(0, 1)
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#999999"))
	label := func(index int, text string) string {
		if m.options.focus == index {
			return selected.Render(text)
		}
		return plain.Render(text)
	}
	lines := []string{selected.Render("Options"), "", "Visual", "", "Background image", label(optionsFocusChooseImage, "Choose image"), ""}
	if rect.Height > 0 {
		lines = append(lines, muted.Render("╭"+strings.Repeat("─", rect.Width)+"╮"))
		for range rect.Height {
			lines = append(lines, muted.Render("│")+strings.Repeat(" ", rect.Width)+muted.Render("│"))
		}
		lines = append(lines, muted.Render("╰"+strings.Repeat("─", rect.Width)+"╯"))
	}
	peak := m.options.darkness * (width - 1) / 95
	slider := strings.Repeat("━", peak) + "●" + strings.Repeat("─", width-peak-1)
	slider = muted.Render(slider)
	lines = append(lines, label(optionsFocusDarkness, fmt.Sprintf("Background darkness: %d%%", m.options.darkness)), slider, "",
		label(optionsFocusApply, "Apply"), label(optionsFocusRemoveBackground, "Remove background"), label(optionsFocusBack, "Back"))
	lines = append(lines, "", muted.Render(m.options.status), "", muted.Render("↑/↓/Tab: focus · ←/→: darkness"), muted.Render("Enter: activate · Q/Esc: back"))
	for i, line := range lines {
		lines[i] = strings.Repeat(" ", rect.X-1) + ansi.Truncate(line, width, "…")
	}
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}
