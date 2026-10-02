package browserterm

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestSessionCommands(t *testing.T) {
	t.Run("joins started batch commands and prevents queued work after shutdown", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		m := &sessionModel{ctx: ctx, cancel: cancel}
		started := make(chan struct{})
		release := make(chan struct{})
		cmd := m.wrap(tea.Batch(func() tea.Msg { close(started); <-ctx.Done(); <-release; return nil }, func() tea.Msg { return nil }))
		batch := cmd().(tea.BatchMsg)
		go batch[0]()
		<-started
		cancel()
		stopped := make(chan struct{})
		go func() { m.stop(); close(stopped) }()
		select {
		case <-stopped:
			t.Fatal("returned before active command released runtime")
		default:
		}
		close(release)
		select {
		case <-stopped:
		case <-time.After(time.Second):
			t.Fatal("did not join finished command")
		}
		queued := m.wrap(func() tea.Msg { t.Error("queued command started after shutdown"); return nil })
		queued()
	})
	t.Run("contains command panic without propagating authored value", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		m := &sessionModel{ctx: ctx, cancel: cancel}
		cmd := m.wrap(func() tea.Msg { panic("PRIVATE-PANIC-MARKER") })
		if got := cmd(); got != nil {
			t.Fatalf("got %T, want nil", got)
		}
		m.stop()
		if !m.failed.Load() || ctx.Err() == nil {
			t.Fatal("panic did not fail and cancel session")
		}
	})
}
