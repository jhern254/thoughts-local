package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
)

func TestNewLogger(t *testing.T) {
	t.Run("writes JSON with default metadata and info level", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "logs", "thoughts.log")
		logger, file, err := newLogger(path, "")
		if err != nil {
			t.Fatal(err)
		}
		logger.Debug("hidden")
		logger.Info("started")
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}

		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		got := string(contents)
		for _, want := range []string{`"message":"started"`, `"application":"thoughts-tui"`, `"time":`, "logging_test.go"} {
			if !strings.Contains(got, want) {
				t.Fatalf("log %q does not contain %q", got, want)
			}
		}
		if strings.Contains(got, "hidden") {
			t.Fatalf("log %q contains debug event at default level", got)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
			t.Fatalf("got log permissions %o, want %o", got, want)
		}
	})

	t.Run("uses configured level", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "thoughts.log")
		logger, file, err := newLogger(path, "debug")
		if err != nil {
			t.Fatal(err)
		}
		logger.Debug("visible")
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(contents), "visible") {
			t.Fatalf("log %q does not contain debug event", contents)
		}
	})

	t.Run("disables logging without creating file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "thoughts.log")
		logger, file, err := newLogger(path, "disabled")
		if err != nil {
			t.Fatal(err)
		}
		if file != nil {
			t.Fatal("got log file when logging is disabled")
		}
		logger.Error(errors.New("hidden"), "hidden")
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("got log file stat error %v, want not exist", err)
		}
	})

	t.Run("rejects invalid level", func(t *testing.T) {
		if _, _, err := newLogger(filepath.Join(t.TempDir(), "thoughts.log"), "verbose"); err == nil || !strings.Contains(err.Error(), "THOUGHTS_LOG_LEVEL") {
			t.Fatalf("got error %v, want invalid THOUGHTS_LOG_LEVEL error", err)
		}
	})
}

func TestTUI_Logging(t *testing.T) {
	t.Run("logs lifecycle separately from Bubble Tea output", func(t *testing.T) {
		var logs, output bytes.Buffer
		app := newApplication(strings.NewReader(""), &output, io.Discard, newTestLogger(t, &logs))
		app.openRuntime = func(context.Context, string) (runtime, error) {
			return &runtimeStub{localUser: &data.User{UserID: "local-user"}}, nil
		}
		app.runProgram = func(_ context.Context, _ tea.Model, _ io.Reader, output io.Writer) error {
			_, err := io.WriteString(output, "screen")
			return err
		}

		if err := newTUI(app).Run(context.Background(), []string{"thoughts-tui"}); err != nil {
			t.Fatal(err)
		}

		for _, want := range []string{"starting application", "stopped application"} {
			if !strings.Contains(logs.String(), want) {
				t.Fatalf("logs %q do not contain %q", logs.String(), want)
			}
		}
		if got, want := output.String(), "screen"; got != want {
			t.Fatalf("got Bubble Tea output %q, want %q", got, want)
		}
	})

	t.Run("logs program failure", func(t *testing.T) {
		want := errors.New("program failed")
		var logs bytes.Buffer
		app := newApplication(strings.NewReader(""), io.Discard, io.Discard, newTestLogger(t, &logs))
		app.openRuntime = func(context.Context, string) (runtime, error) {
			return &runtimeStub{localUser: &data.User{UserID: "local-user"}}, nil
		}
		app.runProgram = func(context.Context, tea.Model, io.Reader, io.Writer) error { return want }

		err := newTUI(app).Run(context.Background(), []string{"thoughts-tui"})

		if err != want {
			t.Fatalf("got error %v, want %v", err, want)
		}
		if got := logs.String(); !strings.Contains(got, "TUI program failed") || !strings.Contains(got, want.Error()) {
			t.Fatalf("logs %q do not contain program failure", got)
		}
	})

	t.Run("logs runtime close failure", func(t *testing.T) {
		want := errors.New("close failed")
		var logs bytes.Buffer
		app := newApplication(strings.NewReader(""), io.Discard, io.Discard, newTestLogger(t, &logs))
		app.openRuntime = func(context.Context, string) (runtime, error) {
			return &runtimeStub{
				localUser: &data.User{UserID: "local-user"},
				close:     func() error { return want },
			}, nil
		}
		app.runProgram = func(context.Context, tea.Model, io.Reader, io.Writer) error { return nil }

		err := newTUI(app).Run(context.Background(), []string{"thoughts-tui"})

		if err != want {
			t.Fatalf("got error %v, want %v", err, want)
		}
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
