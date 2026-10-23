// Package speech defines the small, portable result and error boundary for local
// speech recognition. It has no audio transport, persistence or runtime wiring.
package speech

import (
	"errors"
	"time"
)

type Transcript struct {
	Text     string
	Segments []Segment
}

// Segment timing is relative to recording start, not publication time.
// Completed native phrases have immutable text and timing. Nonpositive duration
// or a negative start means timing is unavailable; Text remains usable.
type Segment struct {
	ID       uint64
	Text     string
	Start    time.Duration
	Duration time.Duration
	Complete bool
}

var (
	ErrRuntime       = errors.New("speech runtime failure")
	ErrInvalidAudio  = errors.New("invalid speech audio")
	ErrModelLoad     = errors.New("speech model load failed")
	ErrTranscription = errors.New("speech transcription failed")
	ErrClosed        = errors.New("speech transcriber closed")
)
