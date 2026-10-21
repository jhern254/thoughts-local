//go:build integration

package integration_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/tui"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
	"github.com/jhern254/go-thoughts/internal/voice"
)

func TestVoiceSubjectWorkflow_SQLite(t *testing.T) {
	for _, scenario := range []string{"exact alias", "unknown Subject creation", "no command remains Misc"} {
		t.Run(scenario, func(t *testing.T) {
			db, dsn := openMigratedSQLite(t)
			runtime, err := application.Open(t.Context(), dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer runtime.Close()
			user := runtime.LocalUser().UserID
			canonical, err := runtime.Subjects().Create(t.Context(), user, "Go Programming")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.Subjects().CreateAlias(t.Context(), user, canonical.SubjectID, "Programming in Go"); err != nil {
				t.Fatal(err)
			}
			for range 3 {
				if _, err := runtime.Thoughts().Create(t.Context(), user, "Existing history", &canonical.SubjectID, time.Time{}); err != nil {
					t.Fatal(err)
				}
			}
			model := tui.NewModel(t.Context(), runtime.LocalUser(), runtime.Subjects(), runtime.Thoughts(), runtime.Metrics(), runtime.Events(), runtime.TimelineView(), logging.Nop())
			update := func(msg tea.Msg) tea.Cmd { next, cmd := model.Update(msg); model = next.(tui.Model); return cmd }
			// Execute bounded data/focus batches; recurring cursor ticks returned by Update
			// are irrelevant to this workflow and are deliberately not scheduled.
			var execute func(tea.Cmd)
			execute = func(cmd tea.Cmd) {
				if cmd == nil {
					return
				}
				msg := cmd()
				if batch, ok := msg.(tea.BatchMsg); ok {
					for _, child := range batch {
						execute(child)
					}
					return
				}
				update(msg)
			}
			update(thoughts.BrowserVoiceSessionMsg{SessionID: "synthetic-session", SpeechRecognitionAvailable: true})
			openedBefore := time.Now().UTC()
			execute(update(tea.KeyPressMsg{Code: tea.KeyF8}))
			openedAfter := time.Now().UTC()
			state := model.VoiceState()
			if state.TranscriptAuthority == nil {
				t.Fatal("F8 did not start recording")
			}
			key := state.TranscriptAuthority.Recording()
			cue := "Programming in Go"
			subjectName := "Go Programming"
			if scenario == "unknown Subject creation" {
				cue = "Machine Learning Research"
				subjectName = cue
			}
			full := cue + " thought, today I learned about interfaces."
			expectedBody := "Today I learned about interfaces."
			if scenario == "no command remains Misc" {
				full = "I thought about learning Go today."
				expectedBody = full
				subjectName = ""
			}
			if scenario != "no command remains Misc" {
				update(voice.TranscriptUpdate{Recording: key, Revision: 1, Text: cue + " thought"})
			}
			update(voice.TranscriptUpdate{Recording: key, Revision: 2, Text: full})
			view := ansi.Strip(model.View().Content)
			if !strings.Contains(view, subjectName) || !strings.Contains(view, full) {
				t.Fatalf("live view got %q, want canonical/cue and full transcript", view)
			}
			execute(update(tea.KeyPressMsg{Code: tea.KeyF8}))
			update(voice.TranscriptUpdate{Recording: key, Revision: 3, Text: "late wrong Subject thought incorrect"})
			execute(update(voice.RecordingEnded{Recording: key}))
			var count int
			if err := db.QueryRow("SELECT COUNT(*) FROM thoughts").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if got, want := count, 3; got != want {
				t.Fatalf("Thoughts before Save: got %d want %d", got, want)
			}
			expectedSubject := &canonical.SubjectID
			if scenario == "no command remains Misc" {
				expectedSubject = nil
			}
			if scenario == "unknown Subject creation" {
				execute(update(tea.KeyPressMsg{Code: tea.KeyTab}))
				execute(update(tea.KeyPressMsg{Code: tea.KeyEnter}))
				if !strings.Contains(ansi.Strip(model.View().Content), "Create subject") {
					t.Fatal("existing Subject creation not opened")
				}
				execute(update(tea.KeyPressMsg{Code: tea.KeyEnter}))
				created, err := runtime.Subjects().ResolveName(t.Context(), user, subjectName)
				if err != nil {
					t.Fatal(err)
				}
				expectedSubject = &created.SubjectID
			}
			execute(update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}))
			reopened := openSQLite(t, dsn)
			var body string
			var subjectID *int64
			var observed int64
			if err := reopened.QueryRow("SELECT thought,subject_id,observed_at FROM thoughts ORDER BY thought_id DESC LIMIT 1").Scan(&body, &subjectID, &observed); err != nil {
				t.Fatal(err)
			}
			if got, want := body, expectedBody; got != want {
				t.Fatalf("saved text: got %q want %q", got, want)
			}
			if (subjectID == nil) != (expectedSubject == nil) || (subjectID != nil && *subjectID != *expectedSubject) {
				t.Fatalf("saved Subject: got %v want %v", subjectID, expectedSubject)
			}
			if observed < openedBefore.Unix() || observed > openedAfter.Unix() {
				t.Fatalf("observed_at: got %d want draft opening interval [%d,%d]", observed, openedBefore.Unix(), openedAfter.Unix())
			}
			resolved, err := runtime.Subjects().ResolveName(t.Context(), user, "Programming in Go")
			if err != nil || resolved.SubjectID != canonical.SubjectID {
				t.Fatalf("existing alias: got %+v,%v want unchanged canonical mapping", resolved, err)
			}
		})
	}
}
