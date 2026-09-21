package tui

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
)

type subjectEventWriter struct {
	homeEventStub
	creates   int
	subjectID *int64
	activity  string
	started   time.Time
}

func (s *subjectEventWriter) Create(_ context.Context, _ string, activity string, at time.Time, id *int64) (*data.Event, error) {
	s.creates++
	s.subjectID = id
	s.activity = activity
	s.started = at
	return nil, errors.New("controlled event write failure")
}

func eventSubjectModel(t *testing.T, subjects SubjectService) (Model, *subjectEventWriter) {
	t.Helper()
	writer := &subjectEventWriter{}
	m := NewModel(t.Context(), &data.User{UserID: "u"}, subjects, thought.NewService(testutils.NewFakeThoughtStore()), &metricsStub{}, writer, &homeTimelineStub{}, logging.Nop())
	m.events.Open() // Timer replies are deliberately controlled by the tests.
	m, _ = rootUpdate(m, runeKey('n'))
	m, _ = rootUpdate(m, tea.PasteMsg{Content: "preserved activity"})
	m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	return m, writer
}
func TestModel_EventSubjectCreation(t *testing.T) {
	t.Run("creation returns to preserved draft selects subject and ignores older replies", func(t *testing.T) {
		service := &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) { return nil, nil }, create: func(_ context.Context, user, name string) (*data.Subject, error) {
			return &data.Subject{SubjectID: 42, UserID: user, SubjectName: name}, nil
		}}
		m, writer := eventSubjectModel(t, service)
		// Obtain a real list command from another fresh form, then retain its reply
		// while creation runs in that same form.
		m, _ = rootUpdate(m, escapeKey())
		var cmd tea.Cmd
		m, cmd = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
		batch := cmd().(tea.BatchMsg)
		oldList := batch[1]()
		m, _ = rootUpdate(m, tea.PasteMsg{Content: "preserved activity"})
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: 'a', Mod: tea.ModCtrl}))
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: 'k', Mod: tea.ModCtrl}))
		m, _ = rootUpdate(m, tea.PasteMsg{Content: "2026-09-11 11:30:00 AM"})
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
		m, _ = rootUpdate(m, tea.PasteMsg{Content: "New subject"})
		m = runModelCommand(t, m, enterKey())
		if m.screen != screenSubjectCreate || m.subjects.input.Value() != "New subject" || service.createCalls != 0 || writer.creates != 0 {
			t.Fatal("creation navigation submitted or lost prefill")
		}
		m, cmd = rootUpdate(m, enterKey())
		result := cmd()
		m, _ = rootUpdate(m, result)
		m, _ = rootUpdate(m, oldList)
		if m.screen != screenEvents || !m.subjects.listStale || !strings.Contains(m.events.View(), "New subject") || writer.creates != 0 {
			t.Fatal("creation did not resume event draft")
		}
		m, cmd = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
		saved := cmd()
		m, _ = rootUpdate(m, saved)
		if writer.subjectID == nil || *writer.subjectID != 42 || writer.activity != "preserved activity" || !writer.started.Equal(time.Date(2026, 9, 11, 18, 30, 0, 0, time.UTC)) {
			t.Fatalf("got event write %+v, want preserved draft and subject 42", writer)
		}
		// A duplicate old creation result cannot navigate out of the event form.
		m, _ = rootUpdate(m, result)
		if m.screen != screenEvents || service.createCalls != 1 {
			t.Fatal("stale creation result changed navigation")
		}
		m, _ = rootUpdate(m, escapeKey())
		if service.createCalls != 1 {
			t.Fatal("canceling event must leave created subject independent")
		}
	})
	t.Run("cancel restores exact draft and subject focus without writes", func(t *testing.T) {
		service := &subjectServiceStub{}
		m, writer := eventSubjectModel(t, service)
		m, _ = rootUpdate(m, tea.PasteMsg{Content: "unchanged query"})
		before := ansi.Strip(m.events.View())
		m = runModelCommand(t, m, enterKey())
		m, _ = rootUpdate(m, tea.PasteMsg{Content: " edited only in creation"})
		m, _ = rootUpdate(m, escapeKey())
		if got := ansi.Strip(m.events.View()); got != before {
			t.Fatalf("got restored view:\n%s\nwant:\n%s", got, before)
		}
		if m.screen != screenEvents || service.createCalls != 0 || writer.creates != 0 {
			t.Fatal("cancel wrote or did not return")
		}
		// Enter still belongs to subject suggestions after cancellation.
		m = runModelCommand(t, m, enterKey())
		if m.screen != screenSubjectCreate || m.subjects.input.Value() != "unchanged query" {
			t.Fatal("cancel lost Subject focus or query")
		}
	})
	for _, failure := range []struct {
		name   string
		err    error
		logged bool
	}{
		{"duplicate", data.ErrDuplicateRecord, false},
		{"validation", &subject.ValidationError{}, false},
		{"unexpected", errors.New("PRIVATE_SUBJECT_FAILURE"), true},
	} {
		t.Run(failure.name+" preserves subject input and event draft with safe feedback", func(t *testing.T) {
			service := &subjectServiceStub{create: func(context.Context, string, string) (*data.Subject, error) { return nil, failure.err }}
			m, writer := eventSubjectModel(t, service)
			var logs bytes.Buffer
			logger, err := logging.New(&logs, "test", "info")
			if err != nil {
				t.Fatal(err)
			}
			m.logger = logger
			m, _ = rootUpdate(m, tea.PasteMsg{Content: "PRIVATE_SUBJECT_NAME"})
			before := ansi.Strip(m.events.View())
			m = runModelCommand(t, m, enterKey())
			m = runModelCommand(t, m, enterKey())
			if m.screen != screenSubjectCreate || m.subjects.input.Value() != "PRIVATE_SUBJECT_NAME" || !errors.Is(m.subjects.err, failure.err) || writer.creates != 0 {
				t.Fatal("failure lost input, error identity, or destination")
			}
			if strings.Contains(m.View().Content, "PRIVATE_SUBJECT_FAILURE") || strings.Contains(logs.String(), "PRIVATE_SUBJECT") {
				t.Fatal("private content entered diagnostics or operational logs")
			}
			wantLogs := 0
			if failure.logged {
				wantLogs = 1
			}
			if got := strings.Count(logs.String(), "\n"); got != wantLogs {
				t.Fatalf("got %d logs, want %d", got, wantLogs)
			}
			m, _ = rootUpdate(m, escapeKey())
			if got := ansi.Strip(m.events.View()); got != before {
				t.Fatalf("got draft:\n%s\nwant:\n%s", got, before)
			}
		})
	}
	t.Run("stale creation failure cannot replace a newer creation session", func(t *testing.T) {
		service := &subjectServiceStub{create: func(context.Context, string, string) (*data.Subject, error) { return nil, data.ErrDuplicateRecord }}
		m, _ := eventSubjectModel(t, service)
		m = runModelCommand(t, m, enterKey())
		m, cmd := rootUpdate(m, enterKey())
		old := cmd()
		m, _ = rootUpdate(m, old)
		m, _ = rootUpdate(m, escapeKey())
		m = runModelCommand(t, m, enterKey())
		m, _ = rootUpdate(m, old)
		if m.subjects.err != nil || m.screen != screenSubjectCreate {
			t.Fatal("old creation failure entered newer session")
		}
	})
}

