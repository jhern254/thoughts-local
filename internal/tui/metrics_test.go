package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
)

func TestSubjectModel_ThoughtCounts(t *testing.T) {
	t.Run("refreshes counts when creation notification arrives after returning to subjects", func(t *testing.T) {
		m := newSubjectTestModel(&subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) { return []data.Subject{{SubjectID: 1}}, nil }})
		m.metrics = &metricsStub{counts: []data.SubjectThoughtCount{{SubjectID: 1, Count: 1}}}
		m.screen = screenSubjectList
		updated, cmd := m.Update(thoughts.ChangedMsg{SubjectID: 1})
		m = updated.(Model)
		if cmd == nil {
			t.Fatal("late creation notification left visible counts stale")
		}
		updated, _ = m.Update(cmd())
		m = updated.(Model)
		if got := m.subjects.list.Items()[1].(subjectRow).Description(); got != "1 thought" {
			t.Fatalf("got %q, want refreshed count", got)
		}
	})
	t.Run("thought editor owns subject action keys until cancelled", func(t *testing.T) {
		m := newSubjectTestModel(&subjectServiceStub{})
		m.screen = screenSubjectDetail
		m.subjects.selected = &data.Subject{SubjectID: 1, SubjectName: "coding"}
		cmd := m.thoughts.Open(1)
		updated, _ := m.Update(cmd())
		m = updated.(Model)
		updated, _ = m.Update(enterKey())
		m = updated.(Model)
		for _, r := range "edq" {
			updated, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
			m = updated.(Model)
			if m.screen != screenSubjectDetail || !strings.Contains(m.View().Content, "Create thought") {
				t.Fatal("subject action stole editor input")
			}
		}
		updated, _ = m.Update(escapeKey())
		m = updated.(Model)
		updated, _ = m.Update(runeKey('e'))
		m = updated.(Model)
		if m.screen != screenSubjectEdit {
			t.Fatal("subject edit unavailable after cancel")
		}
	})
	t.Run("joins counts by subject ID with singular and plural descriptions", func(t *testing.T) {
		m := newSubjectTestModel(&subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) {
			return []data.Subject{{SubjectID: 1}, {SubjectID: 2}, {SubjectID: 3}}, nil
		}})
		m.metrics = &metricsStub{counts: []data.SubjectThoughtCount{{SubjectID: 3, Count: 12}, {SubjectID: 1, Count: 0}, {SubjectID: 2, Count: 1}}}
		m = openSubjects(t, m)
		for i, want := range []string{"0 thoughts", "1 thought", "12 thoughts"} {
			if got := m.subjects.list.Items()[i+1].(subjectRow).Description(); got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		}
	})
	t.Run("count failure keeps subjects usable and can be refreshed", func(t *testing.T) {
		var logs bytes.Buffer
		m := newSubjectTestModel(&subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) {
			return []data.Subject{{SubjectID: 1, SubjectName: "coding"}}, nil
		}})
		metrics := &metricsStub{err: errors.New("private-marker")}
		m.metrics = metrics
		logger, err := logging.New(&logs, "test", "info")
		if err != nil {
			t.Fatal(err)
		}
		m.logger = logger
		m = openSubjects(t, m)
		if len(m.subjects.list.Items()) != 2 || !strings.Contains(m.View().Content, "unavailable") || m.subjects.err != metrics.err {
			t.Fatal("failure discarded subjects or error")
		}
		var event map[string]any
		if err := json.Unmarshal(logs.Bytes(), &event); err != nil {
			t.Fatal(err)
		}
		if len(event) != 7 || event["operation"] != "thought_counts_by_subject" || event["category"] != "unexpected_failure" || event["level"] != "error" || event["application"] != "test" || event["time"] == nil || event["caller"] == nil || event["message"] != "operation failed" {
			t.Fatalf("unexpected event %v", event)
		}
		if strings.Contains(logs.String()+m.View().Content, "private-marker") {
			t.Fatal("private error escaped")
		}
		metrics.err = nil
		metrics.counts = []data.SubjectThoughtCount{{SubjectID: 1, Count: 2}}
		m = runModelCommand(t, m, runeKey('r'))
		if m.subjects.err != nil || m.subjects.list.Items()[1].(subjectRow).Description() != "2 thoughts" {
			t.Fatal("counts did not recover")
		}
	})
}
