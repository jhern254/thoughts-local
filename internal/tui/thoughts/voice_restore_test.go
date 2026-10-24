package thoughts

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/voice"
)

func TestModel_RestoreVoicePrefix(t *testing.T) {
	for _, text := range []string{
		"fitness thought, exercised today.",
		"fitness thought, first sentence.\nSecond sentence.",
		"fitness thoughts, first sentence.\nSecond sentence.",
		"meditation thought — étudier 日本語.",
		"Programming in Go thought: practiced interfaces.",
		"Fitness thoughts: try shorter workouts",
		"meditation THOUGHTS：  étudier 日本語.",
		"Programming in Go thoughts: practiced interfaces.",
	} {
		for _, focus := range []string{"text", "subject"} {
			t.Run(text+"/"+focus, func(t *testing.T) {
				m, load, _ := discoveryModel(t)
				m, _ = m.Update(load())
				stamp := m.draftObservedAt
				m, recording := startDiscoveryRecording(m)
				m = transcript(m, recording, 1, text)
				m, _ = m.Update(voice.RecordingEnded{Recording: recording})
				if m.input.Value() == text || !strings.Contains(m.View(), "u: Restore prefix") {
					t.Fatal("cleanup did not offer restore")
				}
				if focus == "subject" {
					m, _ = m.Update(key(tea.KeyTab))
				}
				m, _ = m.Update(key(tea.KeyF9))
				if m.input.Value() != text || m.subjectID != nil || m.voice.query.Value() != "" || m.draftObservedAt != stamp {
					t.Fatalf("restored text=%q subject=%v query=%q", m.input.Value(), m.subjectID, m.voice.query.Value())
				}
				m, _ = m.Update(key(tea.KeyF9))
				m.refreshSpokenSubject(true)
				if m.input.Value() != text || strings.Contains(m.View(), "u: Restore prefix") {
					t.Fatal("restore duplicated or re-cleaned prefix")
				}
				_, save := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
				if save == nil {
					t.Fatal("Misc save blocked")
				}
				result := save().(Result)
				if result.err != nil || result.item.SubjectID != nil || result.item.Thought != text {
					t.Fatalf("save=%+v error=%v", result.item, result.err)
				}
			})
		}
	}
	t.Run("edits and explicit Subject survive restore", func(t *testing.T) {
		m, load, _ := discoveryModel(t)
		m, _ = m.Update(load())
		m, recording := startDiscoveryRecording(m)
		m = transcript(m, recording, 1, "fitness thoughts, exercised today.")
		m, _ = m.Update(voice.RecordingEnded{Recording: recording})
		m, _ = m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
		m, _ = m.Update(tea.PasteMsg{Content: " More exercise."})
		m, _ = m.Update(key(tea.KeyTab))
		m, _ = m.Update(key(tea.KeyDown))
		m, _ = m.Update(key(tea.KeyEnter))
		m, _ = m.Update(key(tea.KeyF9))
		if m.input.Value() != "fitness thoughts, Exercised today. More exercise." || m.subjectID == nil || *m.subjectID != 8 {
			t.Fatalf("text=%q subject=%v", m.input.Value(), m.subjectID)
		}
	})
	t.Run("delayed snapshot cleanup is restorable and cannot run again", func(t *testing.T) {
		m, load, _ := discoveryModel(t)
		reply := load()
		m, r := startDiscoveryRecording(m)
		text := "Fitness thought, exercise."
		m = transcript(m, r, 1, text)
		m, _ = m.Update(voice.RecordingEnded{Recording: r})
		m, _ = m.Update(reply)
		m, _ = m.Update(key(tea.KeyF9))
		m, _ = m.Update(reply)
		m, _ = m.Update(voice.TranscriptUpdate{Recording: r, Revision: 99, Text: "Fitness thought changed"})
		m, _ = m.Update(voice.RecordingEnded{Recording: r})
		if m.input.Value() != text || m.subjectID != nil || m.voice.query.Value() != "" {
			t.Fatalf("text=%q subject=%v", m.input.Value(), m.subjectID)
		}
	})
	t.Run("created Subject stays selected", func(t *testing.T) {
		m, load, _ := discoveryModel(t)
		m, _ = m.Update(load())
		m, r := startDiscoveryRecording(m)
		m = transcript(m, r, 1, "meditation thought, breathe.")
		m, _ = m.Update(voice.RecordingEnded{Recording: r})
		m, _ = m.Update(key(tea.KeyTab))
		var cmd tea.Cmd
		m, cmd = m.Update(key(tea.KeyEnter))
		request := cmd().(VoiceSubjectRequest)
		m, _ = m.Update(key(tea.KeyF9))
		if m.input.Value() != "Breathe." {
			t.Fatal("restored while creation owns draft")
		}
		m.ReturnFromVoiceSubject(request, &data.Subject{SubjectID: 30, SubjectName: "meditation"})
		m, _ = m.Update(key(tea.KeyF9))
		if m.subjectID == nil || *m.subjectID != 30 || m.input.Value() != "meditation thought, breathe." {
			t.Fatalf("text=%q subject=%v", m.input.Value(), m.subjectID)
		}
	})
	for _, action := range []string{"record again", "cancel", "reset", "save"} {
		t.Run("retire/"+action, func(t *testing.T) {
			m, load, _ := discoveryModel(t)
			m, _ = m.Update(load())
			m, r := startDiscoveryRecording(m)
			m = transcript(m, r, 1, "Fitness thought, exercise.")
			m, _ = m.Update(key(tea.KeyF9))
			if m.input.Value() != "Fitness thought, exercise." {
				t.Fatal("F9 changed active recording")
			}
			m, _ = m.Update(VoiceAction{Action: "stop", DraftID: 1, RecordingID: r.RecordingID})
			m, _ = m.Update(key(tea.KeyF9))
			m, _ = m.Update(voice.RecordingEnded{Recording: r})
			switch action {
			case "record again":
				m, r = startDiscoveryRecording(m)
				m = transcript(m, r, 1, "Literal addition.")
				m, _ = m.Update(voice.RecordingEnded{Recording: r})
			case "cancel":
				m, _ = m.Update(tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl})
				m, _ = m.Update(key(tea.KeyTab))
				m, _ = m.Update(key(tea.KeyEnter))
			case "reset":
				m.Reset()
			case "save":
				var save tea.Cmd
				m, save = m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
				m, _ = m.Update(key(tea.KeyF9))
				if m.input.Value() != "Exercise." {
					t.Fatal("F9 changed saving draft")
				}
				m, _ = m.Update(save())
			}
			before := m.input.Value()
			m, _ = m.Update(key(tea.KeyF9))
			if m.input.Value() != before || strings.Contains(m.View(), "u: Restore prefix") {
				t.Fatal("retired cleanup survived")
			}
		})
	}
	t.Run("oversized restoration preserves draft and remains retryable", func(t *testing.T) {
		m, load, _ := discoveryModel(t)
		m, _ = m.Update(load())
		m, r := startDiscoveryRecording(m)
		m = transcript(m, r, 1, "Fitness thought, exercise.")
		m, _ = m.Update(voice.RecordingEnded{Recording: r})
		body := strings.Repeat("界", 1_000_000)
		m.input.SetValue(body)
		m, _ = m.Update(key(tea.KeyF9))
		if m.input.Value() != body || m.inputWarning == "" {
			t.Fatal("oversized restore changed draft or lacked feedback")
		}
		m.input.SetValue("Edited.")
		m, _ = m.Update(key(tea.KeyF9))
		if m.input.Value() != "Fitness thought, Edited." {
			t.Fatal("rejected restore lost retry")
		}
	})
}