func TestModel_EventSubjectAsyncOwnership(t *testing.T) {
	t.Run("ordinary subject list reply cannot interrupt pending creation", func(t *testing.T) {
		service := &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) { return nil, nil }, create: func(context.Context, string, string) (*data.Subject, error) {
			return &data.Subject{SubjectID: 7, SubjectName: "created"}, nil
		}}
		m, _ := eventSubjectModel(t, service)
		old := m.listSubjects()()
		m = runModelCommand(t, m, enterKey())
		m, cmd := rootUpdate(m, enterKey())
		m, _ = rootUpdate(m, old)
		if !m.subjects.loading || m.screen != screenSubjectCreate {
			t.Fatal("ordinary list reply interrupted pending creation")
		}
		m, _ = rootUpdate(m, cmd())
		m, _ = rootUpdate(m, old)
		if !m.subjects.listStale || m.screen != screenEvents {
			t.Fatal("old ordinary list cleared stale state after creation")
		}
	})
}

func TestModel_EventSubjectCreationResize(t *testing.T) {
	t.Run("long prefilled name and cursor fit a narrow creation screen", func(t *testing.T) {
		m, _ := eventSubjectModel(t, &subjectServiceStub{})
		m, _ = rootUpdate(m, tea.PasteMsg{Content: strings.Repeat("界", 30)})
		m = runModelCommand(t, m, enterKey())
		cursor := m.subjects.input.Position()
		m, _ = rootUpdate(m, tea.WindowSizeMsg{Width: 32, Height: 24})
		for _, row := range strings.Split(m.View().Content, "\n") {
			if width := ansi.StringWidth(row); width > 32 {
				t.Fatalf("got row width %d, want <=32: %q", width, row)
			}
		}
		if m.subjects.input.Position() != cursor || m.subjects.input.Value() != strings.Repeat("界", 30) {
			t.Fatal("resize changed input or cursor")
		}
	})
}
