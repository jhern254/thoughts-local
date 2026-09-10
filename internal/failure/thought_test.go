package failure

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/thought"
)

func TestClassify_Thoughts(t *testing.T) {
	for _, tt := range []struct {
		name     string
		op       logging.Operation
		err      error
		category logging.FailureCategory
		emit     bool
	}{
		{"create validation is expected", logging.ThoughtCreate, &thought.ValidationError{}, logging.UnexpectedFailure, false},
		{"create missing subject is expected", logging.ThoughtCreate, data.ErrRecordNotFound, logging.UnexpectedFailure, false},
		{"get missing thought is expected", logging.ThoughtGet, data.ErrRecordNotFound, logging.UnexpectedFailure, false},
		{"list not found remains operational", logging.ThoughtList, data.ErrRecordNotFound, logging.UnexpectedFailure, true},
		{"counts busy remains operational", logging.ThoughtCountsBySubject, fmt.Errorf("private-marker: %w", data.ErrDatabaseBusy), logging.DatabaseBusy, true},
		{"joined expected error cannot hide failure", logging.ThoughtCreate, errors.Join(&thought.ValidationError{}, errors.New("private-marker")), logging.UnexpectedFailure, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			category, emit := Classify(tt.op, tt.err)
			if category != tt.category || emit != tt.emit {
				t.Fatalf("got %v/%v, want %v/%v", category, emit, tt.category, tt.emit)
			}
		})
	}
}
