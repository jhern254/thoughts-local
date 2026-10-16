package moonshine

import (
	"context"
	"errors"
	"testing"
)

func TestCheckRuntime(t *testing.T) {
	t.Run("cancellation prevents native work", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if err := CheckRuntime(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want cancellation", err)
		}
	})
}
