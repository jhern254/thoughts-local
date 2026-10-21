package data

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSubjectAliasConflictError(t *testing.T) {
	t.Run("retains duplicate identity and driver cause without names in error text", func(t *testing.T) {
		cause := errors.New("PRIVATE-CAUSE")
		conflict := &SubjectAliasConflictError{
			cause:       cause,
			AliasName:   "PRIVATE-ALIAS",
			SubjectID:   7,
			SubjectName: "PRIVATE-SUBJECT",
		}
		wrapped := fmt.Errorf("create subject: %w", conflict)
		if !errors.Is(wrapped, ErrDuplicateRecord) || !errors.Is(wrapped, cause) {
			t.Fatalf("error identities: got %v, want duplicate and original cause", wrapped)
		}
		if strings.Contains(wrapped.Error(), "PRIVATE") {
			t.Fatalf("error text: got %q, want fixed text without authored values or raw cause", wrapped.Error())
		}
	})
}
