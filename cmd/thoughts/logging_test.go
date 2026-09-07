package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
)

func TestNewLogger(t *testing.T) {
	t.Run("uses default metadata and info level", func(t *testing.T) {
		var output bytes.Buffer
		logger, err := newLogger(&output, "")
		if err != nil {
			t.Fatal(err)
		}

		logger.Debug("hidden")
		logger.Info("started")

		got := output.String()
		for _, want := range []string{"started", cliApplicationName, "logging_test.go"} {
			if !strings.Contains(got, want) {
				t.Fatalf("log %q does not contain %q", got, want)
			}
		}
		if strings.Contains(got, "hidden") {
			t.Fatalf("log %q contains debug event at default level", got)
		}
	})

	t.Run("uses configured level", func(t *testing.T) {
		var output bytes.Buffer
		logger, err := newLogger(&output, "debug")
		if err != nil {
			t.Fatal(err)
		}
		logger.Debug("visible")
		if !strings.Contains(output.String(), "visible") {
			t.Fatalf("log %q does not contain debug event", output.String())
		}
	})

	t.Run("disables logging", func(t *testing.T) {
		var output bytes.Buffer
		logger, err := newLogger(&output, "disabled")
		if err != nil {
			t.Fatal(err)
		}
		logger.Error(errors.New("hidden"), "hidden")
		if output.Len() != 0 {
			t.Fatalf("got disabled log output %q", output.String())
		}
	})

	t.Run("rejects invalid level", func(t *testing.T) {
		if _, err := newLogger(io.Discard, "verbose"); err == nil || !strings.Contains(err.Error(), "THOUGHTS_LOG_LEVEL") {
			t.Fatalf("got error %v, want invalid THOUGHTS_LOG_LEVEL error", err)
		}
	})
}

func TestCLI_Logging(t *testing.T) {
	t.Run("logs lifecycle without writing to command output", func(t *testing.T) {
		var logs, out bytes.Buffer
		app := newApplication(&out, io.Discard, newTestLogger(t, &logs))
		app.openRuntime = func(context.Context, string) (cliRuntime, error) {
			return &cliRuntimeStub{localUser: &data.User{UserID: "local-user"}}, nil
		}

		err := newCLI(app).Run(context.Background(), []string{"thoughts", "subjects", "create"})

		if err == nil {
			t.Fatal("expected argument error")
		}
		for _, want := range []string{"starting application", "stopped application"} {
			if !strings.Contains(logs.String(), want) {
				t.Fatalf("logs %q do not contain %q", logs.String(), want)
			}
		}
		if out.Len() != 0 {
			t.Fatalf("got command output %q", out.String())
		}
	})

	t.Run("logs runtime open failure", func(t *testing.T) {
		want := errors.New("open failed")
		var logs bytes.Buffer
		app := newApplication(io.Discard, io.Discard, newTestLogger(t, &logs))
		app.openRuntime = func(context.Context, string) (cliRuntime, error) { return nil, want }

		err := newCLI(app).Run(context.Background(), []string{"thoughts", "subjects", "list"})

		if err != want {
			t.Fatalf("got error %v, want %v", err, want)
		}
		if got := logs.String(); !strings.Contains(got, "failed to start application") || !strings.Contains(got, want.Error()) {
			t.Fatalf("logs %q do not contain startup failure", got)
		}
	})

	t.Run("logs runtime close failure", func(t *testing.T) {
		want := errors.New("close failed")
		var logs bytes.Buffer
		app := newApplication(io.Discard, io.Discard, newTestLogger(t, &logs))
		app.openRuntime = func(context.Context, string) (cliRuntime, error) {
			return &cliRuntimeStub{
				localUser: &data.User{UserID: "local-user"},
				close:     func() error { return want },
			}, nil
		}

		newCLI(app).Run(context.Background(), []string{"thoughts", "subjects", "create"})

		if got := logs.String(); !strings.Contains(got, "failed to close application") || !strings.Contains(got, want.Error()) {
			t.Fatalf("logs %q do not contain close failure", got)
		}
	})
}

func newTestLogger(t *testing.T, output io.Writer) logging.Logger {
	t.Helper()
	level, err := logging.ParseLevel("info")
	if err != nil {
		t.Fatal(err)
	}
	return logging.New(output, "test", level)
}
