package thoughts

import "github.com/charmbracelet/x/ansi"

// editorView is shared by ordinary creation and browser recording.
func (m Model) editorView(status string) string {
	if m.inputWarning != "" {
		status = m.inputWarning + "\n"
	} else if m.voice.message != "" {
		status = m.voice.message + "\n"
	}
	body := "Create thought\n" + status + m.input.View()
	help := "Ctrl+S: save • Enter: newline • Esc: cancel"
	if m.VoiceOpen() {
		body += "\nSubject (optional)\n" + m.voice.query.View()
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
				body += "\n" + ansi.Truncate(prefix+label, max(1, m.voice.width), "…")
			}
		}
		action := "F8: Record"
		if m.voiceLocked() {
			action = "F8: Stop"
		}
		help = action + " • Tab: subject/text • " + help
		if m.voiceLocked() {
			help = action + " • Esc: cancel"
		}
		body += "\n" + ansi.Wrap(help, max(1, m.voice.width), "")
		return body
	}
	return body + "\n" + help
}
