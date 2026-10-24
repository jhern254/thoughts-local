package thoughts

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestModel_EditorModes(t *testing.T) {
	t.Run("Ctrl C leaves Insert and Ctrl D no longer cancels", func(t *testing.T) {
		m := thoughtEditor(t)
		m, _ = m.Update(tea.PasteMsg{Content: "keep draft"})
		m, _ = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		m, _ = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
		if !m.Creating() || !m.normalMode || m.discardPrompt || m.input.Value() != "keep draft" {
			t.Fatal("mode shortcut discarded or changed draft")
		}
	})
	t.Run("mode labels use distinct padded color blocks", func(t *testing.T) {
		m := thoughtEditor(t)
		insert := strings.Split(m.View(), "\n")[0]
		m, _ = m.Update(key(tea.KeyEscape))
		normal := strings.Split(m.View(), "\n")[0]
		if !strings.Contains(insert, "48;5;36") || !strings.Contains(normal, "48;5;62") {
			t.Fatalf("mode badges lack distinct styles: insert=%q normal=%q", insert, normal)
		}
		if !strings.HasSuffix(ansi.Strip(insert), " INSERT ") || !strings.HasSuffix(ansi.Strip(normal), " NORMAL ") {
			t.Fatalf("mode badges lack block padding: insert=%q normal=%q", insert, normal)
		}
	})
	t.Run("Escape preserves draft and normal mode ignores typing and paste", func(t *testing.T) {
		m := thoughtEditor(t)
		m, _ = m.Update(tea.PasteMsg{Content: "hello界"})
		m, _ = m.Update(key(tea.KeyEscape))
		for _, msg := range []tea.Msg{key(tea.KeyEscape), tea.KeyPressMsg(tea.Key{Code: 'z', Text: "z"}), tea.PasteMsg{Content: "unexpected"}} {
			m, _ = m.Update(msg)
		}
		if !m.Creating() || m.input.Value() != "hello界" || !strings.Contains(m.View(), "NORMAL") {
			t.Fatal("normal mode lost or changed draft")
		}
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'i', Text: "i"}))
		m, _ = m.Update(tea.PasteMsg{Content: "!"})
		if m.input.Value() != "hello界!" {
			t.Fatalf("got %q", m.input.Value())
		}
	})
	t.Run("normal motions and open line reuse the editor", func(t *testing.T) {
		m := thoughtEditor(t)
		m, _ = m.Update(tea.PasteMsg{Content: "first\nsecond"})
		m, _ = m.Update(key(tea.KeyEscape))
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'k', Text: "k"}))
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'o', Text: "o"}))
		m, _ = m.Update(tea.PasteMsg{Content: "middle"})
		if m.input.Value() != "first\nmiddle\nsecond" {
			t.Fatalf("got %q", m.input.Value())
		}
	})
	t.Run("Ctrl Q confirms nonempty draft with keep selected", func(t *testing.T) {
		m := thoughtEditor(t)
		m, _ = m.Update(tea.PasteMsg{Content: "keep"})
		cancel := tea.KeyPressMsg(tea.Key{Code: 'q', Mod: tea.ModCtrl})
		m, _ = m.Update(cancel)
		if !strings.Contains(m.View(), "Discard draft?") {
			t.Fatal("missing confirmation")
		}
		m, _ = m.Update(key(tea.KeyEnter))
		if !m.Creating() || m.input.Value() != "keep" {
			t.Fatal("default discarded draft")
		}
		m, _ = m.Update(cancel)
		m, _ = m.Update(key(tea.KeyTab))
		m, _ = m.Update(key(tea.KeyEnter))
		if !m.Browsing() || m.input.Value() != "" {
			t.Fatal("confirmed cancellation did not clear draft")
		}
	})
}

