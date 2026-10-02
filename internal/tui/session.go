package tui

import (
	"context"
	"sync"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
)

// Session guards one native or browser model. Bubble Tea does not join Cmd
// goroutines when Run returns. It joins started commands, prevents queued commands starting after exit,
// and contains authored panic values before Bubble Tea can print them. Thoughts
// and its pinned widgets use ordinary commands and public tea.BatchMsg batches.
// Keep that contract when adding new command scheduling primitives.
type Session struct {
	model    tea.Model
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	closed   bool
	commands sync.WaitGroup
	failed   atomic.Bool
}

// NewSession constructs a model with the context owned by its caller.
func NewSession(ctx context.Context, cancel context.CancelFunc, factory func(context.Context) tea.Model) (m *Session) {
	m = &Session{ctx: ctx, cancel: cancel}
	defer m.recover()
	m.model = factory(ctx)
	return m
}

func (m *Session) recover() {
	if recover() != nil {
		m.failed.Store(true)
		m.cancel()
	}
}

func (m *Session) Init() (cmd tea.Cmd) { defer m.recover(); return m.wrap(m.model.Init()) }
func (m *Session) Update(msg tea.Msg) (model tea.Model, cmd tea.Cmd) {
	model = m
	defer m.recover()
	m.model, cmd = m.model.Update(msg)
	return m, m.wrap(cmd)
}
func (m *Session) View() (view tea.View) { defer m.recover(); return m.model.View() }

func (m *Session) wrap(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() (msg tea.Msg) {
		m.mu.Lock()
		if m.closed || m.ctx.Err() != nil {
			m.mu.Unlock()
			return nil
		}
		m.commands.Add(1)
		m.mu.Unlock()
		defer m.commands.Done()
		defer m.recover()
		msg = cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			wrapped := make(tea.BatchMsg, len(batch))
			for i, cmd := range batch {
				wrapped[i] = m.wrap(cmd)
			}
			return wrapped
		}
		return msg
	}
}

// Stop prevents new commands, cancels the session, and joins started work.
func (m *Session) Stop() {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	m.cancel()
	m.commands.Wait()
}

// Failed reports a contained model or command panic.
func (m *Session) Failed() bool { return m.failed.Load() }
