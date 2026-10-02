package browserterm

import (
	"context"
	"errors"
	"io"

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
