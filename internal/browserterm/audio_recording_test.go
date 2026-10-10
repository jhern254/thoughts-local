package browserterm

import (
	"context"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
	"github.com/jhern254/go-thoughts/internal/voice"
	"testing"
)

func TestAudioRecording_Lifecycle(t *testing.T) {
	t.Run("overflow revokes authority clears PCM and joins the consumer", func(t *testing.T) {
		entered := make(chan struct{})
		var borrowed []int16
		session := newAudioSession(t.Context(), func(ctx context.Context, _ voice.RecordingKey, samples []int16, _ func(voice.TranscriptUpdate) bool) error {
			borrowed = samples
			close(entered)
			<-ctx.Done()
			return ctx.Err()
		})
		authority := voice.NewTranscriptAuthority(voice.RecordingKey{SessionID: session.sessionID, DraftID: 1, RecordingID: 1})
		recording := newAudioRecording(session, authority)
		handshake := recording.recordingCapability
		if status := recording.authorizeAudioConnection(handshake, nil); status != audioStatusReady {
			t.Fatalf("attach got %d", status)
		}
		first := make([]int16, pcmFrameSamples)
		first[0] = 12345
		if status := recording.enqueuePCMChunk(first); status != audioStatusReady {
			t.Fatalf("enqueue got %d", status)
		}
		<-entered
		queued := make([][]int16, 0)
		for frameIndex := 0; frameIndex < maximumPendingAudioChunks; frameIndex++ {
			samples := make([]int16, pcmFrameSamples)
			samples[0] = 12345
			queued = append(queued, samples)
			if status := recording.enqueuePCMChunk(samples); status != audioStatusReady {
				t.Fatalf("enqueue got %d", status)
			}
		}
		overflow := make([]int16, pcmFrameSamples)
		overflow[0] = 12345
		if status := recording.enqueuePCMChunk(overflow); status != audioStatusLimit {
			t.Fatalf("overflow got %d, want limit", status)
		}
		recording.stopRecording(audioStatusLimit)
		<-recording.done
		if authority.Active() {
			t.Fatal("overflow retained transcript authority")
		}
		for _, samples := range append(queued, borrowed, overflow) {
			for _, sample := range samples {
				if sample != 0 {
					t.Fatal("PCM retained after cleanup")
				}
			}
		}
	})
	t.Run("Stop prevents late transcript publication", func(t *testing.T) {
		session := newAudioSession(t.Context(), discardPCM)
		authority := voice.NewTranscriptAuthority(voice.RecordingKey{SessionID: session.sessionID, DraftID: 1, RecordingID: 1})
		recording := newAudioRecording(session, authority)
		recording.stopRecording(audioStatusStopped)
		if recording.publishTranscriptUpdate(voice.TranscriptUpdate{Recording: authority.Recording(), Revision: 1, Text: "late"}) {
			t.Fatal("published after Stop")
		}
		<-recording.done
	})
}

func TestAudioSession_DraftReplacement(t *testing.T) {
	t.Run("rejects a new draft recording safely while the previous consumer joins", func(t *testing.T) {
		entered := make(chan struct{})
		canceled := make(chan struct{})
		release := make(chan struct{})
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		session := newAudioSession(ctx, func(ctx context.Context, _ voice.RecordingKey, _ []int16, _ func(voice.TranscriptUpdate) bool) error {
			close(entered)
			<-ctx.Done()
			close(canceled)
			<-release
			return ctx.Err()
		})
		previousAuthority := voice.NewTranscriptAuthority(voice.RecordingKey{SessionID: session.sessionID, DraftID: 1, RecordingID: 1})
		session.synchronizeRecording(thoughts.VoiceState{TranscriptAuthority: previousAuthority, RecordingStatus: "requesting"})
		previous := session.activeRecording
		previous.authorizeAudioConnection(previous.recordingCapability, nil)
		previous.enqueuePCMChunk(make([]int16, pcmFrameSamples))
		<-entered
		currentAuthority := voice.NewTranscriptAuthority(voice.RecordingKey{SessionID: session.sessionID, DraftID: 2, RecordingID: 1})
		capability, rejected := session.synchronizeRecording(thoughts.VoiceState{TranscriptAuthority: currentAuthority, RecordingStatus: "requesting"})
		<-canceled
		if capability != "" || rejected == nil || rejected.Recording != currentAuthority.Recording() || !rejected.Failed {
			t.Error("busy replacement did not return its own completion result")
		}
		if currentAuthority.Active() || previousAuthority.Active() {
			t.Error("replacement retained transcript authority")
		}
		close(release)
		<-previous.done
		cancel()
		session.close()
	})
}
