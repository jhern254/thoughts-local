package main

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jhern254/go-thoughts/internal/speech"
	"github.com/jhern254/go-thoughts/internal/voice"
)

type testMoonshineStream struct {
	add        func(context.Context, []float32, int) (speech.Transcript, error)
	closeCalls int
}

func (stream *testMoonshineStream) AddAudio(ctx context.Context, samples []float32, rate int) (speech.Transcript, error) {
	return stream.add(ctx, samples, rate)
}
func (stream *testMoonshineStream) Close() error { stream.closeCalls++; return nil }
func TestBrowserSpeechConsumer_ConsumePCM(t *testing.T) {
	t.Run("normalizes one borrowed chunk and publishes only changed cumulative text", func(t *testing.T) {
		var normalized []float32
		texts := []string{"", "hello", "hello", "hello world"}
		callIndex := 0
		nativeStream := &testMoonshineStream{add: func(_ context.Context, samples []float32, rate int) (speech.Transcript, error) {
			if rate != 16000 {
				t.Fatalf("got rate %d", rate)
			}
			normalized = append([]float32(nil), samples...)
			transcript := speech.Transcript{Text: texts[callIndex]}
			callIndex++
			return transcript, nil
		}}
		recordingKey := voice.RecordingKey{SessionID: "session", DraftID: 1, RecordingID: 2}
		var updates []voice.TranscriptUpdate
		consumer := &browserSpeechConsumer{moonshineStream: nativeStream, recording: recordingKey, publish: func(update voice.TranscriptUpdate) bool { updates = append(updates, update); return true }}
		pcmSamples := make([]int16, 1600)
		copy(pcmSamples, []int16{-32768, -16384, 0, 16384, 32767})
		for range texts {
			if err := consumer.ConsumePCM(t.Context(), pcmSamples); err != nil {
				t.Fatal(err)
			}
		}
		expected := []float32{-1, -0.5, 0, float32(16384.0 / 32767), 1}
		if !reflect.DeepEqual(normalized[:5], expected) {
			t.Fatalf("got %v, want %v", normalized[:5], expected)
		}
		wantUpdates := []voice.TranscriptUpdate{{Recording: recordingKey, Revision: 1, Text: "hello"}, {Recording: recordingKey, Revision: 2, Text: "hello world"}}
		if !reflect.DeepEqual(updates, wantUpdates) {
			t.Fatalf("got %+v, want %+v", updates, wantUpdates)
		}
		for _, sample := range consumer.normalizedSamples {
			if sample != 0 {
				t.Fatal("consumer retained PCM after native call")
			}
		}
		if err := consumer.Close(); err != nil {
			t.Fatal(err)
		}
		if nativeStream.closeCalls != 1 || len(updates) != 2 {
			t.Fatal("Close drained or published transcript")
		}
	})
	t.Run("cancellation during inference discards returned text", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		nativeStream := &testMoonshineStream{add: func(context.Context, []float32, int) (speech.Transcript, error) {
			cancel()
			return speech.Transcript{Text: "late"}, nil
		}}
		consumer := &browserSpeechConsumer{moonshineStream: nativeStream, publish: func(voice.TranscriptUpdate) bool { t.Fatal("published after cancellation"); return true }}
		if err := consumer.ConsumePCM(ctx, make([]int16, 1600)); !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v", err)
		}
	})
	for _, text := range []string{string([]byte{255}), strings.Repeat("x", voice.MaximumTranscriptBytes+1)} {
		t.Run("rejects invalid or overlong text safely", func(t *testing.T) {
			nativeStream := &testMoonshineStream{add: func(context.Context, []float32, int) (speech.Transcript, error) {
				return speech.Transcript{Text: text}, nil
			}}
			consumer := &browserSpeechConsumer{moonshineStream: nativeStream, publish: func(voice.TranscriptUpdate) bool { t.Fatal("published invalid text"); return true }}
			if err := consumer.ConsumePCM(t.Context(), make([]int16, 1600)); !errors.Is(err, speech.ErrTranscription) {
				t.Fatalf("got %v", err)
			}
		})
	}
	t.Run("publisher rejection ends processing", func(t *testing.T) {
		nativeStream := &testMoonshineStream{add: func(context.Context, []float32, int) (speech.Transcript, error) {
			return speech.Transcript{Text: "current text"}, nil
		}}
		consumer := &browserSpeechConsumer{moonshineStream: nativeStream, publish: func(voice.TranscriptUpdate) bool { return false }}
		if err := consumer.ConsumePCM(t.Context(), make([]int16, 1600)); !errors.Is(err, speech.ErrTranscription) {
			t.Fatalf("got %v", err)
		}
		if consumer.lastPublishedTranscript != "" {
			t.Fatal("remembered rejected publication")
		}
	})
	t.Run("cancellation before inference prevents native work", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		consumer := &browserSpeechConsumer{moonshineStream: &testMoonshineStream{add: func(context.Context, []float32, int) (speech.Transcript, error) {
			t.Fatal("entered canceled inference")
			return speech.Transcript{}, nil
		}}}
		if err := consumer.ConsumePCM(ctx, make([]int16, 1600)); !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v", err)
		}
	})
}
func TestOpenBrowserSpeech(t *testing.T) {
	t.Run("unconfigured speech returns no consumer factory", func(t *testing.T) {
		factory, closeSpeech, err := openBrowserSpeech(t.Context(), "")
		if err != nil || factory != nil {
			t.Fatalf("got %v, factory present=%v", err, factory != nil)
		}
		if err := closeSpeech(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("explicit invalid setup returns safe startup failure", func(t *testing.T) {
		for _, directory := range []string{t.TempDir() + "/private-missing", t.TempDir()} {
			factory, _, err := openBrowserSpeech(t.Context(), directory)
			if factory != nil || !errors.Is(err, errBrowserSpeechSetup) || err.Error() != errBrowserSpeechSetup.Error() {
				t.Fatalf("got %v", err)
			}
		}
	})
}
