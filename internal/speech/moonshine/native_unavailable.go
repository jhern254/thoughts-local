//go:build !moonshine || !cgo || !linux || !amd64

package moonshine

import (
	"context"
	"os"

	"github.com/jhern254/go-thoughts/internal/modelassets"
	"github.com/jhern254/go-thoughts/internal/speech"
)

func openNativeTranscriber(_ context.Context, _ *os.Root, _ modelassets.Installation) (nativeTranscriber, error) {
	return nil, speech.ErrRuntime
}
