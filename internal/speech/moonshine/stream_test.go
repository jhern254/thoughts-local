package moonshine

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/jhern254/go-thoughts/internal/speech"
)

type fakeNativeStream struct {
	lines                                 []string
	add                                   func() error
	transcribe                            func() error
	close                                 func() error
	addCalls, transcriptCalls, closeCalls int
}

func (stream *fakeNativeStream) AddAudio([]float32, int) error {
	stream.addCalls++
	if stream.add != nil {
		return stream.add()
	}
	return nil
}
func (stream *fakeNativeStream) Transcribe() ([]string, error) {
	stream.transcriptCalls++
	if stream.transcribe != nil {
		if err := stream.transcribe(); err != nil {
			return nil, err
		}
	}
	return stream.lines, nil
}
func (stream *fakeNativeStream) Close() error {
	stream.closeCalls++
	if stream.close != nil {
		return stream.close()
	}
	return nil
}

type fakeStreamingTranscriber struct {
	fakeNativeTranscriber
	stream     *fakeNativeStream
	start      func() error
	startCalls int
}

func (native *fakeStreamingTranscriber) StartStream() (nativeStream, error) {
	native.startCalls++
	if native.start != nil {
		if err := native.start(); err != nil {
			return nil, err
		}
	}
	return native.stream, nil
}
func newStreamingTestTranscriber(native *fakeStreamingTranscriber) *Transcriber {
	return &Transcriber{state: &transcriberState{nativeBackend: native}}
}
func TestTranscriber_StartStream(t *testing.T) {
	t.Run("creates one owned stream and rejects overlapping streams", func(t *testing.T) {
		native := &fakeStreamingTranscriber{stream: &fakeNativeStream{}}
		parent := newStreamingTestTranscriber(native)
		defer parent.Close()
		stream, err := parent.StartStream(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parent.StartStream(context.Background()); !errors.Is(err, speech.ErrTranscription) {
			t.Fatalf("got %v, want overlapping stream rejected", err)
		}
		if native.startCalls != 1 {
			t.Fatal("overlap entered native creation")
		}
		if err := stream.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := parent.StartStream(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("creation failure is safe and does not occupy parent", func(t *testing.T) {
		cause := errors.New("private native detail")
		native := &fakeStreamingTranscriber{start: func() error { return cause }, stream: &fakeNativeStream{}}
		parent := newStreamingTestTranscriber(native)
		defer parent.Close()
		if _, err := parent.StartStream(context.Background()); !errors.Is(err, cause) || err.Error() != speech.ErrTranscription.Error() {
			t.Fatalf("got %v", err)
		}
		native.start = nil
		if _, err := parent.StartStream(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("cancellation during creation releases new stream", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cause := errors.New("private close cause")
		native := &fakeStreamingTranscriber{stream: &fakeNativeStream{close: func() error { return cause }}, start: func() error { cancel(); return nil }}
		parent := newStreamingTestTranscriber(native)
		defer parent.Close()
		stream, err := parent.StartStream(ctx)
		if stream != nil || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) || err.Error() != context.Canceled.Error() || native.stream.closeCalls != 1 {
			t.Fatalf("got %v, %v, close calls %d", stream, err, native.stream.closeCalls)
		}
	})
}
func TestStream_AddAudio(t *testing.T) {
	t.Run("returns cumulative Go-owned text across calls and closure", func(t *testing.T) {
		native := &fakeStreamingTranscriber{stream: &fakeNativeStream{lines: []string{"hello", "", "world"}}}
		parent := newStreamingTestTranscriber(native)
		defer parent.Close()
		stream, err := parent.StartStream(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		first, err := stream.AddAudio(context.Background(), []float32{0}, 16000)
		if err != nil {
			t.Fatal(err)
		}
		native.stream.lines[0] = "later"
		if _, err := stream.AddAudio(context.Background(), []float32{0}, 16000); err != nil {
			t.Fatal(err)
		}
		native.stream.close = func() error { native.stream.lines[0] = "closed"; return nil }
		if err := stream.Close(); err != nil {
			t.Fatal(err)
		}
		if first.Text != "hello world" {
			t.Fatalf("got %q", first.Text)
		}
	})
	for _, operation := range []string{"add", "transcribe"} {
		t.Run(operation+" errors retain safe identity", func(t *testing.T) {
			cause := &NativeStatusError{Code: -3}
			native := &fakeStreamingTranscriber{stream: &fakeNativeStream{}}
			if operation == "add" {
				native.stream.add = func() error { return cause }
			} else {
				native.stream.transcribe = func() error { return cause }
			}
			parent := newStreamingTestTranscriber(native)
			defer parent.Close()
			stream, _ := parent.StartStream(context.Background())
			_, err := stream.AddAudio(context.Background(), []float32{0}, 16000)
			if !errors.Is(err, cause) || err.Error() != speech.ErrTranscription.Error() {
				t.Fatalf("got %v", err)
			}
			if operation == "add" && native.stream.transcriptCalls != 0 {
				t.Fatal("transcribed after failed audio addition")
			}
		})
	}
	for _, phase := range []string{"before", "add", "transcribe"} {
		t.Run("cancellation "+phase+" prevents publication", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			native := &fakeStreamingTranscriber{stream: &fakeNativeStream{lines: []string{"private text"}}}
			parent := newStreamingTestTranscriber(native)
			defer parent.Close()
			stream, _ := parent.StartStream(context.Background())
			switch phase {
			case "before":
				cancel()
			case "add":
				native.stream.add = func() error { cancel(); return nil }
			case "transcribe":
				native.stream.transcribe = func() error { cancel(); return nil }
			}
			result, err := stream.AddAudio(ctx, []float32{0}, 16000)
			if !errors.Is(err, context.Canceled) || result.Text != "" {
				t.Fatalf("got %+v, %v", result, err)
			}
			if phase != "transcribe" && native.stream.transcriptCalls != 0 {
				t.Fatal("canceled work requested transcript")
			}
		})
	}
	t.Run("rejects non UTF-8 native text", func(t *testing.T) {
		native := &fakeStreamingTranscriber{stream: &fakeNativeStream{lines: []string{string([]byte{255})}}}
		parent := newStreamingTestTranscriber(native)
		defer parent.Close()
		stream, _ := parent.StartStream(t.Context())
		transcript, err := stream.AddAudio(t.Context(), []float32{0}, 16000)
		if !errors.Is(err, speech.ErrTranscription) || transcript.Text != "" {
			t.Fatalf("got %+v, %v", transcript, err)
		}
	})

	t.Run("invalid PCM never reaches native stream", func(t *testing.T) {
		native := &fakeStreamingTranscriber{stream: &fakeNativeStream{}}
		parent := newStreamingTestTranscriber(native)
		defer parent.Close()
		stream, _ := parent.StartStream(context.Background())
		for _, samples := range [][]float32{nil, {2}, {float32(math.NaN())}} {
			if _, err := stream.AddAudio(context.Background(), samples, 16000); !errors.Is(err, speech.ErrInvalidAudio) {
				t.Fatalf("got %v", err)
			}
		}
		if native.stream.addCalls != 0 {
			t.Fatal("invalid PCM entered native stream")
		}
	})
}
func TestStream_Close(t *testing.T) {
	t.Run("copies and parent share cleanup outcome without final draining", func(t *testing.T) {
		cause := errors.New("private native close detail")
		native := &fakeStreamingTranscriber{stream: &fakeNativeStream{close: func() error { return cause }}}
		native.fakeNativeTranscriber.close = func() error {
			if native.stream.closeCalls != 1 {
				return errors.New("parent freed first")
			}
			return nil
		}
		parent := newStreamingTestTranscriber(native)
		stream, _ := parent.StartStream(context.Background())
		copied := *stream
		parentErr := parent.Close()
		if !errors.Is(parentErr, cause) || strings.Contains(parentErr.Error(), "private") {
			t.Fatalf("got %v", parentErr)
		}
		firstErr := stream.Close()
		if !errors.Is(firstErr, cause) || copied.Close() != firstErr || native.stream.closeCalls != 1 || native.stream.transcriptCalls != 0 {
			t.Fatal("stream cleanup was not shared or final drain occurred")
		}
		for _, closed := range []*Stream{stream, &copied} {
			if _, err := closed.AddAudio(context.Background(), []float32{0}, 16000); !errors.Is(err, speech.ErrClosed) {
				t.Fatalf("got %v", err)
			}
		}
		if _, err := parent.StartStream(context.Background()); !errors.Is(err, speech.ErrClosed) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestStream_ConcurrentLifecycle(t *testing.T) {
	t.Run("waiting for native runtime remains cancellable", func(t *testing.T) {
		native := &fakeStreamingTranscriber{stream: &fakeNativeStream{}}
		parent := newStreamingTestTranscriber(native)
		stream, err := parent.StartStream(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		moonshineRuntimeGate <- struct{}{}
		ctx, cancel := context.WithCancel(t.Context())
		waitingCtx := &gateWaitingContext{Context: ctx, waiting: make(chan struct{})}
		returned := make(chan error, 1)
		go func() { _, err := stream.AddAudio(waitingCtx, []float32{0}, 16000); returned <- err }()
		<-waitingCtx.waiting
		cancel()
		err = <-returned
		releaseMoonshineRuntime()
		if !errors.Is(err, context.Canceled) || native.stream.addCalls != 0 {
			t.Fatalf("got %v, add calls %d", err, native.stream.addCalls)
		}
		if err := parent.Close(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("parent Close joins inference before stream and model destruction", func(t *testing.T) {
		entered := make(chan struct{})
		finish := make(chan struct{})
		inferenceReturned := make(chan struct{})
		native := &fakeStreamingTranscriber{stream: &fakeNativeStream{}}
		native.stream.transcribe = func() error { close(entered); <-finish; close(inferenceReturned); return nil }
		native.stream.close = func() error {
			select {
			case <-inferenceReturned:
				return nil
			default:
				return errors.New("freed during inference")
			}
		}
		parent := newStreamingTestTranscriber(native)
		stream, _ := parent.StartStream(t.Context())
		inferenceDone := make(chan error, 1)
		closeDone := make(chan error, 1)
		go func() { _, err := stream.AddAudio(t.Context(), []float32{0}, 16000); inferenceDone <- err }()
		<-entered
		go func() { closeDone <- parent.Close() }()
		close(finish)
		if err := <-inferenceDone; err != nil {
			t.Fatal(err)
		}
		if err := <-closeDone; err != nil {
			t.Fatal(err)
		}
		if native.stream.closeCalls != 1 || native.closeCalls.Load() != 1 {
			t.Fatal("native ownership not released once")
		}
	})
}
