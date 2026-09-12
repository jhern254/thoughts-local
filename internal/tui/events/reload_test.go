package events

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
)

type blockedEventRead struct {
	Service
	started chan context.Context
}

func (s blockedEventRead) List(ctx context.Context, _ string, _, _ time.Time) ([]data.Event, error) {
	s.started <- ctx
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestModel_ReloadCancellation(t *testing.T) {
	t.Run("superseded commands do not invoke the list or count services", func(t *testing.T) {
		m, s, v := fixture(t)
		lists, counts := s.lists, v.counts
		old := m.reload()
		current := m.reload()
		m = execute(t, m, old)
		if s.lists != lists || v.counts != counts || !m.loading {
			t.Fatal("obsolete commands invoked services or cleared current loading")
		}
		m = execute(t, m, current)
		if m.loading || m.err != nil || s.lists != lists+1 || v.counts != counts+1 {
			t.Fatal("current reload did not complete normally")
		}
		m.Close()
	})
	t.Run("refresh and close cancel a read already in progress", func(t *testing.T) {
		for _, closeView := range []bool{false, true} {
			m, _, _ := fixture(t)
			started := make(chan context.Context, 1)
			m.service = blockedEventRead{Service: m.service, started: started}
			cmd := m.reload()().(tea.BatchMsg)[0]
			reply := make(chan tea.Msg, 1)
			go func() { reply <- cmd() }()
			ctx := <-started
			if closeView {
				m.Close()
			} else {
				_ = m.reload()
				defer m.Close()
			}
			if !errors.Is(ctx.Err(), context.Canceled) {
				t.Error("obsolete in-flight read was not canceled")
				// Bound the red test without stranding a goroutine.
				return
			}
			before := m.View()
			m = execute(t, m, func() tea.Msg { return <-reply })
			if m.View() != before {
				t.Fatal("canceled reply changed current presentation")
			}
		}
	})
	t.Run("late latest-preview command inherits its reload cancellation", func(t *testing.T) {
		m, _, _ := fixture(t)
		batch := m.reload()().(tea.BatchMsg)
		m, latest := m.Update(batch[0]())
		_ = m.reload()
		result := latest().(Latest)
		if !errors.Is(result.err, context.Canceled) {
			t.Fatalf("got latest error %v, want canceled", result.err)
		}
		m.Close()
	})
}
