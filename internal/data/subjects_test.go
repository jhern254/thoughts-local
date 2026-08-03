package data

import (
	"testing"

	"github.com/jhern254/go-thoughts/internal/validator"
)

func TestValidateSubjectCreateDoesNotRequireSubjectID(t *testing.T) {
	subject := &Subject{
		UserID:      "test-user",
		SubjectName: "coding",
	}
	v := validator.NewValidator()

	ValidateSubjectCreate(v, subject)

	if !v.Valid() {
		t.Fatalf("expected valid subject creation input, got errors: %v", v.Errors)
	}
}
