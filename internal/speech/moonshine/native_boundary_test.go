//go:build moonshine && cgo && ((linux && amd64) || (darwin && !ios && (amd64 || arm64)) || (windows && amd64))

package moonshine

import (
	"context"
	"errors"
	"testing"

	"github.com/jhern254/go-thoughts/internal/speech"
)

func TestNativeModelFiles_Construction(t *testing.T) {
	t.Run("rejects incomplete model buffer sets before crossing the C boundary", func(t *testing.T) {
		nativeBackend, err := loadMappedTranscriber(context.Background(), &mappedModelFiles{})
		if nativeBackend != nil || !errors.Is(err, speech.ErrModelLoad) {
			t.Fatalf("got %v, %v, want rejected incomplete model buffers", nativeBackend, err)
		}
	})
}
