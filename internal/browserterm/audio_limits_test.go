package browserterm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/voice"
)

func TestAudioRecording_Bounds(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		configure func(*audioSettings)
		advance   time.Duration
	}{
		{name: "aggregate samples", configure: func(settings *audioSettings) { settings.maximumSamples = pcmFrameSamples }},
		{name: "elapsed duration", advance: maximumRecordingDuration},
	} {
		t.Run("rejects "+scenario.name, func(t *testing.T) {
			consumed := make(chan struct{}, 1)
			session := newAudioSession(t.Context(), func(context.Context, voice.RecordingKey, []int16, func(voice.TranscriptUpdate) bool) error {
				consumed <- struct{}{}
				return nil
			})
			now := time.Now()
			session.settings.now = func() time.Time { return now }
			if scenario.configure != nil {
				scenario.configure(&session.settings)
			}
			authority := voice.NewTranscriptAuthority(voice.RecordingKey{SessionID: session.sessionID, DraftID: 1, RecordingID: 1})
			recording := newAudioRecording(session, authority)
			t.Cleanup(func() { recording.stopRecording(audioStatusStopped); <-recording.done })
			if got := recording.authorizeAudioConnection(recording.recordingCapability, nil); got != audioStatusReady {
				t.Fatal("attach failed")
			}
			if got := recording.enqueuePCMChunk(make([]int16, pcmFrameSamples)); got != audioStatusReady {
				t.Fatal("initial frame rejected")
			}
			<-consumed
			now = now.Add(scenario.advance)
			rejected := make([]int16, pcmFrameSamples)
			rejected[0] = 123
			if got := recording.enqueuePCMChunk(rejected); got != audioStatusLimit {
				t.Fatalf("got %d, want Limit", got)
			}
			recording.stopRecording(audioStatusLimit)
			<-recording.done
			if authority.Active() || len(recording.pendingAudio) != 0 {
				t.Fatal("limit retained recording authority or audio")
			}
			for _, sample := range rejected {
				if sample != 0 {
					t.Fatal("rejected PCM retained")
				}
			}
		})
	}
}

func TestAudioRecording_AuthorizationAndDeadlines(t *testing.T) {
	t.Run("capabilities are bound one use revoked and expiring", func(t *testing.T) {
		session := newAudioSession(t.Context(), discardPCM)
		now := time.Now()
		session.settings.now = func() time.Time { return now }
		authority := voice.NewTranscriptAuthority(voice.RecordingKey{SessionID: session.sessionID, DraftID: 1, RecordingID: 1})
		recording := newAudioRecording(session, authority)
		t.Cleanup(func() { recording.stopRecording(audioStatusStopped); <-recording.done })
		handshake := recording.recordingCapability
		wrong := handshake
		wrong[0] ^= 1
		if recording.authorizeAudioConnection(wrong, nil) != audioStatusDenied || !authority.Active() {
			t.Fatal("unknown token granted access or disrupted recording")
		}
		now = now.Add(audioCapabilityLifetime)
		if recording.authorizeAudioConnection(handshake, nil) != audioStatusDenied {
			t.Fatal("expired token accepted")
		}
		now = now.Add(-audioCapabilityLifetime)
		if recording.authorizeAudioConnection(handshake, nil) != audioStatusReady {
			t.Fatal("valid token rejected")
		}
		if recording.authorizeAudioConnection(handshake, nil) != audioStatusBusy {
			t.Fatal("duplicate audio connection accepted")
		}
		recording.stopRecording(audioStatusStopped)
		if recording.authorizeAudioConnection(handshake, nil) != audioStatusDenied {
			t.Fatal("revoked token accepted")
		}
	})
	for _, deadline := range []time.Duration{maximumRecordingDuration, audioCapabilityLifetime} {
		t.Run("deadline revokes authority without input "+deadline.String(), func(t *testing.T) {
			session := newAudioSession(t.Context(), discardPCM)
			registered := make(chan struct {
				duration time.Duration
				cancel   context.CancelFunc
			}, 2)
			session.settings.withTimeout = func(parent context.Context, duration time.Duration) (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(parent)
				registered <- struct {
					duration time.Duration
					cancel   context.CancelFunc
				}{duration, cancel}
				return ctx, cancel
			}
			authority := voice.NewTranscriptAuthority(voice.RecordingKey{SessionID: session.sessionID, DraftID: 1, RecordingID: 1})
			recording := newAudioRecording(session, authority)
			for range 2 {
				timeout := <-registered
				if timeout.duration == deadline {
					timeout.cancel()
				}
			}
			<-recording.done
			if authority.Active() {
				t.Fatal("deadline retained authority")
			}
		})
	}
}

func TestAudioRecording_ConsumerFailure(t *testing.T) {
	for _, consumer := range []struct {
		name       string
		consumePCM PCMConsumer
	}{
		{"error", func(context.Context, voice.RecordingKey, []int16, func(voice.TranscriptUpdate) bool) error {
			return errors.New("PRIVATE-ERROR")
		}},
		{"panic", func(context.Context, voice.RecordingKey, []int16, func(voice.TranscriptUpdate) bool) error {
			panic("PRIVATE-PCM")
		}},
	} {
		t.Run("contains "+consumer.name+" and clears audio", func(t *testing.T) {
			session := newAudioSession(t.Context(), consumer.consumePCM)
			authority := voice.NewTranscriptAuthority(voice.RecordingKey{SessionID: session.sessionID, DraftID: 1, RecordingID: 1})
			recording := newAudioRecording(session, authority)
			recording.authorizeAudioConnection(recording.recordingCapability, nil)
			pcmSamples := make([]int16, pcmFrameSamples)
			pcmSamples[0] = 1234
			recording.enqueuePCMChunk(pcmSamples)
			<-recording.done
			if authority.Active() || pcmSamples[0] != 0 {
				t.Fatal("consumer failure retained audio or authority")
			}
			if ended := <-session.ended; !ended.Failed {
				t.Fatal("consumer failure reported success")
			}
		})
	}
}
