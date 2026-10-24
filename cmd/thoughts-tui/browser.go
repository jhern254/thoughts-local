package main

import (
	"context"
	"errors"
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
	"github.com/jhern254/go-thoughts/internal/editorprefs"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/tui"
	"github.com/jhern254/go-thoughts/internal/visual"
)

type browserConfig struct {
	NewModel          func(context.Context, tui.SpeechModelSetup) tea.Model
	Output            io.Writer
	Logger            logging.Logger
	Visual            *visual.Service
	EditorPreferences *editorprefs.Service

	Port           int
	ModelDirectory string
	AutoOpen       bool
}

func runBrowser(ctx context.Context, config browserConfig) error {
	var opener func(context.Context, string) error
	if config.AutoOpen {
		opener = openDefaultBrowser
	}
	return serveBrowser(ctx, config, opener)
}

func serveBrowser(ctx context.Context, config browserConfig, opener func(context.Context, string) error) (serveErr error) {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	speechSetup, err := newBrowserSpeechSetup(config.ModelDirectory)
	if err != nil {
		return err
	}
	defer func() {
		closeErr := speechSetup.Close()
		if closeErr == nil {
			return
		}
		if serveErr == nil {
			serveErr = closeErr
			return
		}
		serveErr = errors.Join(serveErr, closeErr)
	}()
	consumerFactory := speechSetup.RecordingConsumerFactory(config.EditorPreferences)
	browserModel := func(ctx context.Context) tea.Model { return config.NewModel(ctx, speechSetup) }

	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(config.Port)))
	if err != nil {
		return err
	}
	defer listener.Close()
	if _, err = fmt.Fprintf(config.Output, "Thoughts: http://%s\nPress Ctrl+C here to stop the browser server.\n", listener.Addr().String()); err != nil {
		return err
	}
	// Serve rejects terminal tracing before startup. Let that validation finish
	// without opening a tab for a server that cannot run.
	if opener == nil || os.Getenv("TEA_TRACE") != "" {
		return browserterm.ServeWithAudioConsumerFactory(ctx, listener, browserModel, config.Logger, consumerFactory, config.Visual)
	}
	openingCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	opened := make(chan struct{})
	go func() {
		defer close(opened)
		launchCtx, stop := context.WithTimeout(openingCtx, 5*time.Second)
		defer stop()
		if err := opener(launchCtx, "http://"+listener.Addr().String()); err != nil && openingCtx.Err() == nil {
			fmt.Fprintln(config.Output, "Could not open the default browser. Open the printed address manually.")
		}
	}()
	err = browserterm.ServeWithAudioConsumerFactory(ctx, listener, browserModel, config.Logger, consumerFactory, config.Visual)
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
