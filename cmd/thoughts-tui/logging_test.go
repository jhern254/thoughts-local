package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
)

func TestTUI_Logging(t *testing.T) {
	for _, tc := range []struct {
		operation string
		cause     error
		category  string
	}{
		{"application_start", errors.New("PRIVATE-LIFECYCLE-MARKER"), "unexpected_failure"},
		{"application_close", errors.New("PRIVATE-LIFECYCLE-MARKER"), "unexpected_failure"},
		{"tui_run", errors.New("PRIVATE-LIFECYCLE-MARKER"), "unexpected_failure"},
		{"application_start", data.ErrDatabaseReadOnly, "database_read_only"},
	} {
		operation := tc.operation
		t.Run(operation+" "+tc.category+" logs safe failure and returns original error", func(t *testing.T) {
			failure := fmt.Errorf("PRIVATE-LIFECYCLE-MARKER: %w", tc.cause)
			var logs bytes.Buffer
			logger, err := logging.New(&logs, "test", "error")
			if err != nil {
				t.Fatal(err)
			}
			app := newApplication(strings.NewReader(""), io.Discard, io.Discard, logger)
			app.openRuntime = func(context.Context, string) (runtime, error) {
				if operation == "application_start" {
					return nil, failure
				}
				return &runtimeStub{localUser: &data.User{UserID: "local"}, close: func() error {
					if operation == "application_close" {
						return failure
					}
					return nil
				}}, nil
			}
			app.runProgram = func(context.Context, tea.Model, io.Reader, io.Writer) error {
				if operation == "tui_run" {
					return failure
				}
				return nil
			}
			if err := newTUI(app).Run(context.Background(), []string{"thoughts-tui"}); !errors.Is(err, failure) {
				t.Fatalf("got %v, want original error", err)
			}
			var event map[string]any
			if err := json.Unmarshal(logs.Bytes(), &event); err != nil {
				t.Fatal(err)
			}
			want := map[string]any{"level": "error", "application": "test", "message": "operation failed", "operation": operation, "category": tc.category}
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

	t.Run("keeps lifecycle logs in file and screen output in terminal", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "thoughts.log")
		logger, file, err := logging.NewFile(path, "thoughts-tui", "")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := file.Close(); err != nil {
				t.Error(err)
			}
		})
		var output, errOutput bytes.Buffer
		app := newApplication(strings.NewReader(""), &output, &errOutput, logger)
		app.openRuntime = func(context.Context, string) (runtime, error) {
			return &runtimeStub{localUser: &data.User{UserID: "local-user"}}, nil
		}
		app.runProgram = func(_ context.Context, _ tea.Model, _ io.Reader, out io.Writer) error {
			_, err := io.WriteString(out, "screen")
			return err
		}
		if err := newTUI(app).Run(context.Background(), []string{"thoughts-tui"}); err != nil {
			t.Fatal(err)
		}
		if output.String() != "screen" || errOutput.Len() != 0 {
			t.Fatalf("unexpected terminal output: %q, %q", &output, &errOutput)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		decoder := json.NewDecoder(bytes.NewReader(contents))
		for _, want := range []string{"starting application", "stopped application"} {
			var event map[string]any
			if err := decoder.Decode(&event); err != nil {
				t.Fatal(err)
			}
			if event["message"] != want || event["application"] != "thoughts-tui" {
				t.Fatalf("unexpected event: %v", event)
			}
		}
	})
}
