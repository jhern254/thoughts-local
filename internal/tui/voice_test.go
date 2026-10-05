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
	t.Run("t opens the same titled editor as regular browser creation", func(t *testing.T) {
		m := newRootTestModel()
		m, _ = rootUpdate(m, thoughts.VoiceAvailable{})
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: 't', Text: "t"}))
		heading := m.subjects.list.Styles.TitleBar.Render(m.subjects.list.Styles.Title.Render("Thoughts"))
		if !m.thoughts.Creating() || !strings.HasPrefix(m.View().Content, heading+"\nCreate thought\n") || strings.Contains(m.View().Content, "Voice thought") {
			t.Fatalf("quick entry got %q, want normal Thoughts heading and editor", m.View().Content)
		}
		quick := m.View().Content
		m = newRootTestModel()
		m, _ = rootUpdate(m, thoughts.VoiceAvailable{})
		m.entityFocused = true
		m.selectedEntity = entityThoughts
		m = runModelCommand(t, m, enterKey())
		m, _ = rootUpdate(m, enterKey())
		if !m.thoughts.VoiceOpen() || m.View().Content != quick {
			t.Fatalf("regular entry got %q, want same editor %q", m.View().Content, quick)
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: 't', Text: "t"}))
		if !strings.Contains(m.View().Content, "t") || m.voiceDraft != 1 {
			t.Fatal("t replaced the active draft instead of inserting text")
		}
	})

	t.Run("subject creation resumes exact voice draft and selection", func(t *testing.T) {
		m := newRootTestModel()
		m.subjects.service = &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) { return nil, nil }, create: func(_ context.Context, user, name string) (*data.Subject, error) {
			return &data.Subject{SubjectID: 42, UserID: user, SubjectName: name}, nil
		}}
		m, _ = rootUpdate(m, thoughts.VoiceAvailable{})
		m, cmd := rootUpdate(m, thoughts.VoiceAction{Action: "open"})
		original := &data.Subject{SubjectID: 7, SubjectName: "Original"}
		m.subjects.selected = original
		batch := cmd().(tea.BatchMsg)
		m, _ = rootUpdate(m, batch[1]())
		m, _ = rootUpdate(m, tea.PasteMsg{Content: "preserved draft 界"})
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
		m, _ = rootUpdate(m, tea.PasteMsg{Content: "Voice subject"})
		m = runModelCommand(t, m, enterKey())
		if m.screen != screenSubjectCreate || m.subjects.input.Value() != "Voice subject" || m.VoiceState().Draft != 0 {
			t.Fatal("creation did not suspend voice draft")
		}
		m = runModelCommand(t, m, enterKey())
		view := m.View().Content
		if m.subjects.selected != original || m.screen != screenVoiceThought || !strings.Contains(view, "preserved draft 界") || !strings.Contains(view, "Voice subject") || m.VoiceState().Draft == 0 {
			t.Fatalf("got resumed view %s", view)
		}
		m, _ = rootUpdate(m, escapeKey())
		if m.screen != screenEvents || m.VoiceState().Draft != 0 {
			t.Fatal("cancel did not end voice draft")
		}
	})
}
