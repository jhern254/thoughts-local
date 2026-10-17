package main

import (
	"errors"
	"fmt"
	"net"
	"syscall"

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

func browserStartupMessage(err error, port int) string {
	cause := diagnostics.SingleError(err)
	var listenErr *net.OpError
	if errors.As(cause, &listenErr) && listenErr.Op == "listen" {
		// Windows sockets return WSAEADDRINUSE, not syscall's synthetic EADDRINUSE.
		const windowsAddressAlreadyInUse = syscall.Errno(10048)
		if errors.Is(cause, syscall.EADDRINUSE) || errors.Is(cause, windowsAddressAlreadyInUse) {
			return fmt.Sprintf("Browser port %d is already in use. Stop the other server or choose another port with --browser-port or THOUGHTS_BROWSER_PORT.", port)
		}
		return fmt.Sprintf("Could not listen on browser port %d. Check local network permissions or choose another port with --browser-port or THOUGHTS_BROWSER_PORT.", port)
	}
	if errors.Is(err, errBrowserSpeechSetup) {
		return errBrowserSpeechSetup.Error()
	}
	return "Could not run the browser interface."
}
