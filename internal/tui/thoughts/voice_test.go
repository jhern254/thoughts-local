package thoughts

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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
		m, _ = m.Update(VoiceAction{Action: "start", Draft: 1})
		state := m.VoiceState()
		if state.State != "requesting" || state.Recording != 1 {
			t.Fatalf("got %+v, want requesting recording 1", state)
		}
		for _, msg := range []tea.Msg{tea.PasteMsg{Content: "PRIVATE"}, tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}), key(tea.KeyTab), tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"})} {
			var cmd tea.Cmd
			m, cmd = m.Update(msg)
			if cmd != nil || m.input.Value() != "preserved 界" || m.loading {
				t.Fatal("recording mutated or saved draft")
			}
		}
		m, _ = m.Update(VoiceAction{Action: "recording", Draft: 1, Recording: 1})
		m, _ = m.Update(VoiceAction{Action: "stop", Draft: 1, Recording: 1})
		if m.VoiceState().State != "stopping" {
			t.Fatal("Stop did not lock pending finalization")
		}
		m, _ = m.Update(VoiceAction{Action: "stopped", Draft: 1, Recording: 1})
		m, _ = m.Update(tea.PasteMsg{Content: " edited"})
		if got := m.input.Value(); got != "preserved 界 edited" || !m.input.Focused() {
			t.Fatalf("got %q, want preserved focused editor", got)
		}
		m, _ = m.Update(VoiceAction{Action: "start", Draft: 1, Recording: 1})
		if m.VoiceState().Recording != 2 || m.input.Value() != "preserved 界 edited" {
			t.Fatal("restart lost edits or reused identity")
		}
		m, _ = m.Update(VoiceAction{Action: "stopped", Draft: 1, Recording: 1})
		if m.VoiceState().State != "requesting" {
			t.Fatal("stale stopped callback unlocked new recording")
		}
	})
	t.Run("picker selects clears and preserves subject across recording", func(t *testing.T) {
		m := voiceModel(t)
		reader := &voiceSubjects{items: []data.Subject{{SubjectID: 7, SubjectName: "Work"}}}
		cmd := m.loadVoiceSubjects(reader)
		m, _ = m.Update(cmd())
		m, _ = m.Update(key(tea.KeyTab))
		m, _ = m.Update(tea.PasteMsg{Content: "Wor"})
		m, _ = m.Update(key(tea.KeyDown))
		m, _ = m.Update(key(tea.KeyDown))
		m, _ = m.Update(key(tea.KeyEnter))
		if m.subjectID == nil || *m.subjectID != 7 || !strings.Contains(m.View(), "Work") {
			t.Fatal("picker did not select Work")
		}
		m, _ = m.Update(VoiceAction{Action: "start", Draft: 1})
		m, _ = m.Update(VoiceAction{Action: "denied", Draft: 1, Recording: 1})
		if m.subjectID == nil || *m.subjectID != 7 || !m.input.Focused() {
			t.Fatal("permission failure lost subject or focus")
		}
		m, _ = m.Update(key(tea.KeyTab))
		m, _ = m.Update(key(tea.KeyEnter))
		if m.subjectID != nil {
			t.Fatal("Unassigned did not clear subject")
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
		m, _ = m.Update(VoiceAction{Action: "start", Draft: 1})
		m, _ = m.Update(VoiceAction{Action: "stopped", Draft: 1, Recording: 1})
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
		if !m.input.Focused() || m.input.Value() != "PRIVATE-VOICE-DRAFT" || m.subjectID == nil || *m.subjectID != 7 || m.VoiceState().State != "idle" {
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
		if !m.ShowingDetail() || m.VoiceState().Draft != 0 || m.SelectedSubjectName() != "Work" {
			t.Fatal("saved voice detail lost subject name or retained recording capability")
		}
	})

}
