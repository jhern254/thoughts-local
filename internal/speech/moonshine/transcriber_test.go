package moonshine

import (
	"context"
	"errors"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/modelassets"
	"github.com/jhern254/go-thoughts/internal/speech"
)

type fakeNativeTranscriber struct {
	transcriptLines    []string
	transcribe         func() error
	close              func() error
	transcriptionCalls atomic.Int32
	closeCalls         atomic.Int32
}

func (native *fakeNativeTranscriber) Transcribe(_ []float32, _ int) ([]string, error) {
	native.transcriptionCalls.Add(1)
	if native.transcribe != nil {
		if err := native.transcribe(); err != nil {
			return nil, err
		}
	}
	return native.transcriptLines, nil
}
func (native *fakeNativeTranscriber) Close() error {
	native.closeCalls.Add(1)
	if native.close != nil {
		return native.close()
	}
	return nil
}
func TestTranscriber_Transcribe(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		samples      []float32
		sampleRateHz int
	}{
		{"zero sample rate", []float32{0}, 0},
		{"negative sample rate", []float32{0}, -1},
		{"empty audio", nil, 16000},
		{"NaN", []float32{float32(math.NaN())}, 16000},
		{"positive infinity", []float32{float32(math.Inf(1))}, 16000},
		{"negative infinity", []float32{float32(math.Inf(-1))}, 16000},
		{"above PCM range", []float32{1.01}, 16000},
		{"below PCM range", []float32{-1.01}, 16000},
	} {
		t.Run("rejects "+testCase.name+" before native work", func(t *testing.T) {
			native := &fakeNativeTranscriber{}
			transcriber := &Transcriber{state: &transcriberState{nativeBackend: native}}
			_, err := transcriber.Transcribe(context.Background(), testCase.samples, testCase.sampleRateHz)
			if !errors.Is(err, speech.ErrInvalidAudio) {
				t.Fatalf("got %v, want invalid audio", err)
			}
			if native.transcriptionCalls.Load() != 0 {
				t.Fatal("invalid PCM entered native inference")
			}
		})
	}
	t.Run("joins Go-owned lines and preserves results after later inference and closure", func(t *testing.T) {
		native := &fakeNativeTranscriber{transcriptLines: []string{"hello", "", "world"}}
		native.close = func() error {
			native.transcriptLines[0] = "closed result"
			return nil
		}
		transcriber := &Transcriber{state: &transcriberState{nativeBackend: native}}
		transcript, err := transcriber.Transcribe(context.Background(), []float32{-1, 0, 1}, 44100)
		if err != nil {
			t.Fatal(err)
		}
		native.transcriptLines[0] = "later result"
		if _, err := transcriber.Transcribe(context.Background(), []float32{0}, 16000); err != nil {
			t.Fatal(err)
		}
		if err := transcriber.Close(); err != nil {
			t.Fatal(err)
		}
		if transcript.Text != "hello world" {
			t.Fatalf("text got %q, want hello world", transcript.Text)
		}
	})
	t.Run("returns an empty transcript for no speech", func(t *testing.T) {
		transcriber := &Transcriber{state: &transcriberState{nativeBackend: &fakeNativeTranscriber{}}}
		transcript, err := transcriber.Transcribe(context.Background(), []float32{0}, 16000)
		if err != nil || transcript.Text != "" {
			t.Fatalf("got %+v, %v, want empty transcript", transcript, err)
		}
	})
	t.Run("retains typed native status while exposing only safe error text", func(t *testing.T) {
		nativeCause := &NativeStatusError{Code: -3}
		transcriber := &Transcriber{state: &transcriberState{nativeBackend: &fakeNativeTranscriber{transcribe: func() error { return nativeCause }}}}
		_, err := transcriber.Transcribe(context.Background(), []float32{0}, 16000)
		var nativeStatus *NativeStatusError
		if !errors.Is(err, speech.ErrTranscription) || !errors.As(err, &nativeStatus) || nativeStatus.Code != -3 {
			t.Fatalf("got %v, want transcription with status -3", err)
		}
		if err.Error() != speech.ErrTranscription.Error() {
			t.Fatalf("unsafe public error: %v", err)
		}
	})
}
func TestTranscriber_Cancellation(t *testing.T) {
	t.Run("cancellation before inference prevents native work", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		native := &fakeNativeTranscriber{}
		transcriber := &Transcriber{state: &transcriberState{nativeBackend: native}}
		_, err := transcriber.Transcribe(ctx, []float32{0}, 16000)
		if !errors.Is(err, context.Canceled) || native.transcriptionCalls.Load() != 0 {
			t.Fatalf("got %v and %d native calls", err, native.transcriptionCalls.Load())
		}
	})
	t.Run("deadline before inference prevents native work", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Time{})
		defer cancel()
		native := &fakeNativeTranscriber{}
		transcriber := &Transcriber{state: &transcriberState{nativeBackend: native}}
		_, err := transcriber.Transcribe(ctx, []float32{0}, 16000)
		if !errors.Is(err, context.DeadlineExceeded) || native.transcriptionCalls.Load() != 0 {
			t.Fatalf("got %v and %d native calls", err, native.transcriptionCalls.Load())
		}
	})
	t.Run("cancellation after native inference discards transcript", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		native := &fakeNativeTranscriber{
			transcriptLines: []string{"private text"},
			transcribe: func() error {
				cancel()
				return nil
			},
		}
		transcriber := &Transcriber{state: &transcriberState{nativeBackend: native}}
		transcript, err := transcriber.Transcribe(ctx, []float32{0}, 16000)
		if !errors.Is(err, context.Canceled) || transcript.Text != "" {
			t.Fatalf("got %+v, %v, want canceled without transcript", transcript, err)
		}
	})
	t.Run("waiting for another native call is cancellable and close waits for inference", func(t *testing.T) {
		entered := make(chan struct{})
		finish := make(chan struct{})
		inferenceFinished := make(chan error, 1)
		inferenceJoined := make(chan struct{})
		var finishOnce sync.Once
		finishInference := func() { finishOnce.Do(func() { close(finish) }) }
		var inferenceReturned atomic.Bool
		native := &fakeNativeTranscriber{
			transcribe: func() error { close(entered); <-finish; inferenceReturned.Store(true); return nil },
			close: func() error {
				if !inferenceReturned.Load() {
					return errors.New("freed during inference")
				}
				return nil
			},
		}
		transcriber := &Transcriber{state: &transcriberState{nativeBackend: native}}
		go func() {
			defer close(inferenceJoined)
			_, err := transcriber.Transcribe(context.Background(), []float32{0}, 16000)
			inferenceFinished <- err
		}()
		defer func() { finishInference(); <-inferenceJoined }()
		<-entered
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		waitingContext := &gateWaitingContext{Context: ctx, waiting: make(chan struct{})}
		waitingNative := &fakeNativeTranscriber{}
		waitingTranscriber := &Transcriber{state: &transcriberState{nativeBackend: waitingNative}}
		waitingFinished := make(chan error, 1)
		go func() {
			_, err := waitingTranscriber.Transcribe(waitingContext, []float32{0}, 16000)
			waitingFinished <- err
		}()
		<-waitingContext.waiting
		cancel()
		if err := <-waitingFinished; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		if waitingNative.transcriptionCalls.Load() != 0 {
			t.Fatal("concurrent native call entered while another held the gate")
		}
		closeFinished := make(chan error, 1)
		closeJoined := make(chan struct{})
		go func() { defer close(closeJoined); closeFinished <- transcriber.Close() }()
		defer func() { finishInference(); <-closeJoined }()
		if native.closeCalls.Load() != 0 {
			t.Fatal("native transcriber freed during inference")
		}
		finishInference()
		if err := <-inferenceFinished; err != nil {
			t.Fatal(err)
		}
		if err := <-closeFinished; err != nil {
			t.Fatal(err)
		}
		if native.closeCalls.Load() != 1 {
			t.Fatalf("close calls got %d, want 1", native.closeCalls.Load())
		}
	})
}
func TestTranscriber_Close(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		closeCause error
	}{
		{name: "successful cleanup"},
		{name: "failed cleanup", closeCause: errors.New("private native cleanup detail")},
	} {
		t.Run("copies share lifecycle state after "+testCase.name, func(t *testing.T) {
			native := &fakeNativeTranscriber{close: func() error { return testCase.closeCause }}
			original := &Transcriber{state: &transcriberState{nativeBackend: native}}
			copied := *original
			firstCloseErr := original.Close()
			for _, transcriber := range []*Transcriber{original, &copied} {
				for range 2 {
					if err := transcriber.Close(); err != firstCloseErr {
						t.Fatalf("close got %v, want stored outcome %v", err, firstCloseErr)
					}
				}
				if _, err := transcriber.Transcribe(context.Background(), []float32{0}, 16000); !errors.Is(err, speech.ErrClosed) {
					t.Fatalf("transcription got %v, want closed", err)
				}
			}
			if got := native.closeCalls.Load(); got != 1 {
				t.Fatalf("native close calls got %d, want 1", got)
			}
			if testCase.closeCause != nil {
				if !errors.Is(firstCloseErr, testCase.closeCause) || !errors.Is(firstCloseErr, speech.ErrRuntime) || firstCloseErr.Error() != speech.ErrRuntime.Error() {
					t.Fatalf("got %v, want safe retained cleanup failure", firstCloseErr)
				}
			}
		})
	}

	t.Run("releases resources exactly once and rejects use after close", func(t *testing.T) {
		native := &fakeNativeTranscriber{}
		transcriber := &Transcriber{state: &transcriberState{nativeBackend: native}}
		for range 2 {
			if err := transcriber.Close(); err != nil {
				t.Fatal(err)
			}
		}
		if native.closeCalls.Load() != 1 {
			t.Fatalf("close calls got %d, want 1", native.closeCalls.Load())
		}
		if _, err := transcriber.Transcribe(context.Background(), []float32{0}, 16000); !errors.Is(err, speech.ErrClosed) {
			t.Fatalf("got %v, want closed", err)
		}
	})
	t.Run("preserves cleanup failures safely on repeated close", func(t *testing.T) {
		cause := errors.New("private local path")
		native := &fakeNativeTranscriber{close: func() error { return cause }}
		transcriber := &Transcriber{state: &transcriberState{nativeBackend: native}}
		for range 2 {
			err := transcriber.Close()
			if !errors.Is(err, speech.ErrRuntime) || !errors.Is(err, cause) || strings.Contains(err.Error(), "private") {
				t.Fatalf("got %v, want safe retained cleanup failure", err)
			}
		}
		if native.closeCalls.Load() != 1 {
			t.Fatal("cleanup was repeated")
		}
	})
}

