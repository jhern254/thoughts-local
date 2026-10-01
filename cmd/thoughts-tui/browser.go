package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/browserterm"
	"github.com/jhern254/go-thoughts/internal/logging"
)

func runBrowser(ctx context.Context, port int, newModel func(context.Context) tea.Model, out io.Writer, logger logging.Logger) error {
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
	return browserterm.Serve(ctx, listener, newModel, logger)
}
