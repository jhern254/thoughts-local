package diagnostics

import (
	"errors"
	"fmt"
	"maps"
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
		{"intermediate validation type", fmt.Errorf(private+": %w", validationWrapper{errors.New(private)}), "Subject name must be between 1 and 255 characters long."},
		{"unknown", errors.New(private), fallback},
		{"mixed", errors.Join(data.ErrDuplicateRecord, errors.New(private)), fallback},
		{"wrapped CLI aggregate", fmt.Errorf(private+": %w", cliAggregate{data.ErrDuplicateRecord, errors.New(private)}), fallback},
		{"controlled validation", &subject.ValidationError{Fields: map[string]string{"subject_name": "must be between 1 and 255 characters long"}}, "Subject name must be between 1 and 255 characters long."},
		{"untrusted validation", &subject.ValidationError{Fields: map[string]string{private: private}}, "The subject details are invalid."},
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

func TestValidationFields(t *testing.T) {
	const private = "PRIVATE-DIAGNOSTIC-MARKER"
	for _, tc := range []struct {
		name   string
		fields map[string]string
	}{
		{"owner", map[string]string{"user_id": "must be provided"}},
		{"subject length", map[string]string{"subject_name": "must be between 1 and 255 characters long"}},
		{"empty thought", map[string]string{"thought": "must be provided"}},
		{"thought length", map[string]string{"thought": "must not be more than 1000000 characters long"}},
	} {
		t.Run("preserves approved "+tc.name+" guidance", func(t *testing.T) {
			if got := ValidationFields(tc.fields); !maps.Equal(got, tc.fields) {
				t.Fatalf("got fields %v, want %v", got, tc.fields)
			}
		})
	}
	for _, tc := range []struct {
		name   string
		fields map[string]string
	}{
		{"empty", nil}, {"unknown field", map[string]string{private: "must be provided"}},
		{"unknown message", map[string]string{"subject_name": private}},
		{"mixed guidance", map[string]string{"thought": "must be provided", private: private}},
	} {
		t.Run(tc.name+" uses safe fallback", func(t *testing.T) {
			want := map[string]string{"request": "The submitted values are invalid."}
			if got := ValidationFields(tc.fields); !maps.Equal(got, want) {
				t.Fatalf("got fields %v, want %v", got, want)
			}
		})
	}
}
