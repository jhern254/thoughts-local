package events

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/timeline"
)

type cancellationSubjects struct {
	read func(context.Context) ([]data.Subject, error)
}

func (s cancellationSubjects) List(ctx context.Context, _ string) ([]data.Subject, error) {
	return s.read(ctx)
}

type cancellationTimeline struct {
	TimelineView
	read func(context.Context) (timeline.ThoughtScope, error)
}

func (s cancellationTimeline) OpenThoughtsView(ctx context.Context, _ string, _ int64) (timeline.ThoughtScope, error) {
	return s.read(ctx)
}

func TestModel_RequestCancellation(t *testing.T) {
	t.Run("closing a form prevents its queued subject read", func(t *testing.T) {
		m, _, _ := fixture(t)
		m.subjectReader = cancellationSubjects{read: func(context.Context) ([]data.Subject, error) {
			t.Error("closed form called subject service")
			return nil, nil
		}}
		batch := m.startForm(false)().(tea.BatchMsg)
		m.updateForm(eventKey("esc"))
		if got := batch[1](); got != nil {
			t.Fatalf("got %T, want no closed form reply", got)
		}
	})
	t.Run("replacing an expansion cancels its running read", func(t *testing.T) {
		m, _, _ := fixture(t)
		defer m.Close()
		started := make(chan context.Context, 1)
		m.timelineView = cancellationTimeline{TimelineView: m.timelineView, read: func(ctx context.Context) (timeline.ThoughtScope, error) {
			started <- ctx
			<-ctx.Done()
			return timeline.ThoughtScope{}, ctx.Err()
		}}
		cmd := m.openEvent(1)
		done := make(chan tea.Msg, 1)
		go func() { done <- cmd() }()
		ctx := <-started
		m.openEvent(2)
		select {
		case got := <-done:
			if got != nil {
				t.Fatalf("got %T, want no cancelled expansion reply", got)
			}
		case <-time.After(time.Second):
			t.Fatal("superseded expansion did not stop")
		}
		if ctx.Err() != context.Canceled {
			t.Fatalf("got %v, want cancelled expansion", ctx.Err())
		}
	})
	t.Run("closing form reads does not cancel a submitted event write", func(t *testing.T) {
		m, s, _ := fixture(t)
		m.startForm(false)
		cmd := m.updateForm(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
		if cmd == nil {
			t.Fatal("save did not produce a command")
		}
		m.Close()
		got, ok := cmd().(savedMsg)
		if !ok || got.err != nil || s.creates != 1 {
			t.Fatalf("got %#v, creates %d, want successful session-owned save", got, s.creates)
		}
	})
}
