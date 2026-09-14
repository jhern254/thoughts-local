package failure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/thought"
)

func TestClassify(t *testing.T) {
	for _, tc := range []struct {
		name     string
		op       logging.Operation
		err      error
		category logging.FailureCategory
		emit     bool
	}{
		{"ignores thought update validation", logging.ThoughtUpdate, &thought.ValidationError{}, logging.UnexpectedFailure, false},
		{"ignores thought update conflict", logging.ThoughtUpdate, data.ErrVersionConflict, logging.UnexpectedFailure, false},
		{"ignores thought delete conflict", logging.ThoughtDelete, data.ErrVersionConflict, logging.UnexpectedFailure, false},
		{"ignores thought delete not found", logging.ThoughtDelete, data.ErrRecordNotFound, logging.UnexpectedFailure, false},
		{"retains thought delete validation", logging.ThoughtDelete, &thought.ValidationError{}, logging.UnexpectedFailure, true},
		{"retains busy thought update", logging.ThoughtUpdate, data.ErrDatabaseBusy, logging.DatabaseBusy, true},
		{"retains combined thought conflict", logging.ThoughtDelete, errors.Join(data.ErrVersionConflict, errors.New("private")), logging.UnexpectedFailure, true},
		{"ignores update validation", logging.SubjectUpdate, &subject.ValidationError{}, logging.UnexpectedFailure, false},
		{"ignores duplicate update", logging.SubjectUpdate, data.ErrDuplicateRecord, logging.UnexpectedFailure, false},
		{"ignores missing update", logging.SubjectUpdate, data.ErrRecordNotFound, logging.UnexpectedFailure, false},
		{"ignores missing delete", logging.SubjectDelete, data.ErrRecordNotFound, logging.UnexpectedFailure, false},
		{"retains unexpected delete validation", logging.SubjectDelete, &subject.ValidationError{}, logging.UnexpectedFailure, true},
		{"retains busy update", logging.SubjectUpdate, data.ErrDatabaseBusy, logging.DatabaseBusy, true},
		{"retains read-only delete", logging.SubjectDelete, data.ErrDatabaseReadOnly, logging.DatabaseReadOnly, true},
		{"retains combined update failure", logging.SubjectUpdate, errors.Join(data.ErrDuplicateRecord, errors.New("private")), logging.UnexpectedFailure, true},
		{"ignores nil", logging.SubjectCreate, nil, logging.UnexpectedFailure, false},
		{"recognizes wrapped busy failure", logging.SubjectCreate, fmt.Errorf("write: %w", data.ErrDatabaseBusy), logging.DatabaseBusy, true},
		{"recognizes read-only startup", logging.ApplicationStart, data.ErrDatabaseReadOnly, logging.DatabaseReadOnly, true},
		{"ignores create validation", logging.SubjectCreate, &subject.ValidationError{}, logging.UnexpectedFailure, false},
		{"ignores wrapped duplicate create", logging.SubjectCreate, fmt.Errorf("create: %w", data.ErrDuplicateRecord), logging.UnexpectedFailure, false},
		{"ignores unavailable create owner", logging.SubjectCreate, data.ErrRecordNotFound, logging.UnexpectedFailure, false},
		{"ignores missing get result", logging.SubjectGet, data.ErrRecordNotFound, logging.UnexpectedFailure, false},
		{"does not suppress missing list failure", logging.SubjectList, data.ErrRecordNotFound, logging.UnexpectedFailure, true},
		{"does not suppress startup validation", logging.ApplicationStart, &subject.ValidationError{}, logging.UnexpectedFailure, true},
		{"does not suppress duplicate get failure", logging.SubjectGet, data.ErrDuplicateRecord, logging.UnexpectedFailure, true},
		{"retains unknown failures", logging.SubjectCreate, errors.New("private"), logging.UnexpectedFailure, true},
		{"does not silently suppress cancellation", logging.SubjectCreate, context.Canceled, logging.UnexpectedFailure, true},
		{"retains exceeded deadlines", logging.SubjectList, context.DeadlineExceeded, logging.UnexpectedFailure, true},
		{"does not suppress expected and unknown joined failures", logging.SubjectCreate, errors.Join(data.ErrDuplicateRecord, errors.New("private")), logging.UnexpectedFailure, true},
		{"does not suppress mixed expected and operational failures", logging.SubjectCreate, errors.Join(data.ErrRecordNotFound, data.ErrDatabaseBusy), logging.UnexpectedFailure, true},
		{"does not classify one branch of wrapped mixed failures", logging.SubjectCreate, fmt.Errorf("write: %w", errors.Join(data.ErrDatabaseReadOnly, data.ErrDuplicateRecord)), logging.UnexpectedFailure, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			category, emit := Classify(tc.op, tc.err)
			if category != tc.category || emit != tc.emit {
				t.Fatalf("got (%v, %t), want (%v, %t)", category, emit, tc.category, tc.emit)
			}
		})
	}
}

func TestClassify_PrivateCauses(t *testing.T) {
	const marker = "PRIVATE-THOUGHT-7bd328"
	for _, tc := range []struct {
		name     string
		cause    error
		category string
	}{
		{"busy", data.ErrDatabaseBusy, "database_busy"},
		{"read-only", data.ErrDatabaseReadOnly, "database_read_only"},
		{"unknown", errors.New(marker), "unexpected_failure"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := fmt.Errorf(marker+": %w", tc.cause)
			var output bytes.Buffer
			logger, setupErr := logging.New(&output, "test", "info")
			if setupErr != nil {
				t.Fatal(setupErr)
			}
			category, emit := Classify(logging.SubjectCreate, err)
			if !emit {
				t.Fatal("operational failure was suppressed")
			}
			logger.Failure(logging.SubjectCreate, category)
			if strings.Contains(output.String(), marker) {
				t.Fatal("private cause entered log")
			}
			var event map[string]any
			if decodeErr := json.Unmarshal(output.Bytes(), &event); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if event["time"] == nil || event["caller"] == nil {
				t.Fatal("missing diagnostic context")
			}
			delete(event, "time")
			delete(event, "caller")
			want := map[string]any{"level": "error", "application": "test", "operation": "subject_create", "category": tc.category, "message": "operation failed"}
			if !reflect.DeepEqual(event, want) {
				t.Fatalf("got %v, want %v", event, want)
			}
			if !errors.Is(err, tc.cause) || !strings.Contains(err.Error(), marker) {
				t.Fatal("original error no longer available")
			}
		})
	}
}
