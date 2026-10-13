package browserterm

import (
	"context"
	"errors"
	"testing"

	"github.com/jhern254/go-thoughts/internal/voice"
)

type testRecordingConsumer struct {
	consume func(context.Context, []int16) error
	close   func() error
}

func (consumer *testRecordingConsumer) ConsumePCM(ctx context.Context, samples []int16) error {
	return consumer.consume(ctx, samples)
}
func (consumer *testRecordingConsumer) Close() error { return consumer.close() }

func TestRecordingPCMConsumer_Lifecycle(t *testing.T) {
	t.Run("creates once after attachment and closes before recording completion", func(t *testing.T) {
		entered := make(chan struct{})
		canceled := make(chan struct{})
		release := make(chan struct{})
		creationCalls, closeCalls := 0, 0
		var borrowed []int16
		session := newAudioSessionWithFactory(t.Context(), func(ctx context.Context, recordingKey voice.RecordingKey, publish func(voice.TranscriptUpdate) bool) (RecordingPCMConsumer, error) {
			creationCalls++
			return &testRecordingConsumer{
				consume: func(ctx context.Context, samples []int16) error {
					borrowed = samples
					close(entered)
					<-ctx.Done()
					close(canceled)
					<-release
					return ctx.Err()
				},
				close: func() error { closeCalls++; return nil },
			}, nil
		})
		authority := voice.NewTranscriptAuthority(voice.RecordingKey{SessionID: session.sessionID, DraftID: 1, RecordingID: 1})
		recording := newAudioRecording(session, authority)
		if creationCalls != 0 {
			t.Fatal("created consumer before attachment")
		}
		recording.authorizeAudioConnection(recording.recordingCapability, nil)
		samples := make([]int16, pcmFrameSamples)
		samples[0] = 123
		recording.enqueuePCMChunk(samples)
		<-entered
		recording.stopRecording(audioStatusStopped)
		<-canceled
		if authority.Active() {
			t.Fatal("Stop did not revoke authority before consumer returned")
		}
		select {
		case <-recording.done:
			t.Fatal("completed before consumer joined")
		default:
		}
		close(release)
		<-recording.done
		if creationCalls != 1 || closeCalls != 1 {
			t.Fatalf("creation=%d close=%d, want once", creationCalls, closeCalls)
		}
		if borrowed[0] != 0 {
			t.Fatal("retained borrowed PCM")
		}
	})
	t.Run("multiple chunks reuse the recording consumer", func(t *testing.T) {
		consumed := make(chan struct{}, 2)
		creationCalls, closeCalls := 0, 0
		session := newAudioSessionWithFactory(t.Context(), func(context.Context, voice.RecordingKey, func(voice.TranscriptUpdate) bool) (RecordingPCMConsumer, error) {
			creationCalls++
			return &testRecordingConsumer{consume: func(context.Context, []int16) error { consumed <- struct{}{}; return nil }, close: func() error { closeCalls++; return nil }}, nil
		})
		authority := voice.NewTranscriptAuthority(voice.RecordingKey{SessionID: session.sessionID, DraftID: 1, RecordingID: 1})
		recording := newAudioRecording(session, authority)
		recording.authorizeAudioConnection(recording.recordingCapability, nil)
		for range 2 {
			recording.enqueuePCMChunk(make([]int16, pcmFrameSamples))
			<-consumed
		}
		recording.stopRecording(audioStatusStopped)
		<-recording.done
		if creationCalls != 1 || closeCalls != 1 {
			t.Fatalf("creation=%d close=%d, want once", creationCalls, closeCalls)
		}
	})

	t.Run("creation failure revokes authority and reports fixed failure", func(t *testing.T) {
		session := newAudioSessionWithFactory(t.Context(), func(context.Context, voice.RecordingKey, func(voice.TranscriptUpdate) bool) (RecordingPCMConsumer, error) {
			return nil, errors.New("private model path")
		})
		authority := voice.NewTranscriptAuthority(voice.RecordingKey{SessionID: session.sessionID, DraftID: 1, RecordingID: 1})
		recording := newAudioRecording(session, authority)
		recording.authorizeAudioConnection(recording.recordingCapability, nil)
		<-recording.done
		ended := <-session.ended
		if authority.Active() || !ended.Failed {
			t.Fatal("creation failure retained authority or reported success")
		}
	})
	t.Run("close failure remains observable after cancellation", func(t *testing.T) {
		created := make(chan struct{})
		session := newAudioSessionWithFactory(t.Context(), func(context.Context, voice.RecordingKey, func(voice.TranscriptUpdate) bool) (RecordingPCMConsumer, error) {
			close(created)
			return &testRecordingConsumer{consume: func(context.Context, []int16) error { return nil }, close: func() error { return errors.New("private close detail") }}, nil
		})
		authority := voice.NewTranscriptAuthority(voice.RecordingKey{SessionID: session.sessionID, DraftID: 1, RecordingID: 1})
		recording := newAudioRecording(session, authority)
		recording.authorizeAudioConnection(recording.recordingCapability, nil)
		<-created
		recording.stopRecording(audioStatusStopped)
		<-recording.done
		if ended := <-session.ended; !ended.Failed {
			t.Fatal("cleanup failure was hidden by cancellation")
		}
	})
}
