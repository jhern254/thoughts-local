package tui

import (
	"errors"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
)

// An aggregate cannot safely establish a single expected outcome.
func diagnosticCause(err error) error {
	for range 32 {
		switch e := err.(type) {
		case interface{ Unwrap() []error }, interface{ Errors() []error }:
			return nil
		case interface{ Unwrap() error }:
			err = e.Unwrap()
		default:
			return err
		}
	}
	return nil
}

func subjectDiagnostic(err error) string {
	cause := diagnosticCause(err)
	var validation *subject.ValidationError
	switch {
	case errors.Is(cause, data.ErrRecordNotFound):
		return "The requested resource was not found."
	case errors.Is(cause, data.ErrDuplicateRecord):
		return "A subject with that name already exists."
	case errors.As(cause, &validation):
		if len(validation.Fields) == 1 && validation.Fields["subject_name"] == "must be between 1 and 255 characters long" {
			return "Subject name must be between 1 and 255 characters long."
		}
		return "The subject details are invalid."
	default:
		return ""
	}
}

func subjectErrorMessage(err error, fallback string) string {
	if message := subjectDiagnostic(err); message != "" {
		return message
	}
	return fallback
}
