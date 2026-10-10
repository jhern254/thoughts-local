package browserterm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
	"github.com/jhern254/go-thoughts/internal/voice"
)

// PCMConsumer borrows one mono PCM16 chunk at 16 kHz until it returns. It must
// honor ctx and retain no samples. Only tests publish text in this transport PR;
// no recognizer, storage, or logging dependency is supplied to the consumer.
type PCMConsumer func(ctx context.Context, recording voice.RecordingKey, samples []int16, publish func(voice.TranscriptUpdate) bool) error

func discardPCM(context.Context, voice.RecordingKey, []int16, func(voice.TranscriptUpdate) bool) error {
	return nil
}

type audioSettings struct {
	now                func() time.Time
	withTimeout        func(context.Context, time.Duration) (context.Context, context.CancelFunc)
	maximumSamples     uint64
	maximumDuration    time.Duration
	capabilityLifetime time.Duration
}

func defaultAudioSettings() audioSettings {
	return audioSettings{
		now:                time.Now,
		withTimeout:        context.WithTimeout,
		maximumSamples:     maximumRecordingSamples,
		maximumDuration:    maximumRecordingDuration,
		capabilityLifetime: audioCapabilityLifetime,
	}
}

type audioSession struct {
	recordings        sync.WaitGroup
	ctx               context.Context
	sessionID         string
	consumer          PCMConsumer
	settings          audioSettings
	mu                sync.Mutex
	activeRecording   *audioRecording
	closing           bool
	publishMu         sync.Mutex
	pendingTranscript chan voice.TranscriptUpdate
	ended             chan voice.RecordingEnded
	deliveryDone      chan struct{}
}

func newAudioSession(ctx context.Context, consumer PCMConsumer) *audioSession {
	var sessionIdentity [16]byte
	rand.Read(sessionIdentity[:])
	if consumer == nil {
		consumer = discardPCM
	}
	return &audioSession{
		ctx:               ctx,
		sessionID:         hex.EncodeToString(sessionIdentity[:]),
		consumer:          consumer,
		settings:          defaultAudioSettings(),
		pendingTranscript: make(chan voice.TranscriptUpdate, 1),
		ended:             make(chan voice.RecordingEnded, 1),
	}
}

func (session *audioSession) startDelivery(send func(tea.Msg)) {
	session.deliveryDone = make(chan struct{})
	go func() {
		defer close(session.deliveryDone)
		for {
			select {
			case <-session.ctx.Done():
				return
			case update := <-session.pendingTranscript:
				send(update)
			case ended := <-session.ended:
				send(ended)
			}
		}
	}()
}

// synchronizeRecording observes the model after its synchronous authority
// transitions. Stop never waits for a worker inside the Bubble Tea event loop.
func (session *audioSession) synchronizeRecording(voiceState thoughts.VoiceState) (string, *voice.RecordingEnded) {
	session.mu.Lock()
	defer session.mu.Unlock()
	authority := voiceState.TranscriptAuthority
	if session.activeRecording != nil && (authority != session.activeRecording.authority || !authority.Active() || voiceState.RecordingStatus == "stopping") {
		session.activeRecording.stopRecording(audioStatusStopped)
	}
	if authority == nil || !authority.Active() || session.closing {
		return "", nil
	}
	if session.activeRecording == nil || session.activeRecording.authority != authority {
		if session.activeRecording != nil {
			select {
			case <-session.activeRecording.done:
			default:
				authority.Revoke()
				return "", &voice.RecordingEnded{Recording: authority.Recording(), Failed: true}
			}
		}
		session.activeRecording = newAudioRecording(session, authority)
	}
	recording := session.activeRecording
	recording.mu.Lock()
	defer recording.mu.Unlock()
	if recording.attached || recording.ctx.Err() != nil {
		return "", nil
	}
	return hex.EncodeToString(recording.recordingCapability[:]), nil
}

func (session *audioSession) revokeRecording() {
	session.mu.Lock()
	recording := session.activeRecording
	session.mu.Unlock()
	if recording != nil {
		recording.stopRecording(audioStatusStopped)
	}
}

func (session *audioSession) close() {
	session.mu.Lock()
	session.closing = true
	recording := session.activeRecording
	session.mu.Unlock()
	if recording != nil {
		recording.stopRecording(audioStatusStopped)
		<-recording.done
	}
	session.recordings.Wait()
	if session.deliveryDone != nil {
		<-session.deliveryDone
	}
	session.publishMu.Lock()
	select {
	case <-session.pendingTranscript:
	default:
	}
	session.publishMu.Unlock()
}
