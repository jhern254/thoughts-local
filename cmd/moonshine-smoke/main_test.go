package main

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"testing"

	"github.com/jhern254/go-thoughts/internal/speech"
)

func TestDecodePCM(t *testing.T) {
	t.Run("decodes float32 little endian without changing samples", func(t *testing.T) {
		var encoded [12]byte
		for sampleIndex, sample := range []float32{-1, 0, 1} {
			binary.LittleEndian.PutUint32(encoded[sampleIndex*4:], math.Float32bits(sample))
		}
		samples, err := decodePCM(encoded[:])
		if err != nil {
			t.Fatal(err)
		}
		for sampleIndex, expected := range []float32{-1, 0, 1} {
			if samples[sampleIndex] != expected {
				t.Fatalf("sample %d got %f, want %f", sampleIndex, samples[sampleIndex], expected)
			}
		}
	})
	for _, encoded := range [][]byte{nil, {1, 2, 3}} {
		t.Run("rejects empty or partial samples", func(t *testing.T) {
			if _, err := decodePCM(encoded); !errors.Is(err, speech.ErrInvalidAudio) {
				t.Fatalf("got %v, want invalid audio", err)
			}
		})
	}
}

func TestRun(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		arguments []string
		wantError string
	}{
		{name: "missing command", wantError: "usage: moonshine-smoke install|transcribe -models-dir PATH [-pcm FILE -sample-rate 16000]"},
		{name: "unknown command", arguments: []string{"unknown"}, wantError: "usage: moonshine-smoke install|transcribe -models-dir PATH [-pcm FILE -sample-rate 16000]"},
		{name: "invalid flags", arguments: []string{"transcribe", "-unknown"}, wantError: "invalid smoke command arguments"},
		{name: "missing root", arguments: []string{"install", "-models-dir", ""}, wantError: "model root is required"},
		{name: "audio on install", arguments: []string{"install", "-models-dir", t.TempDir(), "-pcm", "fixture"}, wantError: "install does not accept audio"},
		{name: "missing PCM", arguments: []string{"transcribe", "-models-dir", t.TempDir()}, wantError: "transcribe requires an explicit PCM file"},
	} {
		t.Run("rejects "+testCase.name, func(t *testing.T) {
			err := run(context.Background(), testCase.arguments, io.Discard, io.Discard)
			if err == nil || err.Error() != testCase.wantError {
				t.Fatalf("got %v, want %s", err, testCase.wantError)
			}
		})
	}
}
