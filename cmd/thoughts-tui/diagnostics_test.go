package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"syscall"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/editorprefs"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/tui"
	"github.com/jhern254/go-thoughts/internal/visual"
)

func TestTUI_DiagnosticOutput(t *testing.T) {
	const private = "PRIVATE-DIAGNOSTIC-MARKER"
	for _, stage := range []string{"open", "run", "close"} {
		t.Run(stage+" reports a safe error after program returns", func(t *testing.T) {
			failure := errors.New(private)
			var diagnostics, output, logs bytes.Buffer
			logger, err := logging.New(&logs, "test", "error")
			if err != nil {
				t.Fatal(err)
			}
			app := newApplication(strings.NewReader(""), &output, &diagnostics, logger)
			app.openRuntime = func(context.Context, string) (runtime, error) {
				if stage == "open" {
					return nil, failure
				}
				return &runtimeStub{localUser: &data.User{UserID: "local"}, close: func() error {
					if stage == "close" {
						return failure
					}
					return nil
				}}, nil
			}
			app.runProgram = func(context.Context, tea.Model, io.Reader, io.Writer) error {
				if diagnostics.Len() != 0 {
					t.Fatalf("got diagnostics %q while program owns terminal, want empty", diagnostics.String())
				}
				output.WriteString(private)
				if stage == "run" {
					return failure
				}
				return nil
			}
			err = newTUI(app).Run(context.Background(), []string{"thoughts-tui"})
			if !errors.Is(err, failure) {
				t.Fatalf("got error %v, want original error", err)
			}
			if got, want := app.reportError(err), 1; got != want {
				t.Fatalf("got exit code %d, want %d", got, want)
			}
			want := map[string]string{"open": "Could not start the application.", "run": "Could not run the terminal interface.", "close": "Could not close the application database."}[stage] + "\n"
			if got := diagnostics.String(); got != want {
				t.Fatalf("got diagnostic %q, want %q", got, want)
			}
			if strings.Contains(logs.String()+diagnostics.String(), private) {
				t.Fatal("got private marker in diagnostics, want metadata and safe messages")
			}
			if got, want := strings.Count(logs.String(), "operation failed"), 1; got != want {
				t.Fatalf("got failure events %d, want %d", got, want)
			}
			if stage != "open" && output.String() != private {
				t.Fatalf("got screen output %q, want intentional content %q", output.String(), private)
			}
		})
	}
}

func TestTUI_BrowserStartupDiagnostics(t *testing.T) {
	t.Run("reports an occupied port and remedy through the real browser startup", func(t *testing.T) {
		t.Setenv("THOUGHTS_MODELS_DIR", "")
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		port := listener.Addr().(*net.TCPAddr).Port
		var output, feedback, logs bytes.Buffer
		logger, err := logging.New(&logs, "test", "error")
		if err != nil {
			t.Fatal(err)
		}
		app := newApplication(strings.NewReader(""), &output, &feedback, logger)
		closed := false
		app.openRuntime = func(context.Context, string) (runtime, error) {
			return &runtimeStub{close: func() error { closed = true; return nil }}, nil
		}
		err = newTUI(app).Run(t.Context(), []string{"thoughts-tui", "--browser", "--browser-open=false", "--browser-port", fmt.Sprint(port)})
		if err == nil {
			t.Fatal("occupied port started successfully")
		}
		var listenErr *net.OpError
		if !errors.As(err, &listenErr) {
			t.Fatalf("got error %T, want original listen failure", err)
		}
		if got := app.reportError(err); got != 1 {
			t.Fatalf("got exit code %d, want 1", got)
		}
		want := fmt.Sprintf("Browser port %d is already in use. Stop the other server or choose another port with --browser-port or THOUGHTS_BROWSER_PORT.\n", port)
		if got := feedback.String(); got != want {
			t.Fatalf("got diagnostic %q, want %q", got, want)
		}
		if !closed || output.Len() != 0 {
			t.Fatalf("got closed=%v, output=%q, want closed runtime and no server address", closed, output.String())
		}
		if got := strings.Count(logs.String(), "operation failed"); got != 1 {
			t.Fatalf("got failure logs %d, want 1", got)
		}
		if strings.Contains(logs.String(), listener.Addr().String()) {
			t.Fatal("operational logs contain raw listener address")
		}
		dialer := net.Dialer{}
		request, err := dialer.DialContext(t.Context(), "tcp4", listener.Addr().String())
		if err != nil {
			t.Fatalf("original listener stopped after second startup: %v", err)
		}
		request.Close()
	})
	const private = "PRIVATE-BROWSER-STARTUP-MARKER"
	occupied := "Browser port 7878 is already in use. Stop the other server or choose another port with --browser-port or THOUGHTS_BROWSER_PORT.\n"
	unavailable := "Could not listen on browser port 7878. Check local network permissions or choose another port with --browser-port or THOUGHTS_BROWSER_PORT.\n"
	for _, scenario := range []struct {
		name string
		err  error
		want string
	}{
		{"wrapped occupied port", &net.OpError{Op: "listen", Err: fmt.Errorf(private+": %w", syscall.EADDRINUSE)}, occupied},
		{"Windows occupied port", &net.OpError{Op: "listen", Err: fmt.Errorf(private+": %w", syscall.Errno(10048))}, occupied},
		{"listen permission denied", &net.OpError{Op: "listen", Err: fmt.Errorf(private+": %w", syscall.EACCES)}, unavailable},
		{"unknown listen failure", &net.OpError{Op: "listen", Err: errors.New(private)}, unavailable},
		{"unrelated network operation", &net.OpError{Op: "dial", Err: syscall.EADDRINUSE}, "Could not run the browser interface.\n"},
		{"startup and cleanup failures", errors.Join(&net.OpError{Op: "listen", Err: syscall.EADDRINUSE}, errors.New(private)), "Could not complete the application operation.\n"},
	} {
		t.Run(scenario.name+" keeps raw errors private", func(t *testing.T) {
			var feedback, logs bytes.Buffer
			logger, err := logging.New(&logs, "test", "error")
			if err != nil {
				t.Fatal(err)
			}
			app := newApplication(strings.NewReader(""), io.Discard, &feedback, logger)
			app.openRuntime = func(context.Context, string) (runtime, error) { return &runtimeStub{}, nil }
			app.runBrowser = func(context.Context, int, bool, string, func(context.Context, tui.SpeechModelSetup) tea.Model, io.Writer, logging.Logger, *visual.Service, *editorprefs.Service) error {
				return scenario.err
			}
			err = newTUI(app).Run(t.Context(), []string{"thoughts-tui", "--browser", "--browser-port", "7878"})
			if err != scenario.err {
				t.Fatalf("got error %v, want original startup failure %v", err, scenario.err)
			}
			app.reportError(err)
			if got := feedback.String(); got != scenario.want {
				t.Fatalf("got diagnostic %q, want %q", got, scenario.want)
			}
			if strings.Contains(feedback.String()+logs.String(), private) {
				t.Fatal("diagnostics or operational logs exposed raw error text")
			}
		})
	}
}
