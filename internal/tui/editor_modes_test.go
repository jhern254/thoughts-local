package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
	"github.com/jhern254/go-thoughts/internal/voice"
)

func TestModel_ModalRecording(t *testing.T) {
	t.Run("v records and u restores through root dispatch", func(t *testing.T) {
		m := newRootTestModel()
		m.subjects.service = subject.NewService(testutils.NewFakeSubjectStore())
		m, _ = rootUpdate(m, thoughts.BrowserVoiceSessionMsg{SessionID: "session", SpeechRecognitionAvailable: true})
		m = runModelCommand(t, m, tea.KeyPressMsg{Code: 'v', Text: "v"})
		state := m.VoiceState()
		if state.TranscriptAuthority == nil || state.RecordingStatus != "requesting" {
			t.Fatalf("v did not open a recording: %+v", state)
		}
		recording := state.TranscriptAuthority.Recording()
		original := "Fitness thought, exercise today."
		m, _ = rootUpdate(m, voice.TranscriptUpdate{Recording: recording, Revision: 1, Text: original})
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: 'v', Text: "v"})
		m, _ = rootUpdate(m, voice.RecordingEnded{Recording: recording})
		if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Exercise today.") || strings.Contains(view, original) {
			t.Fatalf("got %q, want cleaned recording", view)
		}
		m, _ = rootUpdate(m, escapeKey())
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: 'u', Text: "u"})
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: 'u', Text: "u"})
		if view := ansi.Strip(m.View().Content); strings.Count(view, original) != 1 || strings.Contains(view, "u: Restore prefix") {
			t.Fatalf("got %q, want original recording restored once", view)
		}
		m, _ = rootUpdate(m, BrowserOptionsEnabledMsg{})
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "open"})
		if m.VoiceState().CanUseVoiceKey || m.VoiceState().CanOpenThoughtDraft {
			t.Fatal("Options advertised a voice shortcut behind its controls")
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: 'v', Text: "v"})
		if m.VoiceState().RecordingStatus != "idle" {
			t.Fatal("v started recording behind Options")
		}
	})
	t.Run("empty cancellation waits for recording cleanup then returns to Events", func(t *testing.T) {
		m := newRootTestModel()
		m.subjects.service = subject.NewService(testutils.NewFakeSubjectStore())
		m, _ = rootUpdate(m, thoughts.BrowserVoiceSessionMsg{SessionID: "session", SpeechRecognitionAvailable: true})
		m = runModelCommand(t, m, tea.KeyPressMsg{Code: tea.KeyF8})
		recording := m.VoiceState().TranscriptAuthority.Recording()
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl})
		m, _ = rootUpdate(m, enterKey())
		m, _ = rootUpdate(m, thoughts.VoiceAction{Action: "start", DraftID: recording.DraftID, RecordingID: recording.RecordingID})
		if m.VoiceState().RecordingID != recording.RecordingID || m.VoiceState().RecordingStatus != "stopping" {
			t.Fatal("cancellation did not retain cleanup ownership")
		}
		m, _ = rootUpdate(m, voice.RecordingEnded{Recording: recording})
		if m.screen != screenEvents || m.VoiceState().DraftID != 0 {
			t.Fatal("empty cancellation did not finish after cleanup")
		}
	})

	for _, shortcut := range []string{"Escape", "Ctrl C", "Ctrl Q"} {
		cancel := shortcut == "Ctrl Q"
		name := shortcut + " stops without discarding"
		if cancel {
			name = "Ctrl Q confirms after cleanup"
		}
		t.Run(name, func(t *testing.T) {
			m := newRootTestModel()
			m.subjects.service = subject.NewService(testutils.NewFakeSubjectStore())
			m, _ = rootUpdate(m, thoughts.BrowserVoiceSessionMsg{SessionID: "session", SpeechRecognitionAvailable: true})
			m = runModelCommand(t, m, tea.KeyPressMsg{Code: tea.KeyF8})
			recording := m.VoiceState().TranscriptAuthority.Recording()
			m, _ = rootUpdate(m, voice.TranscriptUpdate{Recording: recording, Revision: 1, Text: "preserved words"})
			stop := tea.KeyPressMsg{Code: tea.KeyEscape}
			if shortcut == "Ctrl C" {
				stop = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
			}
			if cancel {
				stop = tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl}
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
				m, _ = rootUpdate(m, tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl})
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
