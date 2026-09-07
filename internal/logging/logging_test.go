package logging_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jhern254/go-thoughts/internal/logging"
)

func TestLogger(t *testing.T) {
	t.Run("uses configured level and structured fields", func(t *testing.T) {
		level, err := logging.ParseLevel("debug")
		if err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		logger := logging.New(&output, "test-app", level)

		logger.Debug("visible", logging.String("operation", "create"), logging.Int64("subject_id", 7))

		got := output.String()
		for _, want := range []string{`"message":"visible"`, `"application":"test-app"`, `"operation":"create"`, `"subject_id":7`} {
			if !strings.Contains(got, want) {
				t.Fatalf("log %q does not contain %q", got, want)
			}
		}
	})

	t.Run("reports the application caller", func(t *testing.T) {
		level, err := logging.ParseLevel("")
		if err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		logger := logging.New(&output, "test-app", level)

		wantCaller := logFromApplication(logger)

		var event map[string]any
		if err := json.Unmarshal(output.Bytes(), &event); err != nil {
			t.Fatal(err)
		}
		caller, ok := event["caller"].(string)
		if !ok {
			t.Fatalf("got caller %v, want string", event["caller"])
		}
		if got := filepath.Base(caller); got != wantCaller {
			t.Fatalf("got caller %q, want %q", got, wantCaller)
		}
	})

	t.Run("disables output", func(t *testing.T) {
		level, err := logging.ParseLevel("disabled")
		if err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		logging.New(&output, "test-app", level).Error(fmt.Errorf("failed"), "hidden")
		if output.Len() != 0 {
			t.Fatalf("got disabled log output %q", output.String())
		}
	})
}

func TestParseLevel(t *testing.T) {
	t.Run("defaults to info", func(t *testing.T) {
		level, err := logging.ParseLevel("")
		if err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		logger := logging.New(&output, "test-app", level)
		logger.Debug("hidden")
		logger.Info("visible")
		if got := output.String(); strings.Contains(got, "hidden") || !strings.Contains(got, "visible") {
			t.Fatalf("got default-level logs %q", got)
		}
	})

	t.Run("rejects invalid level", func(t *testing.T) {
		if _, err := logging.ParseLevel("verbose"); err == nil || !strings.Contains(err.Error(), "THOUGHTS_LOG_LEVEL") {
			t.Fatalf("got error %v, want invalid THOUGHTS_LOG_LEVEL error", err)
		}
	})
}

func logFromApplication(logger logging.Logger) string {
	_, file, line, _ := runtime.Caller(0)
	logger.Info("caller")
	return fmt.Sprintf("%s:%d", filepath.Base(file), line+1)
}
