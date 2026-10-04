package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
)

func TestModel_VoiceDraft(t *testing.T) {
	t.Run("native mode refuses browser voice entry", func(t *testing.T) {
		m := newRootTestModel()
		m, _ = rootUpdate(m, thoughts.VoiceAction{Action: "open"})
		if m.screen != screenEvents {
			t.Fatal("native voice action changed screen")
		}
	})
	t.Run("subject creation resumes exact voice draft and selection", func(t *testing.T) {
		m := newRootTestModel()
		m.subjects.service = &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) { return nil, nil }, create: func(_ context.Context, user, name string) (*data.Subject, error) {
			return &data.Subject{SubjectID: 42, UserID: user, SubjectName: name}, nil
		}}
		m, _ = rootUpdate(m, thoughts.VoiceAvailable{})
		m, cmd := rootUpdate(m, thoughts.VoiceAction{Action: "open"})
		batch := cmd().(tea.BatchMsg)
		m, _ = rootUpdate(m, batch[1]())
		m, _ = rootUpdate(m, tea.PasteMsg{Content: "preserved draft 界"})
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
		m, _ = rootUpdate(m, tea.PasteMsg{Content: "Voice subject"})
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
		m = runModelCommand(t, m, enterKey())
		if m.screen != screenSubjectCreate || m.subjects.input.Value() != "Voice subject" || m.VoiceState().Draft != 0 {
			t.Fatal("creation did not suspend voice draft")
		}
		m = runModelCommand(t, m, enterKey())
		view := m.View().Content
		if m.screen != screenVoiceThought || !strings.Contains(view, "preserved draft 界") || !strings.Contains(view, "Subject: Voice subject") || m.VoiceState().Draft == 0 {
			t.Fatalf("got resumed view %s", view)
		}
		m, _ = rootUpdate(m, escapeKey())
		if m.screen != screenEvents || m.VoiceState().Draft != 0 {
			t.Fatal("cancel did not end voice draft")
		}
	})
}
