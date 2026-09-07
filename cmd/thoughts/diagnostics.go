package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
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

func subjectDiagnostic(err error) string {
	cause := diagnosticCause(err)
	var validation *subject.ValidationError
	switch {
	case errors.Is(cause, data.ErrRecordNotFound):
		return "The requested resource was not found."
	case errors.Is(cause, data.ErrDuplicateRecord):
		return "A subject with that name already exists."
	case errors.As(cause, &validation):
		if len(validation.Fields) == 1 && validation.Fields["subject_name"] == "must be between 1 and 255 characters long" {
			return "Subject name must be between 1 and 255 characters long."
		}
		return "The subject details are invalid."
	default:
		return ""
	}
}

func (app *application) subjectFailure(err error, fallback string) {
	app.failureMessage = subjectDiagnostic(err)
	if app.failureMessage == "" {
		app.failureMessage = fallback
	}
}

func (app *application) reportError(err error) int {
	message := app.failureMessage
	if message == "" || diagnosticCause(err) == nil {
		message = "Could not complete the command."
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
