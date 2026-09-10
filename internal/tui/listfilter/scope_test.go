package listfilter

import (
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

type item string

func (i item) FilterValue() string { return string(i) }

func TestScope_Commands(t *testing.T) {
	t.Run("preserves nested batches and non-filter messages without eagerly running children", func(t *testing.T) {
		rows := []list.Item{item("alpha"), item("beta")}
		model := list.New(rows, list.NewDefaultDelegate(), 80, 24)
		model.SetFilterText("alpha")
		filter := model.SetItems(rows) // An actual Bubbles asynchronous filter command.
		var scope Scope
		scope.Invalidate()
		marker := &struct{ value string }{"unrelated result"}
		calls := 0
		cmd := scope.wrap(tea.Batch(filter, tea.Batch(func() tea.Msg { calls++; return marker }, tea.Quit)))
		batch, ok := cmd().(tea.BatchMsg)
		if !ok || len(batch) != 2 || calls != 0 {
			t.Fatal("batch was hidden or its children executed eagerly")
		}
		reply, ok := batch[0]().(Reply)
		if !ok || !scope.Owns(reply) {
			t.Fatal("batch child lost filter ownership")
		}
		nested, ok := batch[1]().(tea.BatchMsg)
		if !ok || len(nested) != 2 || calls != 0 {
			t.Fatal("nested batch scheduling changed")
		}
		if got := nested[0](); got != marker || calls != 1 {
			t.Fatal("non-filter command was changed or executed more than once")
		}
		if _, ok := nested[1]().(tea.QuitMsg); !ok {
			t.Fatal("quit command was hidden")
		}
		if scope.wrap(nil) != nil {
			t.Fatal("nil command became executable")
		}
	})
	t.Run("commands keep their originating revision even when executed after invalidation", func(t *testing.T) {
		rows := []list.Item{item("alpha")}
		model := list.New(rows, list.NewDefaultDelegate(), 80, 24)
		model.SetFilterText("alpha")
		var scope, other Scope
		cmd := scope.SetItems(&model, rows)
		scope.Invalidate()
		other.Invalidate()
		reply := cmd().(Reply)
		if scope.Owns(reply) || other.Owns(reply) {
			t.Fatal("old command acquired a newer revision or another owner")
		}
	})
}
