package logging_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jhern254/go-thoughts/internal/logging"
)

func TestLogger_Events(t *testing.T) {
	for _, name := range []string{"started", "stopped", "created", "failure", "unknown mutation", "unknown failure"} {
		t.Run(name+" emits only approved fields from the public caller", func(t *testing.T) {
			var output bytes.Buffer
			logger, err := logging.New(&output, "test", "")
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]any{"application": "test", "level": "info"}
			var file string
			var line int
			switch name {
			case "started":
				_, file, line, _ = runtime.Caller(0)
				logger.Started()
				want["message"] = "starting application"
			case "stopped":
				_, file, line, _ = runtime.Caller(0)
				logger.Stopped()
				want["message"] = "stopped application"
			case "created", "unknown mutation":
				event := logging.SubjectCreated
				if name == "unknown mutation" {
					event = logging.MutationEvent(255)
				}
				_, file, line, _ = runtime.Caller(0)
				logger.Mutation(event, 7)
				want["message"], want["subject_id"] = "subject "+name, float64(7)
				if name == "unknown mutation" {
					want["message"] = name
				}
			case "failure", "unknown failure":
				operation, category := logging.SubjectCreate, logging.UnexpectedFailure
				want["operation"] = "subject_create"
				if name == "unknown failure" {
					operation, category = logging.Operation(255), logging.FailureCategory(255)
					want["operation"] = "unknown"
				}
				_, file, line, _ = runtime.Caller(0)
				logger.Failure(operation, category)
				want["message"], want["level"], want["category"] = "operation failed", "error", "unexpected_failure"
			}
			var event map[string]any
			if err := json.Unmarshal(output.Bytes(), &event); err != nil {
				t.Fatal(err)
			}
			if len(event) != len(want)+2 {
				t.Fatalf("unexpected fields: %v", event)
			}
			for key, value := range want {
				if event[key] != value {
					t.Fatalf("got %s=%v, want %v", key, event[key], value)
				}
			}
			caller, ok := event["caller"].(string)
			if !ok || filepath.Base(caller) != fmt.Sprintf("%s:%d", filepath.Base(file), line+1) {
				t.Fatalf("incorrect caller: %v", event["caller"])
			}
			if event["time"] == nil {
				t.Fatal("missing timestamp")
			}
		})
	}

	t.Run("uses fixed names for each instrumented operation", func(t *testing.T) {
		for operation, want := range map[logging.Operation]string{
			logging.ApplicationStart: "application_start", logging.ApplicationClose: "application_close",
			logging.TUIRun: "tui_run", logging.SubjectCreate: "subject_create",
			logging.SubjectGet: "subject_get", logging.SubjectList: "subject_list",
		} {
			var output bytes.Buffer
			logger, err := logging.New(&output, "test", "")
			if err != nil {
				t.Fatal(err)
			}
			logger.Failure(operation, logging.UnexpectedFailure)
			var event map[string]any
			if err := json.Unmarshal(output.Bytes(), &event); err != nil {
				t.Fatal(err)
			}
			if event["operation"] != want {
				t.Fatalf("got operation %v, want %s", event["operation"], want)
			}
		}
	})
}
