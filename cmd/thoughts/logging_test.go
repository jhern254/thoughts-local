package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
)

func TestCLI_Logging(t *testing.T) {
	t.Run("keeps console logs separate from command output", func(t *testing.T) {
		var logs, output bytes.Buffer
		logger, err := logging.NewConsole(&logs, "thoughts-cli", "")
		if err != nil {
			t.Fatal(err)
		}
		app := newApplication(&output, io.Discard, logger)
		app.openRuntime = func(context.Context, string) (cliRuntime, error) {
			return &cliRuntimeStub{localUser: &data.User{UserID: "local-user"}}, nil
		}

		err = newCLI(app).Run(context.Background(), []string{"thoughts", "subjects", "create"})

		if err == nil {
			t.Fatal("expected argument error")
		}
		for _, want := range []string{"starting application", "stopped application"} {
			if !strings.Contains(logs.String(), want) {
				t.Fatalf("logs %q do not contain %q", logs.String(), want)
			}
		}
		if output.Len() != 0 {
			t.Fatalf("got command output %q", output.String())
		}
	})

	for _, operation := range []string{"application_start", "application_close"} {
		t.Run(operation+" logs safe failure and returns original error", func(t *testing.T) {
			failure := errors.New("PRIVATE-LIFECYCLE-MARKER")
			var logs bytes.Buffer
			logger, err := logging.New(&logs, "test", "error")
			if err != nil {
				t.Fatal(err)
			}
			app := newApplication(io.Discard, io.Discard, logger)
			app.openRuntime = func(context.Context, string) (cliRuntime, error) {
				if operation == "application_start" {
					return nil, failure
				}
				return &cliRuntimeStub{localUser: &data.User{UserID: "local"}, close: func() error { return failure }}, nil
			}
			// Running the root command exercises lifecycle without a service call.
			if err := newCLI(app).Run(context.Background(), []string{"thoughts"}); !errors.Is(err, failure) {
				t.Fatalf("got %v, want original error", err)
			}
			var event map[string]any
			if err := json.Unmarshal(logs.Bytes(), &event); err != nil {
				t.Fatal(err)
			}
			want := map[string]any{"level": "error", "application": "test", "message": "operation failed", "operation": operation, "category": "unexpected_failure"}
			if len(event) != len(want)+2 || event["caller"] == nil || event["time"] == nil {
				t.Fatalf("unexpected fields: %v", event)
			}
			for key, value := range want {
				if event[key] != value {
					t.Fatalf("unexpected event: %v", event)
				}
			}
			if strings.Contains(logs.String(), "PRIVATE-LIFECYCLE-MARKER") {
				t.Fatal("private error entered logs")
			}
		})
	}
}

func newTestLogger(t *testing.T, output io.Writer) logging.Logger {
	t.Helper()
	logger, err := logging.New(output, "test", "info")
	if err != nil {
		t.Fatal(err)
	}
	return logger
}
