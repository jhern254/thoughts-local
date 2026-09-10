package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
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
