//go:build !moonshine || !cgo || !((linux && amd64) || (darwin && !ios && (amd64 || arm64)) || (windows && amd64))

package moonshine

import (
	"context"
	"errors"
	"testing"

	"github.com/jhern254/go-thoughts/internal/modelassets"
	"github.com/jhern254/go-thoughts/internal/speech"
)

func TestNativeRuntime_Unavailable(t *testing.T) {
	t.Run("rejects loading without accessing model files", func(t *testing.T) {
		nativeBackend, err := openNativeTranscriber(context.Background(), nil, modelassets.Installation{})
		if nativeBackend != nil || !errors.Is(err, speech.ErrRuntime) {
			t.Fatalf("got %v, %v, want unavailable runtime", nativeBackend, err)
		}
	})
}
