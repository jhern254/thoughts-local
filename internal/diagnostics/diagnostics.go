// Package diagnostics selects safe presentation messages without classifying operational failures.
package diagnostics

import (
	"errors"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
)

const DuplicateSubjectMessage = "A subject with that name already exists."

// SingleError returns the original error for a supported, non-aggregate chain.
// Aggregates (including urfave MultiError) and chains exceeding the inspection
// bound return nil so callers select a generic diagnostic, not one branch.
func SingleError(err error) error {
	cause := err
	for range 32 {
		switch e := cause.(type) {
		case interface{ Unwrap() []error }, interface{ Errors() []error }:
			return nil
		case interface{ Unwrap() error }:
			cause = e.Unwrap()
		default:
			return err
		}
	}
	return nil
}

// SubjectMessage selects safe subject guidance independently of logging policy.
func SubjectMessage(err error, fallback string) string {
	cause := SingleError(err)
	var validation *subject.ValidationError
	switch {
	case errors.Is(cause, data.ErrRecordNotFound):
		return "The requested resource was not found."
	case errors.Is(cause, data.ErrDuplicateRecord):
		return DuplicateSubjectMessage
	case errors.As(cause, &validation):
		fields := ValidationFields(validation.Fields)
		if len(fields) == 1 && fields["subject_name"] != "" {
			return "Subject name " + fields["subject_name"] + "."
		}
		return "The subject details are invalid."
	default:
		return fallback
	}
}

// ValidationFields copies only approved field/message pairs; any unknown detail
// makes the whole diagnostic generic. Submitted values are never forwarded.
func ValidationFields(fields map[string]string) map[string]string {
	safe := make(map[string]string)
	for field, message := range fields {
		switch {
		case field == "user_id" && message == "must be provided",
			field == "subject_name" && message == "must be between 1 and 255 characters long",
			field == "thought" && (message == "must be provided" || message == "must not be more than 1000000 characters long"):
			safe[field] = message
		default:
			return map[string]string{"request": "The submitted values are invalid."}
		}
	}
	if len(safe) == 0 {
		return map[string]string{"request": "The submitted values are invalid."}
	}
	return safe
}
