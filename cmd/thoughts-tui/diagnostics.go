package main

import (
	"fmt"

	"github.com/jhern254/go-thoughts/cmd/internal/cliutil"
	"github.com/jhern254/go-thoughts/internal/diagnostics"
)

func (app *application) reportError(err error) int {
	message := app.failureMessage
	if message == "" || diagnostics.SingleError(err) == nil {
		message = "Could not complete the application operation."
	}
	fmt.Fprintln(app.errOut, message)
	return cliutil.ExitCode(err)
}
