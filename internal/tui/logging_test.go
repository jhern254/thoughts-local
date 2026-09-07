package tui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/subject"
)

func TestSubjectModel_Logging(t *testing.T) {
	for _, operation := range []string{"create", "list", "get"} {
		for _, outcome := range []struct {
			name   string
			err    error
			logged bool
		}{
			{"unexpected failure", errors.New("database unavailable"), true},
			{"validation", &subject.ValidationError{}, false},
			{"not found", data.ErrRecordNotFound, false},
			{"duplicate", data.ErrDuplicateRecord, false},
		} {
			t.Run(operation+" handles "+outcome.name, func(t *testing.T) {
				var logs bytes.Buffer
				logger, err := logging.New(&logs, "test", "info")
				if err != nil {
					t.Fatal(err)
				}
				model := newSubjectTestModel(&subjectServiceStub{})
				model.logger = logger
				failure := fmt.Errorf("wrapped: %w", outcome.err)
				var message tea.Msg
				switch operation {
				case "create":
					message = subjectCreatedMsg{err: failure}
				case "list":
					message = subjectsListedMsg{err: failure}
				case "get":
					message = subjectFoundMsg{err: failure}
				}
				updated, _ := model.Update(message)
				if !errors.Is(updated.(Model).subjects.err, failure) {
					t.Fatal("lost subject error")
				}
				if !outcome.logged {
					if logs.Len() != 0 {
						t.Fatalf("unexpected logs: %s", &logs)
					}
					return
				}
				var event map[string]any
				if err := json.Unmarshal(logs.Bytes(), &event); err != nil {
					t.Fatal(err)
				}
				if event["operation"] != operation || event["level"] != "error" || event["error"] != failure.Error() {
					t.Fatalf("incorrect failure event: %v", event)
				}
			})
		}
	}
}
