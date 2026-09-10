package thoughts

import (
	"strings"
	"unicode"
	"unicode/utf8"

	keybinding "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
)

// Bubbles v2.2.1 textarea has an unexported 10,000 logical-line insertion
// ceiling even with its configurable limits disabled. This is an editor
// capability, NOT a thought validation rule. Reject whole edits before the
// widget can truncate input or delete a selection. Recheck on widget upgrades.
const textareaMaxLines = 10_000

type clipboardResult struct {
	request uint64
	content string
	err     error
}

func (m Model) updateInput(msg tea.Msg) (Model, tea.Cmd) {
	if m.loading || !m.input.Focused() {
		return m, nil
	}
	var text string
	switch message := msg.(type) {
	case clipboardResult:
		if message.request != m.request {
			return m, nil
		}
		if message.err != nil {
			m.inputWarning = "Could not read the clipboard. Draft unchanged."
			return m, nil
		}
		return m.updateInput(tea.PasteMsg{Content: message.content})
	case tea.PasteMsg:
		if message.Content == "" {
			return m, nil
		}
		text = message.Content
	case tea.KeyPressMsg:
		switch {
		case keybinding.Matches(message, m.input.KeyMap.Paste):
			// Do not use textarea.Paste: its private result bypasses validation and
			// its shortcut deletes the selection before the clipboard read succeeds.
			request := m.request
			return m, func() tea.Msg {
				content, err := clipboard.ReadAll()
				return clipboardResult{request: request, content: content, err: err}
			}
		case keybinding.Matches(message, m.input.KeyMap.InsertNewline):
			text = "\n"
		default:
			text = message.Text
		}
	}
	if text != "" {
		// The pinned widget rewrites CR/tabs and drops control and replacement
		// runes. Refuse those operations rather than silently changing content.
		if !utf8.ValidString(text) {
			m.inputWarning = "Input rejected: invalid UTF-8 cannot be preserved by this editor. Draft unchanged."
			return m, nil
		}
		for _, r := range text {
			if r == utf8.RuneError || (r != '\n' && unicode.IsControl(r)) {
				m.inputWarning = "Input rejected: this editor cannot preserve tabs, carriage returns, other control characters, or U+FFFD. Draft unchanged."
				return m, nil
			}
		}
		lines := m.input.LineCount() - strings.Count(m.input.SelectedText(), "\n") + strings.Count(text, "\n")
		if lines > textareaMaxLines {
			m.inputWarning = "Input rejected: this editor supports at most 10,000 lines. Draft unchanged."
			return m, nil
		}
		m.inputWarning = ""
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}
