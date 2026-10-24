package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
)

func TestModel_ThoughtCaptureTime(t *testing.T) {
	for _, entry := range []struct {
		name    string
		message tea.Msg
	}{
		{"browser t shortcut", tea.KeyPressMsg{Code: 't', Text: "t"}},
		{"browser F8 shortcut", tea.KeyPressMsg{Code: tea.KeyF8}},
		{"browser v shortcut", tea.KeyPressMsg{Code: 'v', Text: "v"}},
		{"browser open action", thoughts.VoiceAction{Action: "open"}},
	} {
		t.Run(entry.name+" saves the time of entry", func(t *testing.T) {
			m := newRootTestModel()
			m.subjects.service = &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) { return nil, nil }}
			service := thought.NewService(testutils.NewFakeThoughtStore())
			m.thoughts = thoughts.New(t.Context(), m.user.UserID, service, logging.Nop())
			m, _ = rootUpdate(m, thoughts.BrowserVoiceSessionMsg{SpeechRecognitionAvailable: true})
			before := time.Now()
			m = runModelCommand(t, m, entry.message)
			after := time.Now()
			state := m.VoiceState()
			if state.RecordingID != 0 {
				m, _ = rootUpdate(m, thoughts.VoiceAction{Action: "stopped", DraftID: state.DraftID, RecordingID: state.RecordingID})
			}
			m, _ = rootUpdate(m, tea.PasteMsg{Content: "capture time regression"})
			m = runModelCommand(t, m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
			items, err := service.ListUnassigned(t.Context(), m.user.UserID)
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != 1 {
				t.Fatalf("saved thoughts got %d, want 1", len(items))
			}
			if got := items[0].ObservedAt; got.Before(before) || got.After(after) {
				t.Fatalf("observed time got %v, want entry between %v and %v", got, before, after)
			}
			if !m.thoughts.ShowingDetail() {
				t.Fatal("save did not open thought detail")
			}
		})
	}
}

func TestModel_VoiceDraft(t *testing.T) {
	t.Run("native mode refuses browser voice entry", func(t *testing.T) {
		m := newRootTestModel()
		m, _ = rootUpdate(m, thoughts.VoiceAction{Action: "open"})
		if m.screen != screenEvents {
			t.Fatal("native voice action changed screen")
		}
	})
	t.Run("browser controls stay enabled when an active draft blocks quick entry", func(t *testing.T) {
		m := newRootTestModel()
		if state := m.VoiceState(); state.BrowserRecordingEnabled || state.CanOpenThoughtDraft {
			t.Fatalf("native state got %+v, want browser controls disabled", state)
		}
		m, _ = rootUpdate(m, thoughts.BrowserVoiceSessionMsg{SpeechRecognitionAvailable: true})
		if state := m.VoiceState(); !state.BrowserRecordingEnabled || !state.CanOpenThoughtDraft {
			t.Fatalf("browser home got %+v, want controls enabled and quick entry allowed", state)
		}
		m, _ = rootUpdate(m, thoughts.VoiceAction{Action: "open"})
		state := m.VoiceState()
		if !state.BrowserRecordingEnabled || state.CanOpenThoughtDraft || state.DraftID == 0 || state.RecordingStatus != "idle" {
			t.Fatalf("draft got %+v, want enabled controls without quick entry", state)
		}
		m, _ = rootUpdate(m, thoughts.VoiceAction{Action: "start", DraftID: state.DraftID})
		m, _ = rootUpdate(m, thoughts.VoiceAction{Action: "unavailable", DraftID: state.DraftID, RecordingID: 1})
		if state := m.VoiceState(); !state.BrowserRecordingEnabled || state.RecordingStatus != "idle" {
			t.Fatalf("unavailable microphone got %+v, want browser controls still enabled", state)
		}
	})

	t.Run("t opens the same titled editor as regular browser creation", func(t *testing.T) {
		m := newRootTestModel()
		m, _ = rootUpdate(m, thoughts.BrowserVoiceSessionMsg{})
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: 't', Text: "t"}))
		heading := m.subjects.list.Styles.TitleBar.Render(m.subjects.list.Styles.Title.Render("Thoughts"))
		if !m.thoughts.Creating() || !strings.HasPrefix(ansi.Strip(m.View().Content), ansi.Strip(heading)+"\nCreate thought · INSERT \n") || strings.Contains(m.View().Content, "Voice thought") {
			t.Fatalf("quick entry got %q, want normal Thoughts heading and editor", m.View().Content)
		}
		quick := m.View().Content
		m = newRootTestModel()
		m, _ = rootUpdate(m, thoughts.BrowserVoiceSessionMsg{})
		m.entityFocused = true
		m.selectedEntity = entityThoughts
		m = runModelCommand(t, m, enterKey())
		m, _ = rootUpdate(m, enterKey())
		if !m.thoughts.VoiceOpen() || m.View().Content != quick {
			t.Fatalf("regular entry got %q, want same editor %q", m.View().Content, quick)
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: 't', Text: "t"}))
		if !strings.Contains(m.View().Content, "t") || m.voiceDraftID != 1 {
			t.Fatal("t replaced the active draft instead of inserting text")
		}
	})

	t.Run("subject creation resumes exact voice draft and selection", func(t *testing.T) {
		m := newRootTestModel()
		m.subjects.service = &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) { return nil, nil }, create: func(_ context.Context, user, name string) (*data.Subject, error) {
			return &data.Subject{SubjectID: 42, UserID: user, SubjectName: name}, nil
		}}
		m, _ = rootUpdate(m, thoughts.BrowserVoiceSessionMsg{})
		m, cmd := rootUpdate(m, thoughts.VoiceAction{Action: "open"})
		original := &data.Subject{SubjectID: 7, SubjectName: "Original"}
		m.subjects.selected = original
		m, _ = rootUpdate(m, cmd())
		m, _ = rootUpdate(m, tea.PasteMsg{Content: "preserved draft 界"})
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
		m, _ = rootUpdate(m, tea.PasteMsg{Content: "Voice subject"})
		m = runModelCommand(t, m, enterKey())
		if m.screen != screenSubjectCreate || m.subjects.input.Value() != "Voice subject" || m.VoiceState().DraftID != 0 {
			t.Fatal("creation did not suspend voice draft")
		}
		m = runModelCommand(t, m, enterKey())
		view := m.View().Content
		if m.subjects.selected != original || m.screen != screenVoiceThought || !strings.Contains(view, "preserved draft 界") || !strings.Contains(view, "Voice subject") || m.VoiceState().DraftID == 0 {
			t.Fatalf("got resumed view %s", view)
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl})
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyTab})
		m, _ = rootUpdate(m, enterKey())
		if m.screen != screenEvents || m.VoiceState().DraftID != 0 {
			t.Fatal("cancel did not end voice draft")
		}
	})
}

