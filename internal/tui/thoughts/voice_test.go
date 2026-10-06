package thoughts

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
)

type voiceSubjects struct {
	ctx   context.Context
	items []data.Subject
}

func (s *voiceSubjects) List(ctx context.Context, _ string) ([]data.Subject, error) {
	s.ctx = ctx
	return s.items, nil
}
func voiceModel(t *testing.T) Model {
	t.Helper()
	m := New(t.Context(), "u", thought.NewService(testutils.NewFakeThoughtStore()), logging.Nop())
	m.OpenVoice(1, &voiceSubjects{})
	return m
}
func TestModel_Voice(t *testing.T) {
	t.Run("recording locks editor paste subject and save until stopped", func(t *testing.T) {
		m := voiceModel(t)
		m, _ = m.Update(tea.PasteMsg{Content: "preserved 界"})
		m, _ = m.Update(VoiceAction{Action: "start", DraftID: 1})
		state := m.VoiceState()
		if state.RecordingStatus != "requesting" || state.RecordingID != 1 {
			t.Fatalf("got %+v, want requesting recording 1", state)
		}
		for _, msg := range []tea.Msg{tea.PasteMsg{Content: "PRIVATE"}, tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}), key(tea.KeyTab), tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"})} {
			var cmd tea.Cmd
			m, cmd = m.Update(msg)
			if cmd != nil || m.input.Value() != "preserved 界" || m.loading {
				t.Fatal("recording mutated or saved draft")
			}
		}
		m, _ = m.Update(VoiceAction{Action: "recording", DraftID: 1, RecordingID: 1})
		if got := m.VoiceState().RecordingStatus; got != "recording" {
			t.Fatalf("recording acknowledgement got %q, want recording", got)
		}
		m, _ = m.Update(VoiceAction{Action: "stop", DraftID: 1, RecordingID: 1})
		if m.VoiceState().RecordingStatus != "stopping" {
			t.Fatal("Stop did not lock pending finalization")
		}
		m, _ = m.Update(VoiceAction{Action: "stopped", DraftID: 1, RecordingID: 1})
		m, _ = m.Update(tea.PasteMsg{Content: " edited"})
		if got := m.input.Value(); got != "preserved 界 edited" || !m.input.Focused() {
			t.Fatalf("got %q, want preserved focused editor", got)
		}
		m, _ = m.Update(VoiceAction{Action: "start", DraftID: 1, RecordingID: 1})
		if m.VoiceState().RecordingID != 2 || m.input.Value() != "preserved 界 edited" {
			t.Fatal("restart lost edits or reused identity")
		}
		m, _ = m.Update(VoiceAction{Action: "stopped", DraftID: 1, RecordingID: 1})
		if m.VoiceState().RecordingStatus != "requesting" {
			t.Fatal("stale stopped callback unlocked new recording")
		}
	})
	t.Run("picker starts with creation and clears assignment through the optional field", func(t *testing.T) {
		m := voiceModel(t)
		reader := &voiceSubjects{items: []data.Subject{{SubjectID: 7, SubjectName: "Work"}}}
		cmd := m.loadVoiceSubjects(reader)
		m, _ = m.Update(cmd())
		m, _ = m.Update(key(tea.KeyTab))
		if view := ansi.Strip(m.View()); !strings.Contains(view, "> Create subject…") || strings.Contains(view, "Unassigned") || !strings.Contains(view, "Misc") {
			t.Fatalf("picker got %q, want creation first without Unassigned", view)
		}
		m, _ = m.Update(tea.PasteMsg{Content: "Wor"})
		m, _ = m.Update(key(tea.KeyDown))
		m, _ = m.Update(key(tea.KeyEnter))
		if m.subjectID == nil || *m.subjectID != 7 || !strings.Contains(ansi.Strip(m.View()), "Work") {
			t.Fatalf("picker subject got %v, name %q, view %q, want Work", m.subjectID, m.voice.subjectName, m.View())
		}
		m, _ = m.Update(VoiceAction{Action: "start", DraftID: 1})
		m, _ = m.Update(VoiceAction{Action: "denied", DraftID: 1, RecordingID: 1})
		if m.subjectID == nil || *m.subjectID != 7 || !m.input.Focused() {
			t.Fatal("permission failure lost subject or focus")
		}
		m, _ = m.Update(key(tea.KeyTab))
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'u', Mod: tea.ModCtrl}))
		if m.subjectID != nil || m.voice.query.Value() != "" || !strings.Contains(ansi.Strip(m.View()), "Misc") {
			t.Fatal("clearing the subject field did not restore Misc")
		}
	})
	t.Run("recording shortcut preserves ordinary v input and shows one active action", func(t *testing.T) {
		m := voiceModel(t)
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'v', Text: "v"}))
		if m.input.Value() != "v" || !strings.Contains(m.View(), "F8: Record") || strings.Contains(m.View(), "F8: Stop") || strings.Contains(m.View(), "Voice thought") {
			t.Fatalf("got %q, want normal editor with v and Record only", m.View())
		}
		m, _ = m.Update(key(tea.KeyF8))
		if m.VoiceState().RecordingStatus != "requesting" || !strings.Contains(m.View(), "F8: Stop") || strings.Contains(m.View(), "F8: Record") {
			t.Fatalf("got %q, want Stop only while requesting", m.View())
		}
		m, _ = m.Update(key(tea.KeyF8))
		if m.VoiceState().RecordingStatus != "stopping" || m.input.Value() != "v" {
			t.Fatal("F8 did not stop without changing draft")
		}
	})

	t.Run("cancel and save preserve the original browsing subject", func(t *testing.T) {
		for _, save := range []bool{false, true} {
			m := New(t.Context(), "u", thought.NewService(testutils.NewFakeThoughtStore()), logging.Nop())
			original, chosen := int64(7), int64(9)
			m.subjectID = &original
			m.screen = create
			m.EnableVoice(1, &voiceSubjects{}, "Original")
			m.subjectID = &chosen
			m.voice.query.SetValue("Chosen")
			m.voice.subjectName = "Chosen"
			m.input.SetValue("preserved")
			if save {
				var cmd tea.Cmd
				m, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
				m, cmd = m.Update(cmd())
				change := cmd().(ChangedMsg)
				if change.SubjectID == nil || *change.SubjectID != chosen || m.selected.SubjectID == nil || *m.selected.SubjectID != chosen {
					t.Fatal("save did not persist and invalidate the chosen subject")
				}
			} else {
				m, _ = m.Update(key(tea.KeyEsc))
			}
			if m.subjectID == nil || *m.subjectID != original || m.VoiceOpen() {
				t.Fatal("leaving editor lost its original browsing scope")
			}
		}
	})

	t.Run("closing cancels subject reads and ignores queued replies", func(t *testing.T) {
		m := voiceModel(t)
		reader := &voiceSubjects{items: []data.Subject{{SubjectID: 7, SubjectName: "old"}}}
		reply := m.loadVoiceSubjects(reader)()
		m.Reset()
		if reader.ctx.Err() == nil {
			t.Fatal("closing did not cancel picker read")
		}
		m.OpenVoice(2, &voiceSubjects{})
		m, _ = m.Update(reply)
		if strings.Contains(m.View(), "old") {
			t.Fatal("old picker reply changed new draft")
		}
	})
	t.Run("reopening preserves editor geometry and rejects an earlier clipboard reply", func(t *testing.T) {
		m := voiceModel(t)
		m.Resize(40, 24)
		before := m.input.Height()
		oldRequest := m.request
		m, _ = m.Update(VoiceAction{Action: "start", DraftID: 1})
		m, _ = m.Update(VoiceAction{Action: "stopped", DraftID: 1, RecordingID: 1})
		m, _ = m.Update(clipboardResult{request: oldRequest, content: "PRIVATE-CLIPBOARD"})
		if m.input.Value() != "" {
			t.Fatal("old clipboard result was inserted after recording")
		}
		m.Reset()
		m.OpenVoice(2, &voiceSubjects{})
		if got := m.input.Height(); got != before {
			t.Fatalf("height got %d, want %d after reopening", got, before)
		}
	})
	t.Run("save failure preserves text subject and safe diagnostics", func(t *testing.T) {
		var logs bytes.Buffer
		logger, err := logging.New(&logs, "test", "info")
		if err != nil {
			t.Fatal(err)
		}
		m := voiceModel(t)
		m.logger = logger
		m.service = failingService{err: errors.New("PRIVATE-VOICE-ERROR")}
		m.input.SetValue("PRIVATE-VOICE-DRAFT")
		id := int64(7)
		m.subjectID = &id
		m.voice.subjectName = "PRIVATE-SUBJECT"
		m, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
		m, _ = m.Update(cmd())
		if !m.input.Focused() || m.input.Value() != "PRIVATE-VOICE-DRAFT" || m.subjectID == nil || *m.subjectID != 7 || m.VoiceState().RecordingStatus != "idle" {
			t.Fatal("save failure lost editable voice draft")
		}
		if strings.Contains(logs.String(), "PRIVATE-") || strings.Contains(m.View(), "PRIVATE-VOICE-ERROR") {
			t.Fatal("private data escaped operational diagnostics")
		}
	})

	t.Run("saved detail keeps the selected subject name", func(t *testing.T) {
		m := voiceModel(t)
		m.input.SetValue("saved thought")
		id := int64(7)
		m.subjectID = &id
		m.voice.subjectName = "Work"
		m, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
		m, _ = m.Update(cmd())
		if !m.ShowingDetail() || m.VoiceState().DraftID != 0 || m.SelectedSubjectName() != "Work" {
			t.Fatal("saved voice detail lost subject name or retained recording capability")
		}
	})

}
