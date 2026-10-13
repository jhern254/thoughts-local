package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	goruntime "runtime"
	"strconv"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/browserterm"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/visual"
)

func runBrowser(ctx context.Context, port int, autoOpen bool, newModel func(context.Context) tea.Model, out io.Writer, logger logging.Logger, visual *visual.Service) error {
	var opener func(context.Context, string) error
	if autoOpen {
		opener = openDefaultBrowser
	}
	return serveBrowser(ctx, port, newModel, out, logger, visual, opener)
}

func serveBrowser(ctx context.Context, port int, newModel func(context.Context) tea.Model, out io.Writer, logger logging.Logger, visual *visual.Service, opener func(context.Context, string) error) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return err
	}
	defer listener.Close()
	if _, err = fmt.Fprintf(out, "Thoughts: http://%s\nPress Ctrl+C here to stop the browser server.\n", listener.Addr().String()); err != nil {
		return err
	}
	// Serve rejects terminal tracing before startup. Let that validation finish
	// without opening a tab for a server that cannot run.
	if opener == nil || os.Getenv("TEA_TRACE") != "" {
		return browserterm.Serve(ctx, listener, newModel, logger, visual)
	}
	openingCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	opened := make(chan struct{})
	go func() {
		defer close(opened)
		launchCtx, stop := context.WithTimeout(openingCtx, 5*time.Second)
		defer stop()
		if err := opener(launchCtx, "http://"+listener.Addr().String()); err != nil && openingCtx.Err() == nil {
			fmt.Fprintln(out, "Could not open the default browser. Open the printed address manually.")
		}
	}()
	err = browserterm.Serve(ctx, listener, newModel, logger, visual)
	cancel()
	<-opened
	return err
}

// These desktop launchers use the user's default browser. The URL is generated
// from our bound listener; no shell or user-supplied command is involved.
func openDefaultBrowser(ctx context.Context, address string) error {
	var command *exec.Cmd
	switch goruntime.GOOS {
	case "darwin":
		command = exec.CommandContext(ctx, "open", address)
	case "windows":
		command = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", address)
	default:
		command = exec.CommandContext(ctx, "xdg-open", address)
	}
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	command.WaitDelay = time.Second
	return command.Run()
}
