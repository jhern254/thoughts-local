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

func logFromApplication(logger logging.Logger) string {
	_, file, line, _ := runtime.Caller(0)
	logger.Info("created", logging.Fields{"subject_id": int64(7)})
	return fmt.Sprintf("%s:%d", filepath.Base(file), line+1)
}
