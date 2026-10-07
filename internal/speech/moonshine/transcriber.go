// Package moonshine transcribes in-memory PCM with the application-approved
// Moonshine Small Streaming English model. Opening never downloads assets.
//
// Native inference is synchronous and cannot be interrupted. Cancellation before
// entry prevents inference; cancellation during inference discards its result
// after the native call finishes. Close waits for native work before freeing it.
package moonshine

import (
	"context"
	"errors"
	"math"
	"os"
	"strings"

	"github.com/jhern254/go-thoughts/internal/modelassets"
	"github.com/jhern254/go-thoughts/internal/speech"
)

// The v0.1.5 C API has process-wide option state and registry accesses outside
// its registry mutex. This gate protects all adapter calls, including loading,
// freeing and copying borrowed results, across transcriber instances. Other
// consumers must not call this shared library directly alongside the adapter.
var nativeCallGate = make(chan struct{}, 1)

type nativeTranscriber interface {
	// Returned text views remain borrowed until the next native call or Close.
	Transcribe(audioSamples []float32, sampleRateHz int) ([][]byte, error)
	Close() error
}

// Transcriber owns its native handle and model buffers. Calls are serialized;
// callers must not mutate audioSamples until Transcribe returns. Do not copy a
// Transcriber value. Construct it with Open and release it explicitly with Close.
type Transcriber struct {
	nativeBackend nativeTranscriber
	closed        bool
	closeErr      error
}

// Open fully verifies the approved installation before loading it. Keep the
// root open until Open returns and its model files unmodified until Close.
func Open(ctx context.Context, modelRoot *os.Root) (*Transcriber, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	installer, err := modelassets.NewInstaller(modelRoot)
	if err != nil {
		return nil, withErrorCategory(speech.ErrModelLoad, err)
	}
	installation, err := installer.VerifyInstallation(ctx, modelassets.MoonshineSmallStreamingEnglish)
	if err != nil {
		return nil, withLifecycleOrCategory(ctx, speech.ErrModelLoad, err)
	}
	return openVerifiedTranscriber(ctx, modelRoot, installation, openNativeTranscriber)
}

func openVerifiedTranscriber(
	ctx context.Context,
	modelRoot *os.Root,
	installation modelassets.Installation,
	loadNative func(context.Context, *os.Root, modelassets.Installation) (nativeTranscriber, error),
) (*Transcriber, error) {
	if err := acquireNativeCall(ctx); err != nil {
		return nil, err
	}
	defer releaseNativeCall()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	nativeBackend, err := loadNative(ctx, modelRoot, installation)
	if err != nil {
		return nil, withLifecycleOrCategory(ctx, speech.ErrModelLoad, err)
	}
	if lifecycleErr := ctx.Err(); lifecycleErr != nil {
		if closeErr := nativeBackend.Close(); closeErr != nil {
			return nil, withErrorCategory(lifecycleErr, withErrorCategory(speech.ErrRuntime, closeErr))
		}
		return nil, lifecycleErr
	}
	return &Transcriber{nativeBackend: nativeBackend}, nil
}

func (transcriber *Transcriber) Transcribe(ctx context.Context, audioSamples []float32, sampleRateHz int) (speech.Transcript, error) {
	if err := ctx.Err(); err != nil {
		return speech.Transcript{}, err
	}
	if err := validatePCM(ctx, audioSamples, sampleRateHz); err != nil {
		return speech.Transcript{}, err
	}
	if err := acquireNativeCall(ctx); err != nil {
		return speech.Transcript{}, err
	}
	defer releaseNativeCall()
	if err := ctx.Err(); err != nil {
		return speech.Transcript{}, err
	}
	if transcriber.closed || transcriber.nativeBackend == nil {
		return speech.Transcript{}, speech.ErrClosed
	}
	transcriptLines, nativeErr := transcriber.nativeBackend.Transcribe(audioSamples, sampleRateHz)
	if err := ctx.Err(); err != nil {
		return speech.Transcript{}, withErrorCategory(err, nativeErr)
	}
	if nativeErr != nil {
		return speech.Transcript{}, withErrorCategory(speech.ErrTranscription, nativeErr)
	}
	var transcriptText strings.Builder
	for _, lineText := range transcriptLines {
		if len(lineText) == 0 {
			continue
		}
		if transcriptText.Len() > 0 {
			transcriptText.WriteByte(' ')
		}
		transcriptText.Write(lineText)
	}
	if err := ctx.Err(); err != nil {
		return speech.Transcript{}, err
	}
	return speech.Transcript{Text: transcriptText.String()}, nil
}

// Close waits for active native work and attempts all owned resource cleanup.
// Repeated calls return the original outcome without releasing resources again.
func (transcriber *Transcriber) Close() error {
	nativeCallGate <- struct{}{}
	defer releaseNativeCall()
	if transcriber.closed {
		return transcriber.closeErr
	}
	transcriber.closed = true
	if transcriber.nativeBackend != nil {
		if err := transcriber.nativeBackend.Close(); err != nil {
			transcriber.closeErr = withErrorCategory(speech.ErrRuntime, err)
		}
		transcriber.nativeBackend = nil
	}
	return transcriber.closeErr
}

func validatePCM(ctx context.Context, audioSamples []float32, sampleRateHz int) error {
	// The batch implementation narrows audio length to int32 before processing it.
	if sampleRateHz <= 0 || int64(sampleRateHz) > math.MaxInt32 {
		return speech.ErrInvalidAudio
	}
	if len(audioSamples) == 0 || int64(len(audioSamples)) > math.MaxInt32 {
		return speech.ErrInvalidAudio
	}
	for sampleIndex, sample := range audioSamples {
		if sampleIndex%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if math.IsNaN(float64(sample)) || math.IsInf(float64(sample), 0) {
			return speech.ErrInvalidAudio
		}
		if sample < -1 || sample > 1 {
			return speech.ErrInvalidAudio
		}
	}
	return nil
}
func acquireNativeCall(ctx context.Context) error {
	select {
	case nativeCallGate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func releaseNativeCall() { <-nativeCallGate }

// NativeStatusError preserves only the numeric upstream status for errors.As.
// No native diagnostic strings, authored text or PCM are retained in this error.
type NativeStatusError struct{ Code int32 }

func (nativeStatus *NativeStatusError) Error() string { return "moonshine native operation failed" }

// categorizedError exposes a fixed safe category, retaining causes only for
// errors.Is/errors.As. Unwrapped causes must not be logged.
type categorizedError struct {
	category error
	cause    error
}

func (categorized *categorizedError) Error() string { return categorized.category.Error() }
func (categorized *categorizedError) Unwrap() []error {
	return []error{categorized.category, categorized.cause}
}
func withErrorCategory(category, cause error) error {
	if cause == nil {
		return category
	}
	return &categorizedError{category: category, cause: cause}
}
func withLifecycleOrCategory(ctx context.Context, category, cause error) error {
	if err := ctx.Err(); err != nil {
		return withErrorCategory(err, cause)
	}
	if errors.Is(cause, context.Canceled) {
		return withErrorCategory(context.Canceled, cause)
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		return withErrorCategory(context.DeadlineExceeded, cause)
	}
	if errors.Is(cause, speech.ErrRuntime) {
		return withErrorCategory(speech.ErrRuntime, cause)
	}
	return withErrorCategory(category, cause)
}
