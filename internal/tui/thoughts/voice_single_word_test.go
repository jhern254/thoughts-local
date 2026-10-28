package thoughts

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/voice"
)

func TestModel_SingleWordVoiceSubjects(t *testing.T) {
	for _, arrival := range []string{"before recording", "during recording", "after Stop", "after recording ends"} {
		t.Run("unknown Fitness cleans command/"+arrival, func(t *testing.T) {
			m, load, subjects := discoveryModel(t)
			subjects.items = nil
			reply := load()
			stamp := m.draftObservedAt
			if arrival == "before recording" {
				m, _ = m.Update(reply)
			}
			m, recording := startDiscoveryRecording(m)
			const text = "Fitness thought I'm trying to start"
			m = transcript(m, recording, 1, text)
			if arrival == "during recording" {
				m, _ = m.Update(reply)
			}
			if m.input.Value() != text {
				t.Fatalf("live text=%q, want %q", m.input.Value(), text)
			}
			m, _ = m.Update(VoiceAction{Action: "stop", DraftID: 1, RecordingID: recording.RecordingID})
			if arrival == "after Stop" {
				m, _ = m.Update(reply)
			}
			m, _ = m.Update(voice.RecordingEnded{Recording: recording})
			if arrival == "after recording ends" {
				m, _ = m.Update(reply)
			}
			if m.subjectID != nil || m.voice.query.Value() != "Fitness" || m.input.Value() != "I'm trying to start" {
				t.Fatalf("subject=%v query=%q text=%q, want optional Fitness and cleaned body", m.subjectID, m.voice.query.Value(), m.input.Value())
			}
			_, save := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
			if save == nil {
				t.Fatal("proposed Subject blocked Misc save")
			}
			result := save().(Result)
			if result.err != nil || result.item.SubjectID != nil || result.item.Thought != "I'm trying to start" || !result.item.ObservedAt.Equal(stamp) {
				t.Fatalf("save=%+v error=%v", result.item, result.err)
			}
		})
	}

	for _, tc := range []struct {
		cue, name string
		id        int64
	}{
		{"fitness", "Fitness", 8}, {"FITNESS", "Fitness", 8}, {"work", "Work", 9}, {"programming", "Programming", 10}, {"prayer", "Prayer", 11}, {"golang", "Go Programming", 7},
	} {
		t.Run(tc.cue, func(t *testing.T) {
			m, load, subjects := discoveryModel(t)
			subjects.items = append(subjects.items, data.Subject{SubjectID: 9, SubjectName: "Work"}, data.Subject{SubjectID: 10, SubjectName: "Programming"}, data.Subject{SubjectID: 11, SubjectName: "Prayer"})
			subjects.aliases = append(subjects.aliases, data.SubjectAlias{UserID: "u", SubjectID: 7, AliasName: "Golang"})
			m, _ = m.Update(load())
			m, r := startDiscoveryRecording(m)
			text := tc.cue + " thought, exercised today."
			m = transcript(m, r, 1, text)
			if m.subjectID == nil || *m.subjectID != tc.id || m.voice.query.Value() != tc.name || m.input.Value() != text {
				t.Fatalf("live subject=%v query=%q text=%q", m.subjectID, m.voice.query.Value(), m.input.Value())
			}
			m, _ = m.Update(voice.RecordingEnded{Recording: r})
			if m.input.Value() != "Exercised today." {
				t.Fatalf("body=%q", m.input.Value())
			}
		})
	}
	t.Run("late folded collision preserves a completed multiword prefix", func(t *testing.T) {
		m, load, subjects := discoveryModel(t)
		subjects.items = append(subjects.items, data.Subject{SubjectID: 20, SubjectName: "go programming"})
		reply := load()
		m, r := startDiscoveryRecording(m)
		text := "Go Programming thought, learned interfaces."
		m = transcript(m, r, 1, text)
		m, _ = m.Update(voice.RecordingEnded{Recording: r})
		if m.input.Value() != text {
			t.Fatal("cleaned before checking possible folded collisions")
		}
		m, _ = m.Update(reply)
		if m.subjectID != nil || m.voice.query.Value() != "" || len(m.voice.suggestion.matches) != 0 || m.input.Value() != text {
			t.Fatalf("subject=%v query=%q text=%q", m.subjectID, m.voice.query.Value(), m.input.Value())
		}
	})
	t.Run("case collision stays literal without candidates and saves as Misc", func(t *testing.T) {
		m, load, subjects := discoveryModel(t)
		subjects.items = append(subjects.items, data.Subject{SubjectID: 20, SubjectName: "coding"}, data.Subject{SubjectID: 21, SubjectName: "coDing"})
		m, _ = m.Update(load())
		m, r := startDiscoveryRecording(m)
		text := "coding thought, practiced algorithms."
		m = transcript(m, r, 1, text)
		m, _ = m.Update(voice.RecordingEnded{Recording: r})
		if m.subjectID != nil || m.voice.query.Value() != "" || len(m.voice.suggestion.matches) != 0 || m.voice.subjectFocused || m.input.Value() != text {
			t.Fatalf("subject=%v query=%q text=%q", m.subjectID, m.voice.query.Value(), m.input.Value())
		}
		_, save := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
		if save == nil {
			t.Fatal("collision blocked Misc save")
		}
		result := save().(Result)
		if result.err != nil || result.item.SubjectID != nil || result.item.Thought != text {
			t.Fatalf("save=%+v error=%v", result.item, result.err)
		}
	})
	for _, action := range []string{"save Misc", "create", "select", "cancel creation", "edit body", "record again"} {
		t.Run("unknown word/"+action, func(t *testing.T) {
			m, load, _ := discoveryModel(t)
			m, _ = m.Update(load())
			stamp := m.draftObservedAt
			m, r := startDiscoveryRecording(m)
			text := "meditation thought, étudier la respiration."
			m = transcript(m, r, 1, text)
			m, _ = m.Update(voice.RecordingEnded{Recording: r})
			text = "Étudier la respiration."
			if m.subjectID != nil || m.voice.query.Value() != "meditation" || m.input.Value() != text {
				t.Fatalf("proposal subject=%v query=%q text=%q", m.subjectID, m.voice.query.Value(), m.input.Value())
			}
			switch action {
			case "record again":
				m, r = startDiscoveryRecording(m)
				m = transcript(m, r, 1, "More literal text.")
				m, _ = m.Update(voice.RecordingEnded{Recording: r})
				text += "\nMore literal text."
			case "create", "cancel creation", "edit body":
				if action == "edit body" {
					m, _ = m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
					m, _ = m.Update(tea.PasteMsg{Content: " Added manually."})
					text += " Added manually."
				}
				m, _ = m.Update(key(tea.KeyTab))
				var cmd tea.Cmd
				m, cmd = m.Update(key(tea.KeyEnter))
				request := cmd().(VoiceSubjectRequest)
				if request.Query != "meditation" {
					t.Fatalf("query=%q", request.Query)
				}
				var created *data.Subject
				if action != "cancel creation" {
					created = &data.Subject{SubjectID: 30, SubjectName: "meditation"}
				}
				m.ReturnFromVoiceSubject(request, created)
			case "select":
				m, _ = m.Update(key(tea.KeyTab))
				m, _ = m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
				m, _ = m.Update(key(tea.KeyDown))
				m, _ = m.Update(key(tea.KeyEnter))
			}
			if m.input.Value() != text || m.draftObservedAt != stamp {
				t.Fatalf("body=%q, want %q at original opening time", m.input.Value(), text)
			}
			if action == "create" || action == "edit body" || action == "select" {
				if m.subjectID == nil {
					t.Fatal("explicit confirmation did not select a Subject")
				}
			} else if m.subjectID != nil {
				t.Fatal("unconfirmed proposal selected a Subject")
			}
			_, save := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
			if save == nil {
				t.Fatal("proposal blocked saving")
			}
			result := save().(Result)
			if result.err != nil || result.item.Thought != text || !result.item.ObservedAt.Equal(stamp) {
				t.Fatalf("save=%+v error=%v", result.item, result.err)
			}
		})
	}
}
