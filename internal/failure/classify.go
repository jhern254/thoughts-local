// Package failure classifies errors at presentation boundaries, independently
// of logging emission and user-facing error rendering.
package failure

import (
	"errors"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/thought"
)

// Classify returns approved log metadata and whether the failure should be
// logged. Known operational failures are not expected application outcomes.
func Classify(operation logging.Operation, err error) (logging.FailureCategory, bool) {
	if err == nil {
		return logging.UnexpectedFailure, false
	}
	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		if _, joined := cause.(interface{ Unwrap() []error }); joined {
			// An expected branch must not hide another failure. Do not guess
			// which category best describes a combination of causes.
			return logging.UnexpectedFailure, true
		}
	}
	var validation *thought.ValidationError
	switch {
	case errors.Is(err, data.ErrDatabaseBusy):
		return logging.DatabaseBusy, true
	case errors.Is(err, data.ErrDatabaseReadOnly):
		return logging.DatabaseReadOnly, true
	case (operation == logging.SubjectCreate || operation == logging.SubjectUpdate) && subject.IsExpectedError(err):
		return logging.UnexpectedFailure, false
	case (operation == logging.SubjectGet || operation == logging.SubjectDelete) && errors.Is(err, data.ErrRecordNotFound):
		return logging.UnexpectedFailure, false
	case operation == logging.ThoughtUpdate && (errors.As(err, &validation) || errors.Is(err, data.ErrRecordNotFound) || errors.Is(err, data.ErrVersionConflict)):
		return logging.UnexpectedFailure, false
	case operation == logging.ThoughtDelete && (errors.Is(err, data.ErrRecordNotFound) || errors.Is(err, data.ErrVersionConflict)):
		return logging.UnexpectedFailure, false
	case operation == logging.ThoughtCreate && (errors.As(err, &validation) || errors.Is(err, data.ErrRecordNotFound)):
		return logging.UnexpectedFailure, false
	case operation == logging.ThoughtGet && errors.Is(err, data.ErrRecordNotFound):
		return logging.UnexpectedFailure, false
	default:
		return logging.UnexpectedFailure, true
	}
}
