package data

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestTranslateSQLiteError(t *testing.T) {
	t.Run("preserves errors outside the SQLite contract", func(t *testing.T) {
		for _, err := range []error{nil, context.Canceled, context.DeadlineExceeded, ErrRecordNotFound, ErrDuplicateRecord, fmt.Errorf("wrapped: %w", errors.New("private cause"))} {
			if got := TranslateSQLiteError(err); got != err {
				t.Fatalf("got %v, want original error %v", got, err)
			}
		}
	})
	t.Run("preserves combined failures without selecting a branch", func(t *testing.T) {
		err := fmt.Errorf("cleanup: %w", errors.Join(ErrRecordNotFound, errors.New("private cause")))
		if got := TranslateSQLiteError(err); got != err {
			t.Fatalf("got %v, want original combined error", got)
		}
	})
}