func TestModel_BrowserF8(t *testing.T) {
	t.Run("Options does not advertise draft creation to browser microphone preparation", func(t *testing.T) {
		model := newRootTestModel()
		model, _ = rootUpdate(model, thoughts.BrowserVoiceSessionMsg{SessionID: "session", SpeechRecognitionAvailable: true})
		model, _ = rootUpdate(model, BrowserOptionsEnabledMsg{})
		model, _ = rootUpdate(model, BrowserOptionsMsg{Action: "open"})
		if model.VoiceState().CanOpenThoughtDraft {
			t.Fatal("Options advertised a recording shortcut it cannot accept")
		}
		model, _ = rootUpdate(model, tea.KeyPressMsg{Code: tea.KeyF8})
		if model.VoiceState().RecordingID != 0 {
			t.Fatal("F8 started a recording behind Options")
		}
	})
	t.Run("opens a draft and requests recording in one keypress from eligible screens", func(t *testing.T) {
		for _, screen := range []screen{screenEvents, screenSubjectList, screenBrowseThoughts} {
			m := newRootTestModel()
			m.screen = screen
			m, _ = rootUpdate(m, thoughts.BrowserVoiceSessionMsg{SessionID: "session", SpeechRecognitionAvailable: true})
			m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyF8}))
			if got, want := m.thoughts.VoiceOpen(), true; got != want {
				t.Fatalf("screen=%v draft=%v, want %v", screen, got, want)
			}
			if recording := m.VoiceState(); recording.RecordingStatus != "requesting" || recording.RecordingID == 0 || recording.TranscriptAuthority == nil {
				t.Fatal("F8 did not immediately request an authorized recording")
			}
			m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyF8}))
			if m.VoiceState().RecordingStatus != "stopping" {
				t.Fatal("second F8 did not stop recording")
			}
		}
	})
	t.Run("does not leave an active event form", func(t *testing.T) {
		m := newRootTestModel()
		m.entityFocused = false
		m.events.SetFocused(true)
		m = startHomeData(t, m, m.events.Open())
		defer func() { m.events.Close() }()
		m, _ = rootUpdate(m, thoughts.BrowserVoiceSessionMsg{})
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
		if m.events.CanLeave() {
			t.Fatal("event form did not open")
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyF8}))
		if m.thoughts.VoiceOpen() || m.events.CanLeave() {
			t.Fatal("F8 interrupted active event form")
		}
	})

	t.Run("unconfigured speech preserves typing without creating recording authority", func(t *testing.T) {
		m := newRootTestModel()
		m, _ = rootUpdate(m, thoughts.BrowserVoiceSessionMsg{})
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyF8}))
		m, _ = rootUpdate(m, tea.PasteMsg{Content: "typed draft"})
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyF8}))
		if m.VoiceState().RecordingID != 0 || m.VoiceState().TranscriptAuthority != nil || m.VoiceState().RecordingStatus != "idle" {
			t.Fatal("unconfigured speech started recording")
		}
		if !strings.Contains(m.View().Content, "Speech recognition is not configured") || !strings.Contains(m.View().Content, "typed draft") {
			t.Fatalf("got view %q", m.View().Content)
		}
		m, _ = rootUpdate(m, tea.PasteMsg{Content: " edited"})
		if !strings.Contains(m.View().Content, "typed draft edited") {
			t.Fatal("unavailable speech locked draft")
		}
	})
}
