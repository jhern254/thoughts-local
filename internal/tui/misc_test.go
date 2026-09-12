package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
)

func openMisc(t *testing.T, m Model) Model {
	t.Helper()
	m.subjects.list.Select(1)
	return runModelCommand(t, m, enterKey())
}

func TestModel_MiscThoughts(t *testing.T) {
	t.Run("quit keys preserve editor and filter input ownership", func(t *testing.T) {
		m := openMisc(t, filterRoot(t))
		_, cmd := rootUpdate(m, runeKey('q'))
		assertQuitCommand(t, cmd)
		for _, inputKey := range []tea.KeyPressMsg{enterKey(), runeKey('/')} {
			input, _ := rootUpdate(m, inputKey)
			input, cmd = rootUpdate(input, runeKey('q'))
			if cmd != nil {
				if _, quits := cmd().(tea.QuitMsg); quits {
					t.Fatal("q quit from text input")
				}
			}
			if strings.Contains(input.View().Content, "q: quit") {
				t.Fatal("input advertises q as quit")
			}
			_, cmd = rootUpdate(input, tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
			assertQuitCommand(t, cmd)
		}
	})
	t.Run("always shows a typed filterable entry without counting it as a subject", func(t *testing.T) {
		m := newSubjectTestModel(&subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) { return nil, nil }})
		for count, want := range []string{"0 thoughts", "1 thought", "2 thoughts"} {
			m.screen = screenEvents
			m.metrics = &metricsStub{miscCount: int64(count)}
			m = openSubjects(t, m)
			row := m.subjects.list.Items()[1].(subjectRow)
			if row.kind != subjectRowMisc || row.Title() != "Misc thoughts" || row.Description() != want || !strings.Contains(m.View().Content, "\n0 subjects\n") {
				t.Fatalf("got row %+v and view %q, want Misc with %s and zero subjects", row, m.View().Content, want)
			}
		}
		m.subjects.list.FilterInput.SetVirtualCursor(false)
		m, _ = rootUpdate(m, runeKey('/'))
		m, cmd := rootUpdate(m, tea.PasteMsg{Content: "Misc"})
		m, _ = rootUpdate(m, rootFilterReply(t, cmd))
		if rows := m.subjects.list.VisibleItems(); len(rows) != 1 || rows[0].(subjectRow).kind != subjectRowMisc {
			t.Fatalf("got filtered rows %v, want only Misc", rows)
		}
	})
	t.Run("creates unassigned text and refreshes counts without subject actions", func(t *testing.T) {
		ctx := context.Background()
		service := thought.NewService(testutils.NewFakeThoughtStore())
		counts := &metricsStub{}
		m := newScreenTestModel(ctx, &data.User{UserID: "u"}, &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) { return nil, nil }}, service, counts, logging.Nop())
		m = openMisc(t, openSubjects(t, m))
		for _, r := range "ed" {
			m, _ = rootUpdate(m, runeKey(r))
		}
		if m.screen != screenMiscThoughts || strings.Contains(m.View().Content, "edit subject") || strings.Contains(m.View().Content, "Added:") {
			t.Fatalf("got Misc view %q, want no subject controls", m.View().Content)
		}
		m, _ = rootUpdate(m, enterKey())
		const body = "edq\ncomplete draft"
		m, _ = rootUpdate(m, tea.PasteMsg{Content: body})
		m, cmd := rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
		m, changed := rootUpdate(m, cmd())
		m, _ = rootUpdate(m, changed())
		rows, err := service.ListUnassigned(ctx, "u")
		if err != nil || len(rows) != 1 || rows[0].SubjectID != nil || rows[0].Thought != body {
			t.Fatalf("got rows %v, error %v, want complete unassigned draft", rows, err)
		}
		if !strings.Contains(m.View().Content, "complete draft") {
			t.Fatal("saved detail not visible")
		}
		counts.miscCount = 1
		m = runModelCommand(t, m, escapeKey()) // Reload thought list after creation.
		m = runModelCommand(t, m, escapeKey()) // Refresh picker counts.
		if m.screen != screenSubjectList || m.subjects.list.Items()[1].(subjectRow).Description() != "1 thought" {
			t.Fatal("Misc count did not refresh")
		}
	})
	t.Run("Misc count failure preserves real counts and emits only approved metadata", func(t *testing.T) {
		m := filterRoot(t)
		counts := &metricsStub{counts: []data.SubjectThoughtCount{{SubjectID: 1, Count: 2}}, miscErr: errors.New("PRIVATE-MISC-MARKER")}
		m.metrics = counts
		var logs bytes.Buffer
		logger, err := logging.New(&logs, "test", "info")
		if err != nil {
			t.Fatal(err)
		}
		m.logger = logger
		m = runModelCommand(t, m, runeKey('r'))
		if m.subjects.list.Items()[1].(subjectRow).Description() != "Thought count unavailable" || m.subjects.list.Items()[2].(subjectRow).Description() != "2 thoughts" || m.subjects.err != counts.miscErr {
			t.Fatal("count failure lost independent results or original error")
		}
		var event map[string]any
		if err := json.Unmarshal(logs.Bytes(), &event); err != nil {
			t.Fatal(err)
		}
		if len(event) != 7 || event["operation"] != "thought_count_unassigned" || event["category"] != "unexpected_failure" || event["level"] != "error" || event["message"] != "operation failed" || event["application"] != "test" || event["caller"] == nil || event["time"] == nil {
			t.Fatalf("got unexpected fields %v", event)
		}
		if strings.Contains(logs.String()+m.View().Content, "PRIVATE-MISC-MARKER") {
			t.Fatal("private error escaped")
		}
		m = openMisc(t, m)
		if m.screen != screenMiscThoughts {
			t.Fatal("count failure blocked Misc")
		}
		m, _ = rootUpdate(m, escapeKey())
		counts.miscErr = nil
		counts.miscCount = 1
		m = runModelCommand(t, m, runeKey('r'))
		if m.subjects.err != nil || m.subjects.list.Items()[1].(subjectRow).Description() != "1 thought" {
			t.Fatal("Misc count did not recover")
		}
		counts.err = errors.New("subject count failed")
		m = runModelCommand(t, m, runeKey('r'))
		if m.subjects.list.Items()[1].(subjectRow).Description() != "1 thought" || m.subjects.list.Items()[2].(subjectRow).Description() != "Thought count unavailable" {
			t.Fatal("subject count failure erased Misc count")
		}
	})
}

