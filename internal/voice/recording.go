// Package voice defines recording identity and draft authority, independently
// of audio transport or any recognizer. It owns no PCM or persistence.
package voice

import "sync"

const MaximumTranscriptBytes = 16 << 10

type RecordingKey struct {
	SessionID   string
	DraftID     uint64
	RecordingID uint64
}

type TranscriptUpdate struct {
	Recording RecordingKey
	Revision  uint64
	Text      string
}

// RecordingEnded is sent only after recording-owned work has joined. Browser
// acknowledgements cannot stand in for server-side resource cleanup.
type RecordingEnded struct {
	Recording RecordingKey
	Failed    bool
}

// TranscriptAuthority shares the revocation boundary between the draft and its
// transport. Cancellation alone cannot invalidate a result already queued.
type TranscriptAuthority struct {
	mu               sync.Mutex
	recording        RecordingKey
	accepting        bool
	acceptedRevision uint64
}

func NewTranscriptAuthority(recording RecordingKey) *TranscriptAuthority {
	return &TranscriptAuthority{recording: recording, accepting: true}
}

func (authority *TranscriptAuthority) Recording() RecordingKey { return authority.recording }

func (authority *TranscriptAuthority) Active() bool {
	authority.mu.Lock()
	defer authority.mu.Unlock()
	return authority.accepting
}

func (authority *TranscriptAuthority) Revoke() {
	authority.mu.Lock()
	defer authority.mu.Unlock()
	authority.accepting = false
}

// applyToDraft must only perform the synchronous editor mutation. Keeping it
// inside the authority lock makes Stop and draft mutation mutually exclusive;
// it must not block on transport, commands, or resource cleanup.
func (authority *TranscriptAuthority) ApplyTranscriptUpdate(update TranscriptUpdate, applyToDraft func(string) bool) bool {
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if !authority.accepting || update.Recording != authority.recording || update.Revision <= authority.acceptedRevision || len(update.Text) > MaximumTranscriptBytes {
		return false
	}
	if !applyToDraft(update.Text) {
		return false
	}
	authority.acceptedRevision = update.Revision
	return true
}
