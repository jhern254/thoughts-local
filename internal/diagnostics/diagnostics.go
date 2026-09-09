// Package diagnostics selects safe presentation messages without classifying operational failures.
package diagnostics

import (
	"errors"
	"sort"
	"strings"

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
		fields := validation.PublicFields()
		if len(fields) != 0 {
			names := make([]string, 0, len(fields))
			for name := range fields {
				names = append(names, name)
			}
			sort.Strings(names)
			messages := make([]string, 0, len(names))
			for _, name := range names {
				messages = append(messages, name+": "+fields[name])
			}
			return strings.Join(messages, "; ")
		}
		return "The subject details are invalid."
	default:
		return fallback
	}
}
