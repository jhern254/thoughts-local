package moonshine

import (
	"context"
	"unicode/utf8"

	"github.com/jhern254/go-thoughts/internal/speech"
)

type nativeStream interface {
	AddAudio(audioSamples []float32, sampleRateHz int) error
	Transcribe() ([]string, error)
	Close() error
}

// Stream owns one recording's native speech state. Copies share ownership.
// Its parent keeps the model loaded and closes this stream before freeing it.
type Stream struct{ state *streamState }
type streamState struct {
	parent        *transcriberState
	nativeBackend nativeStream
	closed        bool
	closeErr      error
}

// StartStream starts one recording without reloading the model. Close the active
// stream before starting another. Creation and native calls share the runtime gate.
func (transcriber *Transcriber) StartStream(ctx context.Context) (*Stream, error) {
	if err := acquireMoonshineRuntime(ctx); err != nil {
		return nil, err
	}
	defer releaseMoonshineRuntime()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	parent := transcriber.state
	if parent == nil || parent.closed || parent.nativeBackend == nil {
		return nil, speech.ErrClosed
	}
	if parent.activeStream != nil {
		return nil, speech.ErrTranscription
	}
	nativeBackend, err := parent.nativeBackend.StartStream()
	if err != nil {
		return nil, withLifecycleOrCategory(ctx, speech.ErrTranscription, err)
	}
	stream := &Stream{state: &streamState{parent: parent, nativeBackend: nativeBackend}}
	if err := ctx.Err(); err != nil {
		return nil, withErrorCategory(err, stream.state.closeWithRuntimeHeld())
	}
	parent.activeStream = stream.state
	return stream, nil
}

// AddAudio accepts nonempty mono float PCM in [-1, 1] and returns the cumulative
// transcript. It borrows samples only until return. A native call cannot be
// interrupted; cancellation discards its result once control returns to Go.
func (stream *Stream) AddAudio(ctx context.Context, audioSamples []float32, sampleRateHz int) (speech.Transcript, error) {
	if err := ctx.Err(); err != nil {
		return speech.Transcript{}, err
	}
	if err := validatePCM(ctx, audioSamples, sampleRateHz); err != nil {
		return speech.Transcript{}, err
	}
	if err := acquireMoonshineRuntime(ctx); err != nil {
		return speech.Transcript{}, err
	}
	defer releaseMoonshineRuntime()
	if err := ctx.Err(); err != nil {
		return speech.Transcript{}, err
	}
	if stream.state == nil || stream.state.closed || stream.state.parent.closed {
		return speech.Transcript{}, speech.ErrClosed
	}
	if err := stream.state.nativeBackend.AddAudio(audioSamples, sampleRateHz); err != nil {
		return speech.Transcript{}, withLifecycleOrCategory(ctx, speech.ErrTranscription, err)
	}
	if err := ctx.Err(); err != nil {
		return speech.Transcript{}, err
	}
	transcriptLines, err := stream.state.nativeBackend.Transcribe()
	if err != nil {
		return speech.Transcript{}, withLifecycleOrCategory(ctx, speech.ErrTranscription, err)
	}
	if err := ctx.Err(); err != nil {
		return speech.Transcript{}, err
	}
	transcript := speech.Transcript{Text: joinTranscriptLines(transcriptLines)}
	if !utf8.ValidString(transcript.Text) {
		return speech.Transcript{}, speech.ErrTranscription
	}
	if err := ctx.Err(); err != nil {
		return speech.Transcript{}, err
	}
	return transcript, nil
}

// Close waits for native work, stops and frees the stream without final-draining
// text. Thoughts revokes recording authority on Stop, so later text is inert.
// Repeated calls return the stored outcome without releasing resources again.
func (stream *Stream) Close() error {
	moonshineRuntimeGate <- struct{}{}
	defer releaseMoonshineRuntime()
	if stream.state == nil {
		return nil
	}
	return stream.state.closeWithRuntimeHeld()
}

func (stream *streamState) closeWithRuntimeHeld() error {
	if stream.closed {
		return stream.closeErr
	}
	stream.closed = true
	if err := stream.nativeBackend.Close(); err != nil {
		stream.closeErr = withErrorCategory(speech.ErrRuntime, err)
	}
	stream.nativeBackend = nil
	if stream.parent.activeStream == stream {
		stream.parent.activeStream = nil
	}
	return stream.closeErr
}
