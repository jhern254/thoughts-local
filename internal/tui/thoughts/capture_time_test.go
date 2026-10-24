package thoughts

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
)

func TestModel_CaptureTime(t *testing.T) {
	for _, entry := range []string{"subject", "misc", "browse thoughts", "quick draft"} {
		t.Run(entry+" retains opening time through a failed save and retry", func(t *testing.T) {
			store := testutils.NewFakeThoughtStore()
			m := New(t.Context(), "u", thought.NewService(store), logging.Nop())
			openedAt := time.Date(2026, 10, 20, 6, 59, 50, 0, time.UTC)
			now := openedAt
			m.now = func() time.Time { return now }
			switch entry {
			case "subject":
				m = runBrowseThoughtsCommand(m, m.Open(7))
			case "misc":
				m = runBrowseThoughtsCommand(m, m.OpenUnassigned())
			case "browse thoughts":
				m = runBrowseThoughtsCommand(m, m.OpenBrowseThoughtsView(store))
			case "quick draft":
				m.OpenVoice(1, &voiceSubjects{})
			}
			if entry != "quick draft" {
				m, _ = m.Update(key(tea.KeyEnter))
			}
			if !m.Creating() || !m.input.Focused() {
				t.Fatal("entry did not focus a new thought draft")
			}
			now = now.Add(5 * time.Minute)
			save := tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl})
			m, cmd := m.Update(save)
			m, _ = m.Update(cmd())
			if m.err == nil || !m.Creating() || !m.input.Focused() {
				t.Fatal("empty save did not preserve the focused draft")
			}
			m, _ = m.Update(tea.PasteMsg{Content: "thought begun before midnight"})
			m, cmd = m.Update(save)
			now = now.Add(time.Minute)
			result := cmd().(Result)
			if result.err != nil {
				t.Fatal(result.err)
			}
			if got := result.item.ObservedAt; !got.Equal(openedAt) {
				t.Fatalf("observed time got %v, want draft opening %v", got, openedAt)
			}
			if !result.item.CreatedAt.After(openedAt) {
				t.Fatalf("created time got %v, want persistence time after %v", result.item.CreatedAt, openedAt)
			}
		})
	}

	t.Run("voice restarts and subject creation preserve the original capture time", func(t *testing.T) {
		m := New(t.Context(), "u", thought.NewService(testutils.NewFakeThoughtStore()), logging.Nop())
		openedAt := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
		now := openedAt
		m.now = func() time.Time { return now }
		m, _ = m.Update(key(tea.KeyEnter))
		now = now.Add(time.Minute)
		m.EnableVoice(1, &voiceSubjects{}, "")
		m.speechRecognitionAvailable = true
		for recordingID := uint64(0); recordingID < 2; recordingID++ {
			m, _ = m.Update(VoiceAction{Action: "start", DraftID: 1, RecordingID: recordingID})
			now = now.Add(time.Minute)
			m, _ = m.Update(VoiceAction{Action: "stopped", DraftID: 1, RecordingID: recordingID + 1})
		}
		m, _ = m.Update(tea.PasteMsg{Content: "voice draft"})
		m, _ = m.Update(key(tea.KeyTab))
		m, cmd := m.Update(key(tea.KeyEnter))
		request := cmd().(VoiceSubjectRequest)
		now = now.Add(time.Minute)
		m.ReturnFromVoiceSubject(request, &data.Subject{SubjectID: 7, SubjectName: "New subject"})
		m, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
		result := cmd().(Result)
		if result.err != nil {
			t.Fatal(result.err)
		}
		if got := result.item.ObservedAt; !got.Equal(openedAt) {
			t.Fatalf("observed time got %v, want original capture %v", got, openedAt)
		}
	})

	t.Run("new drafts get new times while queued saves retain their own time", func(t *testing.T) {
		m := New(t.Context(), "u", thought.NewService(testutils.NewFakeThoughtStore()), logging.Nop())
		now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
		m.now = func() time.Time { return now }
		m.OpenVoice(1, &voiceSubjects{})
		m, _ = m.Update(tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl})
		now = now.Add(time.Minute)
		openedAt := now
		m.OpenVoice(2, &voiceSubjects{})
		m, _ = m.Update(tea.PasteMsg{Content: "second draft"})
		m, pending := m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
		now = now.Add(time.Minute)
		m.OpenVoice(3, &voiceSubjects{})
		result := pending().(Result)
		if result.err != nil {
			t.Fatal(result.err)
		}
		if got := result.item.ObservedAt; !got.Equal(openedAt) {
			t.Fatalf("queued save time got %v, want second draft %v", got, openedAt)
		}
		m, _ = m.Update(result)
		if !m.Creating() || m.VoiceState().DraftID != 3 {
			t.Fatal("old save replaced the new draft")
		}
	})
}