func TestTranscriber_Open(t *testing.T) {
	t.Run("missing approved assets fail without inference or download", func(t *testing.T) {
		modelRoot, err := os.OpenRoot(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer modelRoot.Close()
		if _, err := Open(context.Background(), modelRoot); !errors.Is(err, speech.ErrModelLoad) || !errors.Is(err, modelassets.ErrNotInstalled) {
			t.Fatalf("got %v, want missing model", err)
		}
	})
	t.Run("canceled loading frees the created handle and preserves close failure safely", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		closeCause := errors.New("private model location")
		native := &fakeNativeTranscriber{close: func() error { return closeCause }}
		loadNative := func(context.Context, *os.Root, modelassets.Installation) (nativeTranscriber, error) {
			cancel()
			return native, nil
		}
		transcriber, err := openVerifiedTranscriber(ctx, nil, modelassets.Installation{}, loadNative)
		if transcriber != nil || !errors.Is(err, context.Canceled) || !errors.Is(err, closeCause) || !errors.Is(err, speech.ErrRuntime) {
			t.Fatalf("got %v, %v, want canceled load with close failure", transcriber, err)
		}
		if err.Error() != context.Canceled.Error() {
			t.Fatalf("unsafe lifecycle error: %v", err)
		}
		if native.closeCalls.Load() != 1 {
			t.Fatal("canceled load did not release its native handle exactly once")
		}
	})
	t.Run("native runtime causes never escape the safe category", func(t *testing.T) {
		loadNative := func(context.Context, *os.Root, modelassets.Installation) (nativeTranscriber, error) {
			return nil, errors.Join(speech.ErrRuntime, errors.New("private path"))
		}
		_, err := openVerifiedTranscriber(context.Background(), nil, modelassets.Installation{}, loadNative)
		if !errors.Is(err, speech.ErrRuntime) || err.Error() != speech.ErrRuntime.Error() {
			t.Fatalf("unsafe runtime error: %v", err)
		}
	})
}

type gateWaitingContext struct {
	context.Context
	waiting  chan struct{}
	notified sync.Once
}

func (ctx *gateWaitingContext) Done() <-chan struct{} {
	ctx.notified.Do(func() { close(ctx.waiting) })
	return ctx.Context.Done()
}

func TestTranscriber_NativeInputLimits(t *testing.T) {
	t.Run("rejects sample rates that cannot fit the native type", func(t *testing.T) {
		if strconv.IntSize < 64 {
			t.Skip("platform int cannot represent a sample rate above the native int32 limit")
		}
		oversizedRate := int64(math.MaxInt32) + 1
		native := &fakeNativeTranscriber{}
		transcriber := &Transcriber{state: &transcriberState{nativeBackend: native}}
		_, err := transcriber.Transcribe(context.Background(), []float32{0}, int(oversizedRate))
		if !errors.Is(err, speech.ErrInvalidAudio) || native.transcriptionCalls.Load() != 0 {
			t.Fatalf("got %v and %d native calls", err, native.transcriptionCalls.Load())
		}
	})
}
