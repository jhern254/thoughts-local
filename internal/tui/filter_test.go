package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/jhern254/go-thoughts/internal/tui/listfilter"
)

func filterRoot(t *testing.T) Model {
	t.Helper()
	ctx := context.Background()
	subjects := subject.NewService(testutils.NewFakeSubjectStore())
	thoughts := thought.NewService(testutils.NewFakeThoughtStore())
	for _, name := range []string{"alpha", "beta"} {
		if _, err := subjects.Create(ctx, "u", name); err != nil {
			t.Fatal(err)
		}
	}
	id := int64(1)
	for _, body := range []string{"alpha thought", "beta thought"} {
		if _, err := thoughts.Create(ctx, "u", body, &id, time.Time{}); err != nil {
			t.Fatal(err)
		}
	}
	return openSubjects(t, NewModel(ctx, &data.User{UserID: "u"}, subjects, thoughts, &metricsStub{}, logging.Nop()))
}

func rootFilterReply(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	var find func(tea.Cmd) tea.Msg
	find = func(cmd tea.Cmd) tea.Msg {
		if cmd == nil {
			return nil
		}
		msg := cmd()
		switch msg := msg.(type) {
		case list.FilterMatchesMsg, listfilter.Reply:
			return msg
		case tea.BatchMsg:
			var found tea.Msg
			for _, child := range msg {
				if result := find(child); result != nil {
					found = result
				}
			}
			return found
		}
		return nil
	}
	msg := find(cmd)
	if msg == nil {
		t.Fatal("command did not produce a filter result")
	}
	return msg
}

func rootUpdate(m Model, msg tea.Msg) (Model, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(Model), cmd
}

func rootOpenThoughts(t *testing.T, m Model) Model {
	t.Helper()
	m.subjects.list.Select(1)
	m, cmd := rootUpdate(m, enterKey())
	if cmd == nil {
		t.Fatal("missing subject get")
	}
	m, cmd = rootUpdate(m, cmd())
	if cmd == nil {
		t.Fatal("missing thought list")
	}
	m, _ = rootUpdate(m, cmd())
	return m
}

func TestModel_FilterRouting(t *testing.T) {
	t.Run("superseded subject queries cannot change active rows and selection", func(t *testing.T) {
		m := filterRoot(t)
		m, _ = rootUpdate(m, runeKey('/'))
		m, cmd := rootUpdate(m, tea.PasteMsg{Content: "alpha"})
		old := rootFilterReply(t, cmd)
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: 'u', Mod: tea.ModCtrl}))
		m, cmd = rootUpdate(m, tea.PasteMsg{Content: "beta"})
		m, _ = rootUpdate(m, rootFilterReply(t, cmd))
		m, _ = rootUpdate(m, enterKey())
		m, _ = rootUpdate(m, old)
		rows := m.subjects.list.VisibleItems()
		if len(rows) != 1 || rows[0].(subjectRow).subject.SubjectID != 2 || m.subjects.list.SelectedItem().(subjectRow).subject.SubjectID != 2 {
			t.Fatal("older query replaced subject rows or selection")
		}
	})
	t.Run("previous thought session cannot change the reopened session through root dispatch", func(t *testing.T) {
		m := rootOpenThoughts(t, filterRoot(t))
		m, _ = rootUpdate(m, runeKey('/'))
		m, cmd := rootUpdate(m, tea.PasteMsg{Content: "alpha"})
		old := rootFilterReply(t, cmd)
		m, _ = rootUpdate(m, escapeKey())
		m, _ = rootUpdate(m, escapeKey())
		m = rootOpenThoughts(t, m)
		m, _ = rootUpdate(m, runeKey('/'))
		m, cmd = rootUpdate(m, tea.PasteMsg{Content: "beta"})
		m, _ = rootUpdate(m, rootFilterReply(t, cmd))
		m, _ = rootUpdate(m, enterKey())
		before := m.View().Content
		m, _ = rootUpdate(m, old)
		if m.View().Content != before {
			t.Fatal("old session changed current thought rows")
		}
		m = runModelCommand(t, m, enterKey())
		if !strings.Contains(m.View().Content, "Thought 2") {
			t.Fatal("old session changed current thought selection")
		}
	})
	t.Run("delayed subject filter cannot replace active thought rows or selection", func(t *testing.T) {
		m := filterRoot(t)
		m, _ = rootUpdate(m, runeKey('/'))
		m, cmd := rootUpdate(m, tea.PasteMsg{Content: "alpha"})
		old := rootFilterReply(t, cmd)
		m, _ = rootUpdate(m, enterKey()) // Accept while the current filter is still pending.
		m = rootOpenThoughts(t, m)
		m, _ = rootUpdate(m, runeKey('/'))
		m, cmd = rootUpdate(m, tea.PasteMsg{Content: "beta"})
		m, _ = rootUpdate(m, rootFilterReply(t, cmd))
		m, _ = rootUpdate(m, enterKey())
		before := m.View().Content
		m, _ = rootUpdate(m, old)
		if m.View().Content != before {
			t.Fatalf("foreign filter reply changed active thought rows: before %q, after %q", before, m.View().Content)
		}
		if rows := m.subjects.list.VisibleItems(); len(rows) != 1 || rows[0].(subjectRow).subject.SubjectID != 1 {
			t.Fatal("valid reply did not reach its own hidden subject list")
		}
		m = runModelCommand(t, m, enterKey())
		if !strings.Contains(m.View().Content, "Thought 2") {
			t.Fatal("foreign filter reply changed thought selection")
		}
	})
	t.Run("delayed thought filter cannot replace active subject rows or selection", func(t *testing.T) {
		m := rootOpenThoughts(t, filterRoot(t))
		m, _ = rootUpdate(m, runeKey('/'))
		m, cmd := rootUpdate(m, tea.PasteMsg{Content: "alpha"})
		old := rootFilterReply(t, cmd)
		m, _ = rootUpdate(m, escapeKey())
		m, _ = rootUpdate(m, escapeKey())
		m, _ = rootUpdate(m, runeKey('/'))
		m, cmd = rootUpdate(m, tea.PasteMsg{Content: "beta"})
		m, _ = rootUpdate(m, rootFilterReply(t, cmd))
		m, _ = rootUpdate(m, enterKey())
		before := m.View().Content
		m, _ = rootUpdate(m, old)
		if m.View().Content != before {
			t.Fatal("foreign filter reply changed visible subjects")
		}
		selected, ok := m.subjects.list.SelectedItem().(subjectRow)
		if !ok || selected.subject.SubjectID != 2 {
			t.Fatalf("got selection %#v, want subject 2", m.subjects.list.SelectedItem())
		}
	})
}