func TestModel_MiscFilterOwnership(t *testing.T) {
	for _, tt := range []struct {
		name             string
		fromMisc, toMisc bool
	}{
		{"Misc reply cannot change subject rows", true, false},
		{"subject reply cannot change Misc rows", false, true},
		{"old Misc reply cannot change reopened Misc", true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			subjects := subject.NewService(testutils.NewFakeSubjectStore())
			if _, err := subjects.Create(ctx, "u", "subject"); err != nil {
				t.Fatal(err)
			}
			service := thought.NewService(testutils.NewFakeThoughtStore())
			id := int64(1)
			for _, scope := range []*int64{nil, &id} {
				for _, body := range []string{"alpha", "beta"} {
					if _, err := service.Create(ctx, "u", body, scope, time.Time{}); err != nil {
						t.Fatal(err)
					}
				}
			}
			m := openSubjects(t, newScreenTestModel(ctx, &data.User{UserID: "u"}, subjects, service, &metricsStub{}, logging.Nop()))
			if tt.fromMisc {
				m = openMisc(t, m)
			} else {
				m = rootOpenThoughts(t, m)
			}
			m, _ = rootUpdate(m, runeKey('/'))
			m, cmd := rootUpdate(m, tea.PasteMsg{Content: "alpha"})
			old := rootFilterReply(t, cmd)
			m, _ = rootUpdate(m, escapeKey())
			m, _ = rootUpdate(m, escapeKey())
			if tt.toMisc {
				m = openMisc(t, m)
			} else {
				m = rootOpenThoughts(t, m)
			}
			m, _ = rootUpdate(m, runeKey('/'))
			m, cmd = rootUpdate(m, tea.PasteMsg{Content: "beta"})
			m, _ = rootUpdate(m, rootFilterReply(t, cmd))
			m, _ = rootUpdate(m, enterKey())
			before := m.View().Content
			m, _ = rootUpdate(m, old)
			if got := m.View().Content; got != before {
				t.Fatalf("got changed view %q, want unchanged current filter view %q", got, before)
			}
			m = runModelCommand(t, m, enterKey())
			if got := m.View().Content; !strings.Contains(got, "beta") {
				t.Fatalf("got selected detail %q, want beta", got)
			}
		})
	}
}
