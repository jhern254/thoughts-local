package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
)

type EditorPreferencesService interface {
	Load(context.Context) (data.EditorPreferences, error)
	Save(context.Context, data.EditorPreferences) error
}
type editorPreferencesState struct {
	service     EditorPreferencesService
	preferences data.EditorPreferences
	open        bool
	pending     bool
	focus       int
	request     uint64
	status      string
}
type editorPreferencesResult struct {
	request     uint64
	preferences data.EditorPreferences
	saved       bool
	err         error
}

func (m *Model) ConfigureEditorPreferences(service EditorPreferencesService) {
	if service == nil {
		return
	}
	m.editorPreferences.service = service
	m.editorPreferences.preferences = data.DefaultEditorPreferences()
	m.editorPreferences.request = 1
}
func (m Model) readEditorPreferences() tea.Cmd {
	service, ctx, request := m.editorPreferences.service, m.ctx, m.editorPreferences.request
	if service == nil {
		return nil
	}
	return func() tea.Msg {
		if ctx.Err() != nil {
			return nil
		}
		preferences, err := service.Load(ctx)
		if ctx.Err() != nil {
			return nil
		}
		return editorPreferencesResult{request: request, preferences: preferences, err: err}
	}
}
func (m Model) receiveEditorPreferences(reply editorPreferencesResult) (tea.Model, tea.Cmd) {
	if reply.request != m.editorPreferences.request {
		return m, nil
	}
	m.editorPreferences.pending = false
	if reply.err != nil {
		m.editorPreferences.status = "Could not load editor preferences. Reopen Editor to retry."
		if reply.saved {
			m.editorPreferences.status = "Could not save editor preferences. Try Save again."
		}
		return m, nil
	}
	m.editorPreferences.preferences = reply.preferences
	m.thoughts.SetEditorHighlight(reply.preferences.HighlightCurrentLine)
	m.editorPreferences.status = ""
	if reply.saved {
		m.editorPreferences.status = "Saved."
	}
	return m, nil
}
func (m Model) openEditorPreferences() (tea.Model, tea.Cmd) {
	m.editorPreferences.open = true
	m.editorPreferences.focus = 0
	m.editorPreferences.request++
	m.editorPreferences.pending = m.editorPreferences.service != nil
	m.editorPreferences.status = ""
	if m.editorPreferences.service == nil {
		m.editorPreferences.status = "Editor preferences are unavailable."
	}
	return m, m.readEditorPreferences()
}
func (m Model) updateEditorPreferences(message tea.Msg) (tea.Model, tea.Cmd) {
	if m.editorPreferences.pending {
		return m, nil
	}
	if click, ok := message.(tea.MouseClickMsg); ok && click.Button == tea.MouseLeft && click.Y >= 2 && click.Y <= 5 {
		m.editorPreferences.focus = click.Y - 2
		message = tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	key, ok := message.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc", "q":
		m.editorPreferences.open = false
		return m, nil
	case "tab", "down":
		m.editorPreferences.focus = (m.editorPreferences.focus + 1) % 4
	case "shift+tab", "up":
		m.editorPreferences.focus = (m.editorPreferences.focus + 3) % 4
	case "left", "right", "enter", "space":
		switch m.editorPreferences.focus {
		case 0:
			m.editorPreferences.status = ""
			m.editorPreferences.preferences.HighlightCurrentLine = !m.editorPreferences.preferences.HighlightCurrentLine
		case 1:
			m.editorPreferences.status = ""
			step := 500
			if key.String() == "left" {
				step = -500
			}
			m.editorPreferences.preferences.SpeechPauseMS = max(0, min(10000, m.editorPreferences.preferences.SpeechPauseMS+step))
		case 2:
			if key.String() != "enter" && key.String() != "space" {
				return m, nil
			}
			if m.editorPreferences.service == nil {
				return m, nil
			}
			m.editorPreferences.request++
			m.editorPreferences.pending = true
			service, ctx, preferences, request := m.editorPreferences.service, m.ctx, m.editorPreferences.preferences, m.editorPreferences.request
			return m, func() tea.Msg {
				if ctx.Err() != nil {
					return nil
				}
				err := service.Save(ctx, preferences)
				if ctx.Err() != nil {
					return nil
				}
				return editorPreferencesResult{request: request, preferences: preferences, saved: true, err: err}
			}
		case 3:
			if key.String() == "enter" || key.String() == "space" {
				m.editorPreferences.open = false
			}
		}
	}
	return m, nil
}
func (m Model) viewEditorPreferences() tea.View {
	highlight := "Off"
	if m.editorPreferences.preferences.HighlightCurrentLine {
		highlight = "On"
	}
	pause := "Off"
	if ms := m.editorPreferences.preferences.SpeechPauseMS; ms > 0 {
		pause = fmt.Sprintf("%.1f seconds", float64(ms)/1000)
	}
	rows := []string{"Editor", ""}
	labels := []string{"Highlight current line: " + highlight, "Newline after speech pause: " + pause, "Save", "Back"}
	if m.width < 40 {
		labels[0] = "Line highlight: " + highlight
		labels[1] = "Pause newline: " + pause
	}
	for index, row := range labels {
		if index == m.editorPreferences.focus {
			row = m.subjects.list.Styles.Title.Render(row)
		}
		rows = append(rows, row)
	}
	status := m.editorPreferences.status
	if m.editorPreferences.pending {
		status = "Working…"
	}
	rows = append(rows, "", status, "↑/↓/Tab: focus · ←/→: value", "Enter: activate · Esc: back")
	for i, row := range rows {
		rows[i] = ansi.Truncate(row, max(1, m.width), "…")
	}
	view := tea.NewView(strings.Join(rows, "\n"))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}
