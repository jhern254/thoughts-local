package main

import (
	"encoding/binary"
	"errors"
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