func TestModel_RestoreVoicePrefixAvailability(t *testing.T) {
	for _, text := range []string{"Yesterday I thought about fitness.", "Fitness thought", "coding thought, exercise."} {
		t.Run("no cleanup/"+text, func(t *testing.T) {
			m, load, subjects := discoveryModel(t)
			subjects.items = append(subjects.items, data.Subject{SubjectID: 40, SubjectName: "coding"}, data.Subject{SubjectID: 41, SubjectName: "coDing"})
			m, _ = m.Update(load())
			m, r := startDiscoveryRecording(m)
			m = transcript(m, r, 1, text)
			m, _ = m.Update(voice.RecordingEnded{Recording: r})
			m, _ = m.Update(key(tea.KeyF9))
			if m.input.Value() != text || strings.Contains(m.View(), "Restore prefix") {
				t.Fatal("F9 offered without cleanup")
			}
		})
	}
	t.Run("new draft rejects retired recording and snapshot", func(t *testing.T) {
		m, load, subjects := discoveryModel(t)
		reply := load()
		m, _ = m.Update(reply)
		m, r := startDiscoveryRecording(m)
		m = transcript(m, r, 1, "Fitness thought, exercise.")
		m, _ = m.Update(voice.RecordingEnded{Recording: r})
		m.OpenVoice(2, subjects)
		m, _ = m.Update(reply)
		m, _ = m.Update(voice.RecordingEnded{Recording: r})
		m, _ = m.Update(key(tea.KeyF9))
		if m.input.Value() != "" || m.subjectID != nil || strings.Contains(m.View(), "Restore prefix") {
			t.Fatal("old cleanup leaked to new draft")
		}
	})
	t.Run("wrapped help preserves narrow layout", func(t *testing.T) {
		m, load, _ := discoveryModel(t)
		m, _ = m.Update(load())
		m, r := startDiscoveryRecording(m)
		m = transcript(m, r, 1, "Fitness thought, exercise.")
		m, _ = m.Update(voice.RecordingEnded{Recording: r})
		m.Resize(40, 24)
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "Restore prefix") {
			t.Fatalf("missing restore help: %s", view)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > 40 {
				t.Fatalf("row wider than terminal: %q", line)
			}
		}
	})
}
