// Package listfilter scopes Bubbles' asynchronous filter replies to the list
// revision that requested them. Other Bubble Tea messages remain unchanged.
package listfilter

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

type Scope struct{ revision *byte }

type Reply struct {
	revision *byte
	matches  list.FilterMatchesMsg
}

// Invalidate starts a fresh revision, also separating independent list owners.
// Commands capture the token by value; they never read mutable UI state.
func (s *Scope) Invalidate() { s.revision = new(byte) }

func (s Scope) Owns(reply Reply) bool { return s.revision != nil && reply.revision == s.revision }

func (s *Scope) Update(model *list.Model, msg tea.Msg) tea.Cmd {
	switch reply := msg.(type) {
	case Reply:
		if !s.Owns(reply) {
			return nil
		}
		msg = reply.matches
	case list.FilterMatchesMsg:
		return nil // An unowned reply must never enter a list.
	}
	before, state := model.FilterValue(), model.FilterState()
	updated, cmd := model.Update(msg)
	*model = updated
	if s.revision == nil || before != model.FilterValue() || (state != list.Unfiltered && model.FilterState() == list.Unfiltered) {
		s.Invalidate()
	}
	return s.wrap(cmd)
}

func (s *Scope) SetItems(model *list.Model, items []list.Item) tea.Cmd {
	s.Invalidate()
	return s.wrap(model.SetItems(items))
}

func (s Scope) wrap(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		switch msg := cmd().(type) {
		case list.FilterMatchesMsg:
			return Reply{revision: s.revision, matches: msg}
		case tea.BatchMsg:
			// Preserve normal Bubble Tea scheduling: wrap child commands, don't run
			// them here or hide the BatchMsg inside a component envelope.
			batch := make(tea.BatchMsg, len(msg))
			for i, child := range msg {
				batch[i] = s.wrap(child)
			}
			return batch
		default:
			return msg
		}
	}
}
