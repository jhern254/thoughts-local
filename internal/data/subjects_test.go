package data

import (
	"strings"
	"testing"

	"github.com/jhern254/go-thoughts/internal/validator"
)

func TestValidateSubjectCreate(t *testing.T) {
	t.Run("does not require subject ID", func(t *testing.T) {
		subject := &Subject{UserID: "test-user", SubjectName: "coding"}
		v := validator.NewValidator()

		ValidateSubjectCreate(v, subject)

		if !v.Valid() {
			t.Fatalf("expected valid subject creation input, got errors: %v", v.Errors)
		}
	})

	t.Run("accepts 255 Unicode characters", func(t *testing.T) {
		subject := &Subject{UserID: "test-user", SubjectName: strings.Repeat("界", 255)}
		v := validator.NewValidator()

		ValidateSubjectCreate(v, subject)

		if !v.Valid() {
			t.Fatalf("expected valid subject creation input, got errors: %v", v.Errors)
		}
	})

	t.Run("rejects more than 255 trimmed characters", func(t *testing.T) {
		subject := &Subject{UserID: "test-user", SubjectName: " " + strings.Repeat("a", 256) + " "}
		v := validator.NewValidator()

		ValidateSubjectCreate(v, subject)

		if _, ok := v.Errors["subject_name"]; !ok {
			t.Fatalf("expected subject_name error, got %v", v.Errors)
		}
	})

	t.Run("does not rewrite valid whitespace", func(t *testing.T) {
		const name = "  learn   Go  "
		subject := &Subject{UserID: "test-user", SubjectName: name}
		v := validator.NewValidator()

		ValidateSubjectCreate(v, subject)

		if !v.Valid() {
			t.Fatalf("expected valid subject creation input, got errors: %v", v.Errors)
		}
		if subject.SubjectName != name {
			t.Fatalf("got subject name %q, want %q", subject.SubjectName, name)
		}
	})
}
