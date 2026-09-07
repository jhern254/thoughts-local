package main

import (
	"bytes"
	"context"
	"encoding/json"
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
