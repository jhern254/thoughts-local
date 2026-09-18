package events

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
)

func TestModel_MessageOwnership(t *testing.T) {
	t.Run("real day replies belong to Events but another model cannot accept them", func(t *testing.T) {
		m, _, _ := fixture(t)
		other, _, _ := fixture(t)
		defer m.Close()
		defer other.Close()
		batch := m.loadDay(m.day)().(tea.BatchMsg)
		if Owns(batch) {
			t.Fatal("Bubble Tea must retain ownership of batch scheduling")
		}
		before := other.View()
		for _, cmd := range batch[:2] {
			reply := cmd()
			if !Owns(reply) {
				t.Fatalf("Events did not recognize its %T reply", reply)
			}
			var next tea.Cmd
			other, next = other.Update(reply)
			if next != nil || other.View() != before {
				t.Fatal("message family membership bypassed model identity")
			}
		}
	})
	t.Run("shared thought results and terminal messages keep their own routing", func(t *testing.T) {
		for _, msg := range []tea.Msg{thoughts.Result{}, thoughts.BrowseThoughtsResult{}, thoughts.ThoughtCountResult{}, eventKey("enter"), tea.WindowSizeMsg{}, nil} {
			if Owns(msg) {
				t.Fatalf("Events claimed shared message %T", msg)
			}
		}
	})
}
