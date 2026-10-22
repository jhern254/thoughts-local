package thoughts

import (
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/voice"
)

func TestModel_VoiceCommandEligibility(t *testing.T) {
	t.Run("first-recording position does not confer command permission", func(t *testing.T) {
		for _, tc := range []struct {
			name, text, body, query string
			subjectID               int64
			needsExactEvidence      bool
		}{
			{name: "confirmed alias", text: "Programming in Go thought, étudier les interfaces.", body: "Étudier les interfaces.", query: "Go Programming", subjectID: 7},
			{name: "confirmed unknown", text: "Machine Learning Research thought, studied embeddings.", body: "Studied embeddings.", query: "Machine Learning Research"},
			{name: "ordinary", text: "Yesterday I thought about programming."},
			{name: "conversational article", text: "Just a thought about work."},
			{name: "conversational introduction", text: "Here's another thought about Go."},
			{name: "uncertain noun phrase", text: "Just an ordinary thought about today."},
			{name: "uncertain comparative", text: "One more thought about my goals."},
			{name: "possible exact name", text: "Fitness thought, exercise afterward.", body: "Exercise afterward.", query: "Fitness", subjectID: 8, needsExactEvidence: true},
		} {
			for _, arrival := range []string{"before recording", "during recording", "after Stop", "after recording ends"} {
				t.Run(tc.name+"/"+arrival, func(t *testing.T) {
					m, load, _ := discoveryModel(t)
					reply := load()
					stamp := m.draftObservedAt
					if arrival == "before recording" {
						m, _ = m.Update(reply)
						if m.subjectID != nil || m.voice.query.Value() != "" || m.input.Value() != "" || m.VoiceState().RecordingStatus != "idle" {
							t.Fatal("candidate data alone changed an empty idle draft")
						}
					}
					m, recording := startDiscoveryRecording(m)
					m = transcript(m, recording, 1, tc.text)
					if arrival == "during recording" {
						m, _ = m.Update(reply)
					}
					if m.input.Value() != tc.text || m.VoiceState().RecordingStatus != "requesting" {
						t.Fatalf("live text=%q status=%q, want full transcript during recording", m.input.Value(), m.VoiceState().RecordingStatus)
					}
					m, _ = m.Update(VoiceAction{Action: "stop", DraftID: 1, RecordingID: recording.RecordingID})
					if arrival == "after Stop" {
						m, _ = m.Update(reply)
					}
					if m.input.Value() != tc.text || m.VoiceState().RecordingStatus != "stopping" {
						t.Fatalf("stopped text=%q status=%q, want frozen full transcript", m.input.Value(), m.VoiceState().RecordingStatus)
					}
					m, _ = m.Update(voice.RecordingEnded{Recording: recording})
					if arrival == "after recording ends" {
						m, _ = m.Update(reply)
					}
					body, query, subjectID := tc.body, tc.query, tc.subjectID
					if body == "" || (tc.needsExactEvidence && (arrival == "after Stop" || arrival == "after recording ends")) {
						body, query, subjectID = tc.text, "", 0
					}
					var gotID int64
					if m.subjectID != nil {
						gotID = *m.subjectID
					}
					if gotID != subjectID || m.voice.query.Value() != query || m.input.Value() != body || m.VoiceState().RecordingStatus != "idle" || m.draftObservedAt != stamp {
						t.Fatalf("subject=%d query=%q text=%q status=%q, want subject=%d query=%q text=%q and original opening time", gotID, m.voice.query.Value(), m.input.Value(), m.VoiceState().RecordingStatus, subjectID, query, body)
					}
				})
			}
		}
	})
	t.Run("candidate arrivals never reinterpret typed or subsequent recordings", func(t *testing.T) {
		for _, previous := range []string{"typed text", "ordinary recording", "confirmed recording"} {
			for _, arrival := range []string{"during recording", "after Stop", "after recording ends"} {
				t.Run(previous+"/"+arrival, func(t *testing.T) {
					m, load, _ := discoveryModel(t)
					reply := load()
					base := "Previously typed text."
					query := ""
					switch previous {
					case "typed text":
						m.input.SetValue(base)
					case "ordinary recording", "confirmed recording":
						var first voice.RecordingKey
						m, first = startDiscoveryRecording(m)
						text := "Yesterday I thought about Go."
						base = text
						if previous == "confirmed recording" {
							text = "Programming in Go thought, learned interfaces."
							base, query = text, "Programming in Go"
						}
						m = transcript(m, first, 1, text)
						m, _ = m.Update(voice.RecordingEnded{Recording: first})
					}
					stamp := m.draftObservedAt
					m, recording := startDiscoveryRecording(m)
					text := "Fitness thought I should exercise afterward."
					m = transcript(m, recording, 1, text)
					if arrival == "during recording" {
						m, _ = m.Update(reply)
					}
					if m.subjectID != nil || m.voice.query.Value() != query || m.input.Value() != base+"\n"+text || m.VoiceState().RecordingStatus != "requesting" {
						t.Fatalf("live subject=%v query=%q text=%q status=%q, want literal appended recording", m.subjectID, m.voice.query.Value(), m.input.Value(), m.VoiceState().RecordingStatus)
					}
					m, _ = m.Update(VoiceAction{Action: "stop", DraftID: 1, RecordingID: recording.RecordingID})
					if arrival == "after Stop" {
						m, _ = m.Update(reply)
					}
					m, _ = m.Update(voice.RecordingEnded{Recording: recording})
					if arrival == "after recording ends" {
						m, _ = m.Update(reply)
					}
					if m.subjectID != nil || m.voice.query.Value() != query || m.input.Value() != base+"\n"+text || m.VoiceState().RecordingStatus != "idle" || m.draftObservedAt != stamp {
						t.Fatalf("ended subject=%v query=%q text=%q status=%q, want original selection, literal text and opening time", m.subjectID, m.voice.query.Value(), m.input.Value(), m.VoiceState().RecordingStatus)
					}
				})
			}
		}
	})
	t.Run("uncertain command cannot gain exact-name permission after revocation", func(t *testing.T) {
		for _, stopAction := range []string{"stop", "failed", "stopped", "denied", "unavailable", "revoked", "ended"} {
			for _, arrival := range []string{"before ended", "after ended"} {
				t.Run(stopAction+"/"+arrival, func(t *testing.T) {
					m, load, _ := discoveryModel(t)
					reply := load()
					m, recording := startDiscoveryRecording(m)
					text := "Fitness thought, exercise afterward."
					m = transcript(m, recording, 1, text)
					if stopAction != "ended" {
						if stopAction == "revoked" {
							m.voice.transcriptAuthority.Revoke()
						} else {
							m, _ = m.Update(VoiceAction{Action: stopAction, DraftID: 1, RecordingID: recording.RecordingID})
						}
						if arrival == "before ended" {
							m, _ = m.Update(reply)
						}
					}
					m, _ = m.Update(voice.RecordingEnded{Recording: recording})
					m, _ = m.Update(reply)
					if m.subjectID != nil || m.voice.query.Value() != "" || m.input.Value() != text || m.VoiceState().RecordingStatus != "idle" {
						t.Fatalf("subject=%v query=%q text=%q status=%q, want unassigned literal completed recording", m.subjectID, m.voice.query.Value(), m.input.Value(), m.VoiceState().RecordingStatus)
					}
				})
			}
		}
	})
	t.Run("contextual Subject remains explicit during delayed resolution", func(t *testing.T) {
		m, _, subjects := discoveryModel(t)
		id := int64(7)
		m.subjectID = &id
		load := m.EnableVoice(1, subjects, "Go Programming")
		reply := load()
		m, recording := startDiscoveryRecording(m)
		m = transcript(m, recording, 1, "Fitness Training thought, study movement.")
		m, _ = m.Update(voice.RecordingEnded{Recording: recording})
		if got, want := m.input.Value(), "Fitness Training thought, study movement."; got != want {
			t.Fatalf("text before candidate data = %q, want %q", got, want)
		}
		m, _ = m.Update(reply)
		if m.subjectID == nil || *m.subjectID != id || m.voice.query.Value() != "Go Programming" || m.input.Value() != "Study movement." || m.VoiceState().RecordingStatus != "idle" {
			t.Fatalf("subject=%v query=%q text=%q status=%q, want contextual selection and cleaned confirmed body", m.subjectID, m.voice.query.Value(), m.input.Value(), m.VoiceState().RecordingStatus)
		}
	})
	t.Run("contextual Subject does not authorize cleanup of a delayed name collision", func(t *testing.T) {
		m, _, subjects := discoveryModel(t)
		id := int64(7)
		m.subjectID = &id
		subjects.items = append(subjects.items,
			data.Subject{SubjectID: 9, SubjectName: "Fitness Training"},
			data.Subject{SubjectID: 10, SubjectName: "fitness training"})
		load := m.EnableVoice(1, subjects, "Go Programming")
		reply := load()
		stamp := m.draftObservedAt
		m, recording := startDiscoveryRecording(m)
		const text = "Fitness Training thought, study movement."
		m = transcript(m, recording, 1, text)
		m, _ = m.Update(voice.RecordingEnded{Recording: recording})
		m, _ = m.Update(reply)
		if m.subjectID == nil || *m.subjectID != id || m.voice.query.Value() != "Go Programming" || m.input.Value() != text || m.draftObservedAt != stamp || m.VoiceState().RecordingStatus != "idle" {
			t.Fatalf("subject=%v query=%q text=%q status=%q, want contextual selection and unchanged ambiguous dictation", m.subjectID, m.voice.query.Value(), m.input.Value(), m.VoiceState().RecordingStatus)
		}
	})
	t.Run("late contextual cleanup preserves edits made after recording", func(t *testing.T) {
		m, _, subjects := discoveryModel(t)
		id := int64(7)
		m.subjectID = &id
		load := m.EnableVoice(1, subjects, "Go Programming")
		reply := load()
		m, recording := startDiscoveryRecording(m)
		m = transcript(m, recording, 1, "Fitness Training thought, study movement.")
		m, _ = m.Update(voice.RecordingEnded{Recording: recording})
		m.input.SetValue("My edited thought.")
		m, _ = m.Update(reply)
		if m.subjectID == nil || *m.subjectID != id || m.voice.query.Value() != "Go Programming" || m.input.Value() != "My edited thought." {
			t.Fatalf("subject=%v query=%q text=%q, want contextual selection and preserved edits", m.subjectID, m.voice.query.Value(), m.input.Value())
		}
	})
	t.Run("a confirmed selection survives another recording and delayed data", func(t *testing.T) {
		m, load, subjects := discoveryModel(t)
		m, _ = m.Update(load())
		m, first := startDiscoveryRecording(m)
		m = transcript(m, first, 1, "Programming in Go thought, learned interfaces.")
		m, _ = m.Update(voice.RecordingEnded{Recording: first})
		// A queued read must not give the next recording the previous one's cue.
		reply := m.loadVoiceSubjects(subjects)()
		m, second := startDiscoveryRecording(m)
		m = transcript(m, second, 1, "Fitness thought I should exercise afterward.")
		m, _ = m.Update(reply)
		m, _ = m.Update(voice.RecordingEnded{Recording: second})
		if m.subjectID == nil || *m.subjectID != 7 || m.voice.query.Value() != "Go Programming" || m.input.Value() != "Learned interfaces.\nFitness thought I should exercise afterward." || m.VoiceState().RecordingStatus != "idle" {
			t.Fatalf("subject=%v query=%q text=%q, want first selection and literal second recording", m.subjectID, m.voice.query.Value(), m.input.Value())
		}
	})
}
