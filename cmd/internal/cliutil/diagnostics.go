// Package cliutil keeps urfave diagnostics separate from application output.
package cliutil

import (
	"context"
	"io"

	cli "github.com/urfave/cli/v3"
)

// ExitCode mirrors urfave's exit-code selection without raw diagnostic printing.
func ExitCode(err error) int {
	if e, ok := err.(cli.ExitCoder); ok {
		return e.ExitCode()
	}
	code := 1
	if multi, ok := err.(cli.MultiError); ok {
		for _, child := range multi.Errors() {
			switch e := child.(type) {
			case cli.MultiError:
				code = ExitCode(e)
			case cli.ExitCoder:
				code = e.ExitCode()
			}
		}
	}
	return code
}

// ConfigureDiagnostics leaves error reporting to the executable after Run returns.
// Discarding framework error output also covers its generated help command.
func ConfigureDiagnostics(cmd *cli.Command, failureMessage *string) {
	const usageMessage = "Invalid command arguments. Use --help for usage."
	*failureMessage = usageMessage
	cmd.ErrWriter = io.Discard
	cmd.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	cmd.OnUsageError = func(_ context.Context, _ *cli.Command, err error, _ bool) error {
		*failureMessage = usageMessage
		return err
	}
	for _, child := range cmd.Commands {
		ConfigureDiagnostics(child, failureMessage)
	}
}
