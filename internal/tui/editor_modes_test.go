package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
	"github.com/jhern254/go-thoughts/internal/voice"
)

func TestModel_ModalRecording(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		name := "Escape stops without discarding"
		if cancel {
			name = "Ctrl D confirms after cleanup"
		}
		t.Run(name, func(t *testing.T) {
			m := newRootTestModel()
			m.subjects.service = subject.NewService(testutils.NewFakeSubjectStore())
			m, _ = rootUpdate(m, thoughts.BrowserVoiceSessionMsg{SessionID: "session", SpeechRecognitionAvailable: true})
			m = runModelCommand(t, m, tea.KeyPressMsg{Code: tea.KeyF8})
			recording := m.VoiceState().TranscriptAuthority.Recording()
			m, _ = rootUpdate(m, voice.TranscriptUpdate{Recording: recording, Revision: 1, Text: "preserved words"})
			stop := tea.KeyPressMsg{Code: tea.KeyEscape}
			if cancel {
				stop = tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}
			}
			for range 3 {
				m, _ = rootUpdate(m, stop)
			}
			if m.VoiceState().RecordingID != recording.RecordingID || m.VoiceState().RecordingStatus != "stopping" {
				t.Fatal("repeated stop replaced ownership")
			}
			m, _ = rootUpdate(m, voice.TranscriptUpdate{Recording: recording, Revision: 2, Text: "late words"})
			m, _ = rootUpdate(m, tea.PasteMsg{Content: "locked input"})
			m, _ = rootUpdate(m, enterKey())
			m, _ = rootUpdate(m, voice.RecordingEnded{Recording: recording})
			view := m.View().Content
			if !strings.Contains(view, "preserved words") || strings.Contains(view, "late words") || strings.Contains(view, "locked input") || !strings.Contains(view, "NORMAL") {
				t.Fatal("stop lost draft or accepted late edits")
			}
			if cancel {
				m, _ = rootUpdate(m, enterKey())
				if !m.thoughts.Creating() {
					t.Fatal("default confirmation discarded draft")
				}
				m, _ = rootUpdate(m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
				m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyTab})
				m, _ = rootUpdate(m, enterKey())
				if m.screen != screenEvents || m.VoiceState().DraftID != 0 {
					t.Fatal("confirmed cancel did not return to Events")
				}
			} else {
				m, _ = rootUpdate(m, tea.KeyPressMsg{Code: 'i', Text: "i"})
				m, _ = rootUpdate(m, tea.PasteMsg{Content: " edited"})
				m = runModelCommand(t, m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
				if !m.thoughts.ShowingDetail() {
					t.Fatal("stopped draft cannot be saved")
				}
			}
		})
	}
}
