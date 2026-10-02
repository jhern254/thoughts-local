package thoughts

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
)

type cancellationService struct {
	Service
	list func(context.Context) ([]data.Thought, error)
}

func (s cancellationService) ListUnassigned(ctx context.Context, _ string) ([]data.Thought, error) {
	return s.list(ctx)
}

type cancellationMetrics struct {
	count func(context.Context) (int64, error)
}

func (s cancellationMetrics) CountThoughts(ctx context.Context, _ string) (int64, error) {
	return s.count(ctx)
}

func TestModel_ReadCancellation(t *testing.T) {
	t.Run("reset prevents a queued read from calling the service", func(t *testing.T) {
		service := cancellationService{list: func(context.Context) ([]data.Thought, error) {
			t.Error("obsolete read called service")
			return nil, nil
		}}
		m := New(t.Context(), "u", service, logging.Nop())
		cmd := m.OpenUnassigned()
		m.Reset()
		if got := cmd(); got != nil {
			t.Fatalf("got %T, want no obsolete reply", got)
		}
	})
	t.Run("replacing a running read cancels it and produces no failure reply", func(t *testing.T) {
		started := make(chan context.Context, 1)
		service := cancellationService{list: func(ctx context.Context) ([]data.Thought, error) { started <- ctx; <-ctx.Done(); return nil, ctx.Err() }}
		m := New(t.Context(), "u", service, logging.Nop())
		cmd := m.OpenUnassigned()
		done := make(chan tea.Msg, 1)
		go func() { done <- cmd() }()
		ctx := <-started
		m.Open(7)
		select {
		case got := <-done:
			if got != nil {
				t.Fatalf("got %T, want no cancellation reply", got)
			}
		case <-time.After(time.Second):
			t.Fatal("superseded read did not stop")
		}
		if ctx.Err() != context.Canceled {
			t.Fatalf("got %v, want cancelled read context", ctx.Err())
		}
	})
	t.Run("pagination does not cancel the independent count but reset does", func(t *testing.T) {
		store := testutils.NewFakeThoughtStore()
		started := make(chan context.Context, 1)
		metrics := cancellationMetrics{count: func(ctx context.Context) (int64, error) { started <- ctx; <-ctx.Done(); return 0, ctx.Err() }}
		m := New(t.Context(), "u", thought.NewService(store), logging.Nop())
		batch := m.OpenBrowseThoughtsView(metrics)().(tea.BatchMsg)
		done := make(chan tea.Msg, 1)
		go func() { done <- batch[1]() }()
		ctx := <-started
		m.loadThoughtsView(data.ThoughtSummaryViewRequest{}, 0)
		if ctx.Err() != nil {
			t.Fatal("page request cancelled independent count")
		}
		m.Reset()
		select {
		case got := <-done:
			if got != nil {
				t.Fatalf("got %T, want no cancelled count reply", got)
			}
		case <-time.After(time.Second):
			t.Fatal("reset did not cancel count")
		}
	})
}
