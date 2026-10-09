package thoughts

import (
	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/voice"
	"testing"
)

func TestModel_TranscriptUpdate(t *testing.T) {
	t.Run("replaces only current recording contribution and Stop freezes it", func(t *testing.T) {
		model := voiceModel(t)
		model, _ = model.Update(BrowserRecordingEnabledMsg{SessionID: "session"})
		model, _ = model.Update(tea.PasteMsg{Content: "typed 界"})
		model, _ = model.Update(VoiceAction{Action: "start", DraftID: 1})
		authority := model.VoiceState().TranscriptAuthority
		if authority == nil {
			t.Fatal("recording has no authority")
		}
		recording := authority.Recording()
		model, _ = model.Update(VoiceAction{Action: "recording", DraftID: 1, RecordingID: 1})
		for revision, text := range []string{"first", "first second"} {
			model, _ = model.Update(voice.TranscriptUpdate{Recording: recording, Revision: uint64(revision + 1), Text: text})
		}
		if got, want := model.input.Value(), "typed 界\nfirst second"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
		model, _ = model.Update(VoiceAction{Action: "stop", DraftID: 1, RecordingID: 1})
		model, _ = model.Update(voice.TranscriptUpdate{Recording: recording, Revision: 3, Text: "late"})
		model, _ = model.Update(VoiceAction{Action: "stopped", DraftID: 1, RecordingID: 1})
		if model.VoiceState().RecordingStatus != "stopping" {
			t.Fatal("browser acknowledgement bypassed server cleanup")
		}
		model, _ = model.Update(voice.RecordingEnded{Recording: recording})
		model, _ = model.Update(tea.PasteMsg{Content: " edited"})
		model, _ = model.Update(VoiceAction{Action: "start", DraftID: 1, RecordingID: 1})
		model, _ = model.Update(voice.TranscriptUpdate{Recording: recording, Revision: 4, Text: "A arrived after B"})
		if got, want := model.input.Value(), "typed 界\nfirst second edited"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
	t.Run("rejects stale identity revision and unrepresentable text", func(t *testing.T) {
		model := voiceModel(t)
		model, _ = model.Update(BrowserRecordingEnabledMsg{SessionID: "session"})
		model, _ = model.Update(VoiceAction{Action: "start", DraftID: 1})
		recording := model.VoiceState().TranscriptAuthority.Recording()
		model, _ = model.Update(VoiceAction{Action: "recording", DraftID: 1, RecordingID: 1})
		model, _ = model.Update(voice.TranscriptUpdate{Recording: recording, Revision: 1, Text: "accepted"})
		staleSession := recording
		staleSession.SessionID = "old"
		staleDraft := recording
		staleDraft.DraftID++
		for _, update := range []voice.TranscriptUpdate{
			{Recording: recording, Revision: 1, Text: "duplicate"},
			{Recording: staleSession, Revision: 2, Text: "old session"},
			{Recording: staleDraft, Revision: 2, Text: "old draft"},
			{Recording: recording, Revision: 2, Text: "\x1b[31m"},
		} {
			model, _ = model.Update(update)
		}
		if model.input.Value() != "accepted" {
			t.Fatal("invalid update changed draft")
		}
		model, _ = model.Update(key(tea.KeyEsc))
		if model.VoiceOpen() {
			t.Fatal("cancel retained draft")
		}
		model, _ = model.Update(voice.TranscriptUpdate{Recording: recording, Revision: 3, Text: "late"})
		if model.VoiceOpen() {
			t.Fatal("late update revived draft")
		}
	})
}