func TestModel_ModalVoiceKeys(t *testing.T) {
	t.Run("Insert and Subject input keep v and u literal", func(t *testing.T) {
		m := voiceModel(t)
		for _, letter := range "vu" {
			m, _ = m.Update(tea.KeyPressMsg{Code: letter, Text: string(letter)})
		}
		if m.input.Value() != "vu" || m.VoiceState().RecordingID != 0 || m.VoiceState().CanUseVoiceKey {
			t.Fatalf("Insert text=%q state=%+v", m.input.Value(), m.VoiceState())
		}
		m, _ = m.Update(key(tea.KeyEscape))
		m, _ = m.Update(key(tea.KeyTab))
		for _, letter := range "vu" {
			m, _ = m.Update(tea.KeyPressMsg{Code: letter, Text: string(letter)})
		}
		if m.voice.query.Value() != "vu" || m.VoiceState().RecordingID != 0 || m.VoiceState().CanUseVoiceKey {
			t.Fatalf("Subject text=%q state=%+v", m.voice.query.Value(), m.VoiceState())
		}
	})
	t.Run("Normal v starts and stops without inserting text", func(t *testing.T) {
		m := voiceModel(t)
		m, _ = m.Update(key(tea.KeyEscape))
		if !m.VoiceState().CanUseVoiceKey {
			t.Fatal("Normal mode did not enable browser voice key preparation")
		}
		m, _ = m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
		if m.VoiceState().RecordingStatus != "requesting" {
			t.Fatalf("got %q, want requesting", m.VoiceState().RecordingStatus)
		}
		m, _ = m.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
		m, _ = m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
		if m.VoiceState().RecordingStatus != "stopping" || m.input.Value() != "" {
			t.Fatalf("text=%q state=%+v, want empty text and stopping", m.input.Value(), m.VoiceState())
		}
	})
}

func TestModel_EditorOwnership(t *testing.T) {
	t.Run("clipboard queued before Normal cannot arrive after returning to Insert", func(t *testing.T) {
		m := thoughtEditor(t)
		request := m.request
		m, _ = m.Update(key(tea.KeyEscape))
		m, _ = m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
		m, _ = m.Update(clipboardResult{request: request, content: "late"})
		if m.input.Value() != "" {
			t.Fatal("late clipboard inserted text")
		}
	})
	t.Run("open line at the ceiling preserves cursor and draft", func(t *testing.T) {
		m := thoughtEditor(t)
		m.input.SetValue(strings.Repeat("line\n", 9999) + "end")
		m.input.CursorStart()
		m, _ = m.Update(key(tea.KeyEscape))
		line, column := m.input.Line(), m.input.Column()
		before := m.input.Value()
		m, _ = m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
		if m.input.Value() != before || m.input.Line() != line || m.input.Column() != column || !strings.Contains(m.inputWarning, "10,000") {
			t.Fatal("rejected open-line changed draft/cursor")
		}
	})
	t.Run("cursor remains steady with independently toggled row highlight", func(t *testing.T) {
		m := thoughtEditor(t)
		if m.input.Styles().Cursor.Blink || m.input.Styles().Focused.CursorLine.GetBackground() != (lipgloss.NoColor{}) {
			t.Fatal("wrong editor defaults")
		}
		m.SetEditorHighlight(true)
		if m.input.Styles().Cursor.Blink || m.input.Styles().Focused.CursorLine.GetBackground() == (lipgloss.NoColor{}) {
			t.Fatal("highlight changed cursor behavior")
		}
	})
}

func TestModel_EditorLayout(t *testing.T) {
	for _, normal := range []bool{false, true} {
		t.Run("normal and narrow layouts reserve space for controls", func(t *testing.T) {
			m := thoughtEditor(t)
			m.Resize(32, 12)
			m, _ = m.Update(tea.PasteMsg{Content: strings.Repeat("wrapped 界 text ", 30)})
			if normal {
				m, _ = m.Update(key(tea.KeyEscape))
			}
			for _, confirm := range []bool{false, true} {
				if confirm {
					m, _ = m.Update(tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl})
				}
				view := m.View()
				if rows := strings.Count(view, "\n") + 1; rows > 12 {
					t.Fatalf("got %d rows, want <=12", rows)
				}
				for _, line := range strings.Split(view, "\n") {
					if width := ansi.StringWidth(line); width > 32 {
						t.Fatalf("got width %d, want <=32", width)
					}
				}
			}
		})
	}
}

func TestModel_EditorScrollAnchor(t *testing.T) {
	t.Run("moving upward inside the viewport keeps its first visible line", func(t *testing.T) {
		m := voiceModel(t)
		m.Resize(32, 16)
		var body strings.Builder
		for i := 0; i < 30; i++ {
			fmt.Fprintf(&body, "line %02d\n", i)
		}
		m, _ = m.Update(tea.PasteMsg{Content: strings.TrimSuffix(body.String(), "\n")})
		before := strings.Split(ansi.Strip(m.View()), "\n")[1]
		m, _ = m.Update(key(tea.KeyUp))
		after := strings.Split(ansi.Strip(m.View()), "\n")[1]
		if after != before {
			t.Fatalf("first visible row changed from %q to %q while cursor stayed inside viewport", before, after)
		}
	})
}
