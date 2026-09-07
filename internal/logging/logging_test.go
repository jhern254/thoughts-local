package logging_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jhern254/go-thoughts/internal/logging"
)

func TestLogger(t *testing.T) {
	t.Run("filters events by configured level", func(t *testing.T) {
		for _, level := range []string{"info", "error", "disabled"} {
			t.Run(level, func(t *testing.T) {
				var output bytes.Buffer
				logger, err := logging.New(&output, "test", level)
				if err != nil {
					t.Fatal(err)
				}
				logger.Info("information", nil)
				logger.Error(errors.New("failure"), "problem", nil)
				if got := strings.Contains(output.String(), `"level":"info"`); got != (level == "info") {
					t.Fatalf("unexpected info output: %s", &output)
				}
				if got := strings.Contains(output.String(), `"level":"error"`); got != (level != "disabled") {
					t.Fatalf("unexpected error output: %s", &output)
				}
				if level == "disabled" && output.Len() != 0 {
					t.Fatalf("disabled logger wrote %q", &output)
				}
			})
		}
	})

	t.Run("zero value and Nop accept events", func(t *testing.T) {
		for _, logger := range []logging.Logger{{}, logging.Nop()} {
			logger.Info("ignored", nil)
			logger.Error(errors.New("ignored"), "ignored", nil)
		}
	})

	t.Run("writes configured event from application caller", func(t *testing.T) {
		var output bytes.Buffer
		logger, err := logging.New(&output, "test-app", "")
		if err != nil {
			t.Fatal(err)
		}

		wantCaller := logFromApplication(logger)

		var event map[string]any
		if err := json.Unmarshal(output.Bytes(), &event); err != nil {
			t.Fatal(err)
		}
		for key, want := range map[string]any{
			"level": "info", "application": "test-app", "message": "created", "subject_id": float64(7),
		} {
			if got := event[key]; got != want {
				t.Fatalf("got %s %v, want %v", key, got, want)
			}
		}
		caller, ok := event["caller"].(string)
		if !ok || filepath.Base(caller) != wantCaller {
			t.Fatalf("got caller %v, want %q", event["caller"], wantCaller)
		}
		if event["time"] == nil {
			t.Fatal("log has no timestamp")
		}
	})

	t.Run("rejects invalid level", func(t *testing.T) {
		if _, err := logging.New(&bytes.Buffer{}, "test-app", "verbose"); err == nil || !strings.Contains(err.Error(), "THOUGHTS_LOG_LEVEL") {
			t.Fatalf("got error %v, want invalid THOUGHTS_LOG_LEVEL error", err)
		}
	})
}

func TestLogger_NewFile(t *testing.T) {
	t.Run("creates private file and preserves events when reopened", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "logs", "thoughts.log")
		for _, message := range []string{"first", "second"} {
			logger, file, err := logging.NewFile(path, "test", "info")
			if err != nil {
				t.Fatal(err)
			}
			logger.Info(message, nil)
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("log readable by other users: %o", info.Mode().Perm())
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		decoder := json.NewDecoder(bytes.NewReader(contents))
		for _, want := range []string{"first", "second"} {
			var event map[string]any
			if err := decoder.Decode(&event); err != nil {
				t.Fatal(err)
			}
			if event["message"] != want {
				t.Fatalf("got event %v, want %s", event, want)
			}
		}
	})

	for _, level := range []string{"disabled", "invalid"} {
		t.Run(level+" creates no directory or file", func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "logs")
			_, file, err := logging.NewFile(filepath.Join(directory, "thoughts.log"), "test", level)
			if file != nil {
				file.Close()
				t.Fatal("unexpected log file")
			}
			if (err != nil) != (level == "invalid") {
				t.Fatalf("unexpected error: %v", err)
			}
			if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("log directory exists or stat failed: %v", err)
			}
		})
	}

	t.Run("returns file open failure", func(t *testing.T) {
		_, file, err := logging.NewFile(t.TempDir(), "test", "info")
		if file != nil || err == nil {
			t.Fatalf("got file %v, error %v", file, err)
		}
	})
}

func logFromApplication(logger logging.Logger) string {
	_, file, line, _ := runtime.Caller(0)
	logger.Info("created", logging.Fields{"subject_id": int64(7)})
	return fmt.Sprintf("%s:%d", filepath.Base(file), line+1)
}
