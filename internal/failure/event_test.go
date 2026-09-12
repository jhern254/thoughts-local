package failure

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/event"
	"github.com/jhern254/go-thoughts/internal/logging"
)

func TestClassify_Event(t *testing.T) {
	t.Run("expected event conflicts are not operational failures", func(t *testing.T) {
		for _, err := range []error{data.ErrEventOverlap, data.ErrEventVersionConflict, data.ErrEventStateConflict, data.ErrInvalidEventInterval, &event.ValidationError{}} {
			if _, emit := Classify(logging.EventCreate, fmt.Errorf("PRIVATE: %w", err)); emit {
				t.Fatalf("logged expected %T", err)
			}
		}
	})
	t.Run("joined failures are not suppressed by an expected event outcome", func(t *testing.T) {
		category, emit := Classify(logging.EventEnd, errors.Join(data.ErrEventStateConflict, errors.New("PRIVATE")))
		if !emit || category != logging.UnexpectedFailure {
			t.Fatal("suppressed mixed event failure")
		}
	})
}
