// Package speech defines the small, portable result and error boundary for local
// speech recognition. It has no audio transport, persistence or runtime wiring.
package speech

import "errors"

type Transcript struct {
	Text string
}

var (
	ErrRuntime       = errors.New("speech runtime failure")
	ErrInvalidAudio  = errors.New("invalid speech audio")
	ErrModelLoad     = errors.New("speech model load failed")
	ErrTranscription = errors.New("speech transcription failed")
	ErrClosed        = errors.New("speech transcriber closed")
)
