package browserterm

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
)

var errProgram = errors.New("browser terminal program failed")

// Program options are adapted from sip's MakeOptions (see UPSTREAM.md). A
// browser supplies explicit input messages and dimensions, so no PTY, inherited
// environment, terminal reader, or process signal handler is needed.
func newProgram(ctx context.Context, model tea.Model, out io.Writer, size tea.WindowSizeMsg) *tea.Program {
	return tea.NewProgram(model,
		tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(out),
		tea.WithColorProfile(colorprofile.TrueColor), tea.WithWindowSize(size.Width, size.Height),
		tea.WithEnvironment([]string{"TERM=xterm-256color", "COLORTERM=truecolor"}),
		tea.WithoutSignalHandler(), tea.WithoutCatchPanics(),
		tea.WithFilter(func(_ tea.Model, msg tea.Msg) tea.Msg {
			if _, ok := msg.(tea.SuspendMsg); ok {
				return tea.ResumeMsg{}
			}
			return msg
		}),
	)
}

func runProgram(program *tea.Program) (err error) {
	defer func() {
		if recover() != nil {
			program.Kill()
			err = errProgram
		}
	}()
	_, err = program.Run()
	return err
}

// Bubble Tea does not join Cmd goroutines when Run returns. This session-local
// wrapper joins started commands, prevents queued commands starting after exit,
// and contains authored panic values before Bubble Tea can print them. Thoughts
// and its pinned widgets use ordinary commands and public tea.BatchMsg batches.
// Keep that contract when adding new command scheduling primitives.
type sessionModel struct {
	model    tea.Model
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	closed   bool
	commands sync.WaitGroup
	failed   atomic.Bool
}

func newSessionModel(ctx context.Context, cancel context.CancelFunc, factory func(context.Context) tea.Model) (m *sessionModel) {
	m = &sessionModel{ctx: ctx, cancel: cancel}
	defer m.recover()
	m.model = factory(ctx)
	return m
}

func (m *sessionModel) recover() {
	if recover() != nil {
		m.failed.Store(true)
		m.cancel()
	}
}

func (m *sessionModel) Init() (cmd tea.Cmd) { defer m.recover(); return m.wrap(m.model.Init()) }
func (m *sessionModel) Update(msg tea.Msg) (model tea.Model, cmd tea.Cmd) {
	model = m
	defer m.recover()
	m.model, cmd = m.model.Update(msg)
	return m, m.wrap(cmd)
}
func (m *sessionModel) View() (view tea.View) { defer m.recover(); return m.model.View() }

func (m *sessionModel) wrap(cmd tea.Cmd) tea.Cmd {
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

func (m *sessionModel) stop() {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	m.commands.Wait()
}
