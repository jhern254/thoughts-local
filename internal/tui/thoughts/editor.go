package thoughts

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// editorView reserves actual wrapped control rows before sizing the text area.
// Rendering a resized copy keeps the cursor visible without changing the draft.
func (m Model) editorView(status string) string {
	width := max(1, m.editorWidth)
	if m.inputWarning != "" {
		status = m.inputWarning
	} else if m.voice.message != "" {
		status = m.voice.message
	}
	mode := "INSERT"
	modeStyle := m.list.Styles.Title.Background(lipgloss.Color("36"))
	if m.normalMode && !m.voice.subjectFocused {
		mode = "NORMAL"
		modeStyle = m.list.Styles.Title
	}
	heading := ansi.Truncate("Create thought ·"+modeStyle.Padding(0, 1).Render(mode), width, "…")
	if status = strings.TrimSuffix(status, "\n"); status != "" {
		heading += "\n" + ansi.Wrap(status, width, "")
	}
	footer := m.editorControls()
	textArea := m.input
	textArea.SetHeight(max(1, m.editorHeight-(strings.Count(heading, "\n")+1)-(strings.Count(footer, "\n")+1)))
	return heading + "\n" + textArea.View() + "\n" + footer
}

func (m Model) editorControls() string {
	width := max(1, m.editorWidth)
	if m.discardPrompt {
		if m.voiceLocked() {
			return "Stopping recording…"
		}
		choices := m.list.Styles.Title.Render("Keep editing") + "   Discard"
		if m.discardSelected {
			choices = "Keep editing   " + m.list.Styles.Title.Render("Discard")
		}
		return ansi.Wrap("Discard draft?\n"+choices+"\nTab/←/→: choose · Enter: confirm · Esc: keep", width, "")
	}
	help := "Ctrl+S: save • Enter: newline • Esc/Ctrl+C: normal • Ctrl+Q: cancel"
	if m.normalMode && !m.voice.subjectFocused {
		help = "h/j/k/l/Arrows: move • i/a/o: insert • Enter/Ctrl+S: save • Ctrl+Q: cancel"
	}
	if m.voice.subjectFocused {
		help = "↑/↓: subjects • Enter: select/create • Esc: text • Ctrl+S: save • Ctrl+Q: cancel"
	}
	if !m.VoiceOpen() {
		return ansi.Wrap(help, width, "")
	}
	body := m.voiceSubjectLabel() + "\n" + m.voice.query.View()
	if m.voice.subjectFocused {
		items := m.matchingVoiceSubjects()
		visible := max(1, min(4, m.voice.height-8))
		top := max(0, m.voice.row-visible+1)
		for row := top; row < min(top+visible, len(items)+1); row++ {
			label := "Create subject…"
			if row > 0 {
				label = items[row-1].SubjectName
			}
			prefix := "    "
			if row == m.voice.row {
				prefix = "  > "
			}
			body += "\n" + ansi.Truncate(prefix+label, width, "…")
		}
	}
	voiceHint, restoreHint := "v: Record", "u: Restore prefix"
	if !m.normalMode || m.voice.subjectFocused {
		voiceHint, restoreHint = "Esc then "+voiceHint, "Esc then "+restoreHint
	}
	help = voiceHint + " • Tab: subject/text • " + help
	if m.canRestoreVoicePrefix() {
		help += " • " + restoreHint
	}
	if m.voiceLocked() {
		help = "v: Stop • Esc/Ctrl+C: stop • Ctrl+Q: cancel"
	}
	return body + "\n" + ansi.Wrap(help, width, "")
}
