package thoughts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
)

type failingService struct{ err error }

func (s failingService) BrowseView(context.Context, string, data.ThoughtViewRequest) (data.ThoughtView, error) {
	return data.ThoughtView{}, s.err
}

func (s failingService) ListUnassigned(context.Context, string) ([]data.Thought, error) {
	return nil, s.err
}

func (s failingService) List(context.Context, string, int64) ([]data.Thought, error) {
	return nil, s.err
}
func (s failingService) Get(context.Context, string, int64) (*data.Thought, error) { return nil, s.err }
func (s failingService) Create(context.Context, string, string, *int64, time.Time) (*data.Thought, error) {
	return nil, s.err
}

func TestModel_FailurePrivacy(t *testing.T) {
	t.Run("thought browse view logs only approved fields and preserves its original failure", func(t *testing.T) {
		var logs bytes.Buffer
		logger, err := logging.New(&logs, "test", "info")
		if err != nil {
			t.Fatal(err)
		}
		private := fmt.Errorf("PRIVATE-BROWSE-MARKER: %w", data.ErrDatabaseBusy)
		m := New(t.Context(), "u", failingService{err: private}, logger)
		cmd := m.OpenBrowseThoughtsView(testutils.NewFakeThoughtStore())
		m = runBrowseThoughtsCommand(m, cmd)
		if m.err != private || m.loading || !m.Browsing() {
			t.Fatal("failure lost original error or navigation")
		}
		var event map[string]any
		if err := json.Unmarshal(logs.Bytes(), &event); err != nil {
			t.Fatal(err)
		}
		if len(event) != 7 || event["operation"] != "thought_list" || event["category"] != "database_busy" || event["level"] != "error" || event["message"] != "operation failed" || event["application"] != "test" || event["time"] == nil || event["caller"] == nil {
			t.Fatalf("unapproved event: %v", event)
		}
		if strings.Contains(logs.String()+m.View(), "PRIVATE-BROWSE-MARKER") {
			t.Fatal("private error escaped")
		}
	})
	t.Run("unassigned list failure retains original error and logs safe metadata", func(t *testing.T) {
		var logs bytes.Buffer
		logger, err := logging.New(&logs, "test", "info")
		if err != nil {
			t.Fatal(err)
		}
		private := errors.New("PRIVATE-MISC-LIST")
		m := New(context.Background(), "u", failingService{err: private}, logger)
		cmd := m.OpenUnassigned()
		m, _ = m.Update(cmd())
		if m.err != private || m.loading || !m.Browsing() {
			t.Fatal("list failure lost original error or usable navigation")
		}
		var event map[string]any
		if err := json.Unmarshal(logs.Bytes(), &event); err != nil {
			t.Fatal(err)
		}
		if len(event) != 7 || event["operation"] != "thought_list" || event["category"] != "unexpected_failure" || event["level"] != "error" || event["message"] != "operation failed" || event["application"] != "test" || event["time"] == nil || event["caller"] == nil {
			t.Fatalf("got unapproved fields %v", event)
		}
		if strings.Contains(logs.String()+m.View(), "PRIVATE-MISC-LIST") {
			t.Fatal("private error escaped")
		}
	})
	for _, tt := range []struct {
		name     string
		err      error
		category string
	}{
		{"unknown failure", errors.New("private-marker"), "unexpected_failure"},
		{"recognized infrastructure failure", fmt.Errorf("private-marker: %w", data.ErrDatabaseBusy), "database_busy"},
		{"expected validation", &thought.ValidationError{Fields: map[string]string{"thought": "private-marker"}}, ""},
		{"mixed failure", errors.Join(data.ErrRecordNotFound, errors.New("private-marker")), "unexpected_failure"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger, err := logging.New(&logs, "test", "info")
			if err != nil {
				t.Fatal(err)
			}
			m := New(context.Background(), "u", failingService{err: tt.err}, logger)
			id := int64(7)
			m.subjectID = &id
			m.screen = create
			m.input.SetValue("private-marker")
			cmd := m.createThought(m.input.Value())
			m, _ = m.Update(cmd())
			if m.err != tt.err || m.input.Value() != "private-marker" || !m.input.Focused() {
				t.Fatal("lost original error or input")
			}
			if strings.Contains(m.errMessage+logs.String(), "private-marker") {
				t.Fatal("private error escaped")
			}
			if tt.category == "" {
				if logs.Len() != 0 {
					t.Fatalf("expected outcome logged: %s", &logs)
				}
				return
			}
			var event map[string]any
			if err := json.Unmarshal(logs.Bytes(), &event); err != nil {
				t.Fatal(err)
			}
			if len(event) != 7 || event["level"] != "error" || event["category"] != tt.category || event["operation"] != "thought_create" || event["message"] != "operation failed" || event["application"] != "test" || event["caller"] == nil || event["time"] == nil {
				t.Fatalf("unexpected fields %v", event)
			}
		})
	}
}
