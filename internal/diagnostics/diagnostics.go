// Package diagnostics selects safe presentation messages without classifying operational failures.
package diagnostics

import (
	"errors"
	"sort"
	"strings"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/thought"
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

// ThoughtMessage only exposes validator-produced guidance, never arbitrary Fields.
func ThoughtMessage(err error, fallback string) string {
	cause := SingleError(err)
	var validation *thought.ValidationError
	switch {
	case errors.Is(cause, data.ErrRecordNotFound):
		return "The requested resource was not found."
	case errors.As(cause, &validation):
		return validationMessage(validation.PublicFields(), "The thought details are invalid.")
	default:
		return fallback
	}
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
		return validationMessage(validation.PublicFields(), "The subject details are invalid.")
	default:
		return fallback
	}
}

func validationMessage(fields map[string]string, fallback string) string {
	if len(fields) == 0 {
		return fallback
	}
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
