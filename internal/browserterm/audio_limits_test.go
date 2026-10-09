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
		name         string
		configure    func(*audioSettings)
		firstSamples int
		nextSamples  int
		nextSequence uint32
		advance      time.Duration
		want         byte
	}{
		{name: "aggregate bytes", configure: func(settings *audioSettings) { settings.maximumBytes = 8 }, firstSamples: 4, nextSamples: 1, nextSequence: 1, want: audioStatusLimit},
		{name: "aggregate samples", configure: func(settings *audioSettings) { settings.maximumSamples = 4 }, firstSamples: 4, nextSamples: 1, nextSequence: 1, want: audioStatusLimit},
		{name: "elapsed duration", firstSamples: 1, nextSamples: 1, nextSequence: 1, advance: maximumRecordingDuration, want: audioStatusLimit},
		{name: "out of sequence", firstSamples: 1, nextSamples: 1, nextSequence: 2, want: audioStatusInvalid},
		{name: "oversized chunk", firstSamples: 1, nextSamples: maximumPCMChunkSamples + 1, nextSequence: 1, want: audioStatusInvalid},
	} {
		t.Run("rejects "+scenario.name, func(t *testing.T) {
			consumed := make(chan struct{}, 2)
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
			if status := recording.authorizeAudioConnection(audioHandshake{recording: authority.Recording(), capability: recording.capability}, nil); status != audioStatusReady {
				t.Fatal("attach failed")
			}
			if status := recording.enqueuePCMChunk(0, make([]int16, scenario.firstSamples)); status != audioStatusReady {
				t.Fatal("initial chunk rejected")
			}
			<-consumed
			now = now.Add(scenario.advance)
			rejected := make([]int16, scenario.nextSamples)
			rejected[0] = 123
			if got := recording.enqueuePCMChunk(scenario.nextSequence, rejected); got != scenario.want {
				t.Fatalf("got %d, want %d", got, scenario.want)
			}
			for _, sample := range rejected {
				if sample != 0 {
					t.Fatal("rejected audio retained")
				}
			}
		})
	}
	for _, scenario := range []struct {
		name     string
		samples  int
		accepted int
	}{
		{"message rate", 1, 20}, {"sample ingress rate", 3200, 10},
	} {
		t.Run("limits "+scenario.name+" independently of consumer speed", func(t *testing.T) {
			consumed := make(chan struct{}, 1)
			session := newAudioSession(t.Context(), func(context.Context, voice.RecordingKey, []int16, func(voice.TranscriptUpdate) bool) error {
				consumed <- struct{}{}
				return nil
			})
			now := time.Now()
			session.settings.now = func() time.Time { return now }
			authority := voice.NewTranscriptAuthority(voice.RecordingKey{SessionID: session.sessionID, DraftID: 1, RecordingID: 1})
			recording := newAudioRecording(session, authority)
			t.Cleanup(func() { recording.stopRecording(audioStatusStopped); <-recording.done })
			recording.authorizeAudioConnection(audioHandshake{recording: authority.Recording(), capability: recording.capability}, nil)
			for sequence := 0; sequence < scenario.accepted; sequence++ {
				if got := recording.enqueuePCMChunk(uint32(sequence), make([]int16, scenario.samples)); got != audioStatusReady {
					t.Fatalf("chunk %d got %d", sequence, got)
				}
				<-consumed
			}
			if got := recording.enqueuePCMChunk(uint32(scenario.accepted), make([]int16, scenario.samples)); got != audioStatusLimit {
				t.Fatalf("got %d, want rate limit", got)
			}
			now = now.Add(time.Second)
			if got := recording.enqueuePCMChunk(uint32(scenario.accepted), make([]int16, scenario.samples)); got != audioStatusReady {
				t.Fatalf("got %d after replenishment, want Ready", got)
			}
		})
	}
	t.Run("tiny frames cannot exceed queue depth", func(t *testing.T) {
		entered := make(chan struct{})
		session := newAudioSession(t.Context(), func(ctx context.Context, _ voice.RecordingKey, _ []int16, _ func(voice.TranscriptUpdate) bool) error {
			close(entered)
			<-ctx.Done()
			return ctx.Err()
		})
		now := time.Now()
		session.settings.now = func() time.Time { return now }
		authority := voice.NewTranscriptAuthority(voice.RecordingKey{SessionID: session.sessionID, DraftID: 1, RecordingID: 1})
		recording := newAudioRecording(session, authority)
		t.Cleanup(func() { recording.stopRecording(audioStatusStopped); <-recording.done })
		recording.authorizeAudioConnection(audioHandshake{recording: authority.Recording(), capability: recording.capability}, nil)
		recording.enqueuePCMChunk(0, []int16{1})
		<-entered
		for sequence := uint32(1); sequence <= maximumPendingAudioChunks; sequence++ {
			now = now.Add(time.Second)
			if got := recording.enqueuePCMChunk(sequence, []int16{1}); got != audioStatusReady {
				t.Fatalf("got %d before queue full", got)
			}
		}
		now = now.Add(time.Second)
		if got := recording.enqueuePCMChunk(21, []int16{1}); got != audioStatusLimit {
			t.Fatalf("got %d, want bounded queue", got)
		}
	})
}

func TestAudioRecording_AuthorizationAndDeadlines(t *testing.T) {
	t.Run("capabilities are bound one use revoked and expiring", func(t *testing.T) {
		session := newAudioSession(t.Context(), discardPCM)
		now := time.Now()
		session.settings.now = func() time.Time { return now }
		authority := voice.NewTranscriptAuthority(voice.RecordingKey{SessionID: session.sessionID, DraftID: 1, RecordingID: 1})
		recording := newAudioRecording(session, authority)
		t.Cleanup(func() { recording.stopRecording(audioStatusStopped); <-recording.done })
		handshake := audioHandshake{recording: authority.Recording(), capability: recording.capability}
		wrong := handshake
		wrong.capability[0] ^= 1
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
			recording.authorizeAudioConnection(audioHandshake{recording: authority.Recording(), capability: recording.capability}, nil)
			pcmSamples := []int16{1234}
			recording.enqueuePCMChunk(0, pcmSamples)
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
