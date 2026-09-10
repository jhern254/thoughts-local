package thoughts

import (
	"context"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/jhern254/go-thoughts/internal/tui/listfilter"
)

func filteringThoughts(t *testing.T) Model {
	t.Helper()
	service := thought.NewService(testutils.NewFakeThoughtStore())
	for _, body := range []string{"alpha", "beta"} {
		id := int64(1)
		if _, err := service.Create(context.Background(), "u", body, &id, time.Time{}); err != nil {
			t.Fatal(err)
		}
	}
	m := New(context.Background(), "u", service, logging.Nop())
	cmd := m.Open(1)
	m, _ = m.Update(cmd())
	m, _ = m.Update(key('/'))
	return m
}

// Execute the actual Bubbles commands, including their batched children, but
// retain the filter response for deliberate out-of-order delivery.
func thoughtFilterReply(t *testing.T, cmd tea.Cmd) tea.Msg {
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

func TestModel_FilterReplies(t *testing.T) {
	t.Run("current query can finish after the user accepts the filter", func(t *testing.T) {
		m := filteringThoughts(t)
		m, cmd := m.Update(tea.PasteMsg{Content: "beta"})
		m, _ = m.Update(key(tea.KeyEnter))
		m, _ = m.Update(thoughtFilterReply(t, cmd))
		if got := m.list.VisibleItems(); len(got) != 1 || got[0].(row).item.Thought != "beta" {
			t.Fatal("accepting the filter discarded its pending current reply")
		}
	})
	t.Run("reloading rows invalidates outstanding matches and scopes the replacement filter command", func(t *testing.T) {
		m := filteringThoughts(t)
		m, cmd := m.Update(tea.PasteMsg{Content: "beta"})
		old := thoughtFilterReply(t, cmd)
		m, _ = m.Update(key(tea.KeyEnter))
		id := int64(1)
		added, err := m.service.Create(context.Background(), "u", "beta newest", &id, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		m, cmd = m.Update(key('r'))
		m, cmd = m.Update(cmd())
		m, _ = m.Update(thoughtFilterReply(t, cmd))
		m.list.Select(1)
		m, _ = m.Update(old)
		selected, ok := m.list.SelectedItem().(row)
		if len(m.list.VisibleItems()) != 2 || !ok || selected.item.ThoughtID != added.ThoughtID {
			t.Fatal("old matches replaced reloaded rows or selection")
		}
	})
	t.Run("ignores an actual older query reply after a newer query is applied", func(t *testing.T) {
		m := filteringThoughts(t)
		m, cmd := m.Update(tea.PasteMsg{Content: "alpha"})
		old := thoughtFilterReply(t, cmd)
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'u', Mod: tea.ModCtrl}))
		m, cmd = m.Update(tea.PasteMsg{Content: "beta"})
		m, _ = m.Update(thoughtFilterReply(t, cmd))
		m, _ = m.Update(key(tea.KeyEnter))
		m, _ = m.Update(old)
		if got := m.list.VisibleItems(); len(got) != 1 || got[0].(row).item.Thought != "beta" {
			t.Fatalf("got visible rows %v, want beta only", got)
		}
		if m.list.SelectedItem().(row).item.Thought != "beta" {
			t.Fatal("older query changed selection")
		}
	})
	t.Run("ignores an actual reply from a previous opening of the same subject", func(t *testing.T) {
		m := filteringThoughts(t)
		m, cmd := m.Update(tea.PasteMsg{Content: "alpha"})
		old := thoughtFilterReply(t, cmd)
		cmd = m.Open(1)
		m, _ = m.Update(cmd())
		m, _ = m.Update(key('/'))
		m, cmd = m.Update(tea.PasteMsg{Content: "beta"})
		m, _ = m.Update(thoughtFilterReply(t, cmd))
		m, _ = m.Update(key(tea.KeyEnter))
		m, _ = m.Update(old)
		if got := m.list.VisibleItems(); len(got) != 1 || got[0].(row).item.Thought != "beta" {
			t.Fatalf("got visible rows %v, want current session beta", got)
		}
		if m.list.SelectedItem().(row).item.Thought != "beta" {
			t.Fatal("prior session changed selection")
		}
	})
}
