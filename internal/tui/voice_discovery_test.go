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

func TestModel_VoiceSubjectDiscovery(t *testing.T) {
	t.Run("plural revision freezes at Stop and F9 restores the accepted prefix", func(t *testing.T) {
		service := subject.NewService(testutils.NewFakeSubjectStore())
		if _, err := service.Create(t.Context(), "local-user-id", "Fitness"); err != nil {
			t.Fatal(err)
		}
		model := newSubjectTestModel(service)
		model, _ = rootUpdate(model, thoughts.BrowserVoiceSessionMsg{SessionID: "session", SpeechRecognitionAvailable: true})
		model = runModelCommand(t, model, tea.KeyPressMsg{Code: tea.KeyF8})
		recording := model.VoiceState().TranscriptAuthority.Recording()
		model, _ = rootUpdate(model, voice.TranscriptUpdate{Recording: recording, Revision: 1, Text: "Fitness thought： étudier."})
		finalTranscript := "Fitness thoughts： étudier."
		model, _ = rootUpdate(model, voice.TranscriptUpdate{Recording: recording, Revision: 2, Text: finalTranscript})
		model, _ = rootUpdate(model, tea.KeyPressMsg{Code: tea.KeyF8})
		model, _ = rootUpdate(model, voice.TranscriptUpdate{Recording: recording, Revision: 3, Text: "Fitness thought: rejected late revision"})
		if view := ansi.Strip(model.View().Content); !strings.Contains(view, finalTranscript) || strings.Contains(view, "rejected late revision") {
			t.Fatalf("stopped view = %q, want latest accepted plural transcript", view)
		}
		model, _ = rootUpdate(model, voice.RecordingEnded{Recording: recording})
		model, _ = rootUpdate(model, voice.RecordingEnded{Recording: recording})
		if view := ansi.Strip(model.View().Content); !strings.Contains(view, "Étudier.") || strings.Contains(view, "Fitness thoughts") || !strings.Contains(view, "F9: Restore prefix") {
			t.Fatalf("ended view = %q, want cleaned body and restore action", view)
		}
		model, _ = rootUpdate(model, tea.KeyPressMsg{Code: tea.KeyF9})
		model, _ = rootUpdate(model, tea.KeyPressMsg{Code: tea.KeyF9})
		if view := ansi.Strip(model.View().Content); strings.Count(view, finalTranscript) != 1 || strings.Contains(view, "F9: Restore prefix") {
			t.Fatalf("restored view = %q, want original plural transcript exactly once", view)
		}
	})

	t.Run("manual independent Subject creation keeps alias conflict confirmation", func(t *testing.T) {
		service := subject.NewService(testutils.NewFakeSubjectStore())
		original, err := service.Create(t.Context(), "local-user-id", "Go Programming")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.CreateAlias(t.Context(), "local-user-id", original.SubjectID, "Golang"); err != nil {
			t.Fatal(err)
		}
		model := newSubjectTestModel(service)
		model, _ = rootUpdate(model, thoughts.BrowserVoiceSessionMsg{SessionID: "session", SpeechRecognitionAvailable: true})
		model = runModelCommand(t, model, tea.KeyPressMsg{Code: tea.KeyF8})
		recording := model.VoiceState().TranscriptAuthority.Recording()
		model, _ = rootUpdate(model, voice.TranscriptUpdate{Recording: recording, Revision: 1, Text: "Golang thought, learn interfaces."})
		if !strings.Contains(model.View().Content, "Go Programming") {
			t.Fatal("alias did not recommend canonical Subject")
		}
		model, _ = rootUpdate(model, tea.KeyPressMsg{Code: tea.KeyF8})
		model, _ = rootUpdate(model, voice.RecordingEnded{Recording: recording})
		model, _ = rootUpdate(model, tea.KeyPressMsg{Code: tea.KeyTab})
		model, _ = rootUpdate(model, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
		model, _ = rootUpdate(model, tea.PasteMsg{Content: "Golang"})
		model = runModelCommand(t, model, enterKey())
		model = runModelCommand(t, model, enterKey())
		if !strings.Contains(model.View().Content, "d: remove alias and create") {
			t.Fatal("alias conflict flow missing")
		}
		model, _ = rootUpdate(model, runeKey('d'))
		model, _ = rootUpdate(model, escapeKey())
		aliases, err := service.ListAliases(t.Context(), "local-user-id", original.SubjectID)
		if err != nil || len(aliases) != 1 {
			t.Fatal("cancel changed alias")
		}
		model = runModelCommand(t, model, enterKey())
		model, _ = rootUpdate(model, runeKey('d'))
		model = runModelCommand(t, model, runeKey('y'))
		if model.screen != screenVoiceThought || !strings.Contains(model.View().Content, "Learn interfaces.") {
			t.Fatalf("confirmation returned screen=%v view=%q", model.screen, model.View().Content)
		}
		resolved, err := service.ResolveName(t.Context(), "local-user-id", "Golang")
		if err != nil || resolved.SubjectID == original.SubjectID {
			t.Fatalf("got %+v,%v want independent Subject", resolved, err)
		}
	})
	t.Run("ambiguous recommendation is selectable and editor fits narrow widths", func(t *testing.T) {
		service := subject.NewService(testutils.NewFakeSubjectStore())
		for _, name := range []string{"coding practice", "coDing practice", "Coding practice"} {
			if _, err := service.Create(t.Context(), "local-user-id", name); err != nil {
				t.Fatal(err)
			}
		}
		model := newSubjectTestModel(service)
		model, _ = rootUpdate(model, thoughts.BrowserVoiceSessionMsg{SessionID: "session", SpeechRecognitionAvailable: true})
		model = runModelCommand(t, model, tea.KeyPressMsg{Code: tea.KeyF8})
		recording := model.VoiceState().TranscriptAuthority.Recording()
		model, _ = rootUpdate(model, voice.TranscriptUpdate{Recording: recording, Revision: 1, Text: "CODING practice skills thought practice"})
		for _, width := range []int{80, 32} {
			model, _ = rootUpdate(model, tea.WindowSizeMsg{Width: width, Height: 24})
			view := model.View().Content
			if !strings.Contains(view, "ambiguous") {
				t.Fatalf("got %q want ambiguity label", view)
			}
			for _, line := range strings.Split(view, "\n") {
				if ansi.StringWidth(line) > width {
					t.Fatalf("row width=%d want <=%d: %q", ansi.StringWidth(line), width, line)
				}
			}
		}
		model, _ = rootUpdate(model, voice.RecordingEnded{Recording: recording})
		model, _ = rootUpdate(model, tea.KeyPressMsg{Code: tea.KeyTab})
		model, _ = rootUpdate(model, tea.KeyPressMsg{Code: tea.KeyDown})
		model, _ = rootUpdate(model, enterKey())
		if !strings.Contains(model.View().Content, "Subject (optional)") {
			t.Fatal("explicit selection still marked provisional")
		}
	})
	t.Run("candidate reply arriving during Subject creation stays owned by original draft", func(t *testing.T) {
		service := subject.NewService(testutils.NewFakeSubjectStore())
		if _, err := service.Create(t.Context(), "local-user-id", "Fitness"); err != nil {
			t.Fatal(err)
		}
		model := newSubjectTestModel(service)
		model, _ = rootUpdate(model, thoughts.BrowserVoiceSessionMsg{SessionID: "session", SpeechRecognitionAvailable: true})
		var cmd tea.Cmd
		model, cmd = rootUpdate(model, tea.KeyPressMsg{Code: tea.KeyF8})
		reply := cmd()
		r := model.VoiceState().TranscriptAuthority.Recording()
		model, _ = rootUpdate(model, voice.TranscriptUpdate{Recording: r, Revision: 1, Text: "Fitness thought hello"})
		model, _ = rootUpdate(model, voice.RecordingEnded{Recording: r})
		model, _ = rootUpdate(model, tea.KeyPressMsg{Code: tea.KeyTab})
		model = runModelCommand(t, model, enterKey())
		model, _ = rootUpdate(model, reply)
		model, _ = rootUpdate(model, escapeKey())
		model, _ = rootUpdate(model, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
		model, _ = rootUpdate(model, tea.KeyPressMsg{Code: tea.KeyDown})
		model, _ = rootUpdate(model, enterKey())
		if !strings.Contains(model.View().Content, "Fitness") {
			t.Fatal("late reply was lost while creation owned root")
		}
	})
}
