package thoughts

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m *Model) SetEditorHighlight(enabled bool) {
	styles := m.input.Styles()
	styles.Cursor.Blink = false
	styles.Cursor.Color = lipgloss.Color("#E9D5FF")
	styles.Focused.CursorLine = lipgloss.NewStyle()
	if enabled {
		styles.Focused.CursorLine = lipgloss.NewStyle().Background(lipgloss.Color("#352541"))
	}
	m.input.SetStyles(styles)
}

func (m Model) updateNormalInput(message tea.Msg) (Model, tea.Cmd) {
	key, ok := message.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	var movement rune
	switch key.String() {
	case "i":
		m.normalMode = false
		return m, nil
	case "a":
		m.input.SetCursorColumn(m.input.Column() + 1)
		m.normalMode = false
		return m, nil
	case "o":
		// Refuse before moving the cursor, just as rejected Insert edits do.
		if m.input.LineCount() >= textareaMaxLines {
			m.inputWarning = "Input rejected: this editor supports at most 10,000 lines. Draft unchanged."
			return m, nil
		}
		m.input.ClearSelection()
		m.input.CursorEnd()
		m.normalMode = false
		return m.updateInput(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	case "h", "left":
		movement = tea.KeyLeft
	case "j", "down":
		movement = tea.KeyDown
	case "k", "up":
		movement = tea.KeyUp
	case "l", "right":
		movement = tea.KeyRight
	case "home":
		movement = tea.KeyHome
	case "end":
		movement = tea.KeyEnd
	default:
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(tea.KeyPressMsg(tea.Key{Code: movement}))
	return m, cmd
}

func (m Model) updateDiscardPrompt(key tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.voiceLocked() {
		return m, nil
	}
	switch key.String() {
	case "tab", "shift+tab", "left", "right":
		m.discardSelected = !m.discardSelected
	case "esc":
		m.discardPrompt = false
	case "enter":
		if m.discardSelected {
			return m.discardDraft()
		}
		m.discardPrompt = false
	}
	return m, nil
}

func (m Model) discardDraft() (Model, tea.Cmd) {
	if m.VoiceOpen() {
		m.subjectID = m.voice.originalSubjectID
	}
	m.stopVoice()
	m.stopClipboard()
	m.request++
	m.input.Blur()
	m.input.Reset()
	m.draftObservedAt = time.Time{}
	m.err = nil
	m.inputWarning = ""
	m.normalMode, m.discardPrompt, m.discardSelected = false, false, false
	m.screen = browse
	return m, nil
}
