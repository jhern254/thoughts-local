package main

import (
	"context"
	"fmt"

	cli "github.com/urfave/cli/v3"
)

// An aggregate cannot safely establish a single expected outcome.
func diagnosticCause(err error) error {
	for range 32 {
		switch e := err.(type) {
		case interface{ Unwrap() []error }, interface{ Errors() []error }:
			return nil
		case interface{ Unwrap() error }:
			err = e.Unwrap()
		default:
			return err
		}
	}
	return nil
}

func (app *application) reportError(err error) int {
	message := app.failureMessage
	if message == "" || diagnosticCause(err) == nil {
		message = "Could not complete the application operation."
	}
	fmt.Fprintln(app.errOut, message)
	return commandExitCode(err)
}

// Mirror urfave's exit-code selection without its raw diagnostic printing.
func commandExitCode(err error) int {
	if e, ok := err.(cli.ExitCoder); ok {
		return e.ExitCode()
	}
	code := 1
	if multi, ok := err.(cli.MultiError); ok {
		for _, child := range multi.Errors() {
			switch e := child.(type) {
			case cli.MultiError:
				code = commandExitCode(e)
			case cli.ExitCoder:
				code = e.ExitCode()
			}
		}
	}
	return code
}

func (app *application) configureDiagnostics(cmd *cli.Command) {
	cmd.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	cmd.OnUsageError = func(_ context.Context, _ *cli.Command, err error, _ bool) error {
		app.failureMessage = "Invalid command arguments. Use --help for usage."
		return err
	}
	for _, child := range cmd.Commands {
		app.configureDiagnostics(child)
	}
}
