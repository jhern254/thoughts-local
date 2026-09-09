package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
)

// These wrappers expose contracts at an intermediate level, above an unknown cause.
type identityWrapper struct{ error }

func (identityWrapper) Is(target error) bool { return target == data.ErrDuplicateRecord }
func (w identityWrapper) Unwrap() error      { return w.error }

type validationWrapper struct{ error }

func (validationWrapper) As(target any) bool {
	if v, ok := target.(**subject.ValidationError); ok {
		*v = &subject.ValidationError{Fields: map[string]string{"subject_name": "must be between 1 and 255 characters long"}}
		return true
	}
	return false
}
func (w validationWrapper) Unwrap() error { return w.error }

type cliAggregate []error

func (cliAggregate) Error() string     { return "PRIVATE-DIAGNOSTIC-MARKER" }
func (e cliAggregate) Errors() []error { return e }

func TestSubjectMessage(t *testing.T) {
	const private = "PRIVATE-DIAGNOSTIC-MARKER"
	const fallback = "Could not save the subject."
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"wrapped missing", fmt.Errorf(private+": %w", data.ErrRecordNotFound), "The requested resource was not found."},
		{"wrapped duplicate", fmt.Errorf(private+": %w", data.ErrDuplicateRecord), "A subject with that name already exists."},
		{"intermediate identity", fmt.Errorf(private+": %w", identityWrapper{errors.New(private)}), "A subject with that name already exists."},
		{"intermediate validation type", fmt.Errorf(private+": %w", validationWrapper{errors.New(private)}), "The subject details are invalid."},
		{"unknown", errors.New(private), fallback},
		{"mixed", errors.Join(data.ErrDuplicateRecord, errors.New(private)), fallback},
		{"wrapped CLI aggregate", fmt.Errorf(private+": %w", cliAggregate{data.ErrDuplicateRecord, errors.New(private)}), fallback},
		{"controlled validation", fmt.Errorf(private+": %w", &subject.ValidationError{Fields: map[string]string{"subject_name": "must be between 1 and 255 characters long"}}), "The subject details are invalid."},
		{"untrusted validation", fmt.Errorf(private+": %w", &subject.ValidationError{Fields: map[string]string{private: private}}), "The subject details are invalid."},
		{"mixed validation fields", fmt.Errorf(private+": %w", &subject.ValidationError{Fields: map[string]string{"subject_name": "must be between 1 and 255 characters long", private: private}}), "The subject details are invalid."},
		{"empty validation fields", fmt.Errorf(private+": %w", &subject.ValidationError{}), "The subject details are invalid."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := tc.err.Error()
			if got := SubjectMessage(tc.err, fallback); got != tc.want {
				t.Fatalf("got message %q, want %q", got, tc.want)
			}
			if tc.err.Error() != original {
				t.Fatal("got changed error, want original private error available")
			}
		})
	}
}

func TestSingleError(t *testing.T) {
	t.Run("retains original supported chain", func(t *testing.T) {
		err := fmt.Errorf("PRIVATE-DIAGNOSTIC-MARKER: %w", identityWrapper{errors.New("cause")})
		if got := SingleError(err); got != err {
			t.Fatalf("got error %v, want original chain %v", got, err)
		}
	})
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"joined expected errors", errors.Join(data.ErrDuplicateRecord, data.ErrRecordNotFound)},
		{"CLI aggregate", cliAggregate{data.ErrDuplicateRecord}},
	} {
		t.Run(tc.name+" stays ambiguous", func(t *testing.T) {
			if got := SingleError(tc.err); got != nil {
				t.Fatalf("got error %v, want nil for aggregate", got)
			}
		})
	}
}

func TestSubjectMessage_ValidatorFeedback(t *testing.T) {
	t.Run("renders validator guidance without submitted values or wrapper text", func(t *testing.T) {
		const private = "PRIVATE-VALIDATION-MARKER"
		_, original := subject.NewService(nil).Create(context.Background(), "local", strings.Repeat(private, 20))
		var validation *subject.ValidationError
		if !errors.As(original, &validation) {
			t.Fatalf("got error %v, want validation error", original)
		}
		want := "subject_name: " + validation.Fields["subject_name"]
		wrapped := fmt.Errorf(private+": %w", original)
		if got := SubjectMessage(wrapped, "Could not save the subject."); got != want {
			t.Fatalf("got diagnostic %q, want %q", got, want)
		}
		if !errors.Is(wrapped, original) {
			t.Fatal("got lost original error, want retained identity")
		}
		// Mutable internal fields cannot change the validator-owned public feedback.
		validation.Fields["subject_name"] = private
		validation.Fields[private] = private
		if got := SubjectMessage(wrapped, "Could not save the subject."); got != want {
			t.Fatalf("got diagnostic %q, want unchanged guidance %q", got, want)
		}
	})
}
