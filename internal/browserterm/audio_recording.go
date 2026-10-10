package browserterm

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
	"github.com/jhern254/go-thoughts/internal/voice"
)

type audioRecording struct {
	session             *audioSession
	authority           *voice.TranscriptAuthority
	ctx                 context.Context
	cancel              context.CancelFunc
	mu                  sync.Mutex
	recordingCapability [32]byte
	created             time.Time
	attached            bool
	attachedSignal      chan struct{}
	audioConnection     *websocket.Conn
	pendingAudio        chan []int16
	totalSamples        uint64
	endStatus           byte
	done                chan struct{}
}

func newAudioRecording(session *audioSession, authority *voice.TranscriptAuthority) *audioRecording {
	ctx, cancel := session.settings.withTimeout(session.ctx, session.settings.maximumDuration)
	created := session.settings.now()
	recording := &audioRecording{
		session:        session,
		authority:      authority,
		ctx:            ctx,
		cancel:         cancel,
		created:        created,
		attachedSignal: make(chan struct{}),
		pendingAudio:   make(chan []int16, maximumPendingAudioChunks),
		endStatus:      audioStatusStopped,
		done:           make(chan struct{}),
	}
	rand.Read(recording.recordingCapability[:])
	session.recordings.Add(1)
	go recording.runRecording()
	return recording
}

func (recording *audioRecording) authorizeAudioConnection(recordingCapability [32]byte, audioConnection *websocket.Conn) byte {
	recording.mu.Lock()
	defer recording.mu.Unlock()
	if recording.ctx.Err() != nil || !recording.authority.Active() {
		return audioStatusDenied
	}
	if recording.attached {
		return audioStatusBusy
	}
	if recording.session.settings.now().Sub(recording.created) >= recording.session.settings.capabilityLifetime {
		return audioStatusDenied
	}
	if subtle.ConstantTimeCompare(recordingCapability[:], recording.recordingCapability[:]) != 1 {
		return audioStatusDenied
	}
	recording.attached = true
	clear(recording.recordingCapability[:])
	recording.audioConnection = audioConnection
	close(recording.attachedSignal)
	return audioStatusReady
}

// Rejected chunks are cleared too; the caller stops the recording on rejection.
func (recording *audioRecording) enqueuePCMChunk(pcmSamples []int16) byte {
	recording.mu.Lock()
	defer recording.mu.Unlock()
	reject := func(status byte) byte {
		clear(pcmSamples)
		return status
	}
	if recording.ctx.Err() != nil || !recording.authority.Active() || !recording.attached {
		return reject(audioStatusDenied)
	}
	if len(pcmSamples) != pcmFrameSamples {
		return reject(audioStatusInvalid)
	}
	if recording.session.settings.now().Sub(recording.created) >= recording.session.settings.maximumDuration {
		return reject(audioStatusLimit)
	}
	if uint64(len(pcmSamples)) > recording.session.settings.maximumSamples-recording.totalSamples {
		return reject(audioStatusLimit)
	}
	select {
	case recording.pendingAudio <- pcmSamples:
		recording.totalSamples += uint64(len(pcmSamples))
		return audioStatusReady
	default:
		return reject(audioStatusLimit)
	}
}

func (recording *audioRecording) publishTranscriptUpdate(update voice.TranscriptUpdate) bool {
	if recording.ctx.Err() != nil || !recording.authority.Active() {
		return false
	}
	if update.Recording != recording.authority.Recording() || update.Revision == 0 {
		return false
	}
	if len(update.Text) > voice.MaximumTranscriptBytes || !utf8.ValidString(update.Text) {
		return false
	}
	session := recording.session
	session.publishMu.Lock()
	defer session.publishMu.Unlock()
	if recording.ctx.Err() != nil || !recording.authority.Active() {
		return false
	}
	select {
	case previous := <-session.pendingTranscript:
		if previous.Recording == update.Recording && previous.Revision > update.Revision {
			update = previous
		}
	default:
	}
	session.pendingTranscript <- update
	return true
}

// Revocation precedes cancellation, transport close, and joining. Queued
// results become inert immediately, even if a consumer has not returned yet.
func (recording *audioRecording) stopRecording(status byte) {
	recording.authority.Revoke()
	recording.mu.Lock()
	if recording.ctx.Err() == nil {
		recording.endStatus = status
	}
	clear(recording.recordingCapability[:])
	recording.cancel()
	recording.discardPendingAudio()
	recording.mu.Unlock()
}

func (recording *audioRecording) discardPendingAudio() {
	for {
		select {
		case samples := <-recording.pendingAudio:
			clear(samples)
		default:
			return
		}
	}
}

func (recording *audioRecording) consumePCMChunk(samples []int16) (consumeErr error) {
	defer func() {
		clear(samples)
		if recover() != nil {
			consumeErr = errors.New("audio consumer failed")
		}
	}()
	if recording.ctx.Err() != nil {
		return recording.ctx.Err()
	}
	return recording.session.consumer(recording.ctx, recording.authority.Recording(), samples, recording.publishTranscriptUpdate)
}

func (recording *audioRecording) consumePendingAudio() {
	for {
		select {
		case <-recording.ctx.Done():
			return
		case samples := <-recording.pendingAudio:
			if err := recording.consumePCMChunk(samples); err != nil {
				recording.stopRecording(audioStatusFailed)
				return
			}
		}
	}
}

func (recording *audioRecording) runRecording() {
	defer recording.session.recordings.Done()
	defer close(recording.done)
	defer recording.cancel()
	capabilityCtx, cancelCapability := recording.session.settings.withTimeout(recording.ctx, recording.session.settings.capabilityLifetime)
	select {
	case <-recording.attachedSignal:
	case <-capabilityCtx.Done():
		recording.stopRecording(audioStatusDenied)
	}
	cancelCapability()
	var consumerDone chan struct{}
	if recording.ctx.Err() == nil {
		consumerDone = make(chan struct{})
		// The owner must revoke authority and close transport at the deadline
		// even while a consumer is returning from cancellation. Completion
		// still waits for that consumer; no work is detached.
		go func() { defer close(consumerDone); recording.consumePendingAudio() }()
		<-recording.ctx.Done()
	}
	recording.mu.Lock()
	status := recording.endStatus
	if errors.Is(recording.ctx.Err(), context.DeadlineExceeded) {
		status = audioStatusLimit
		recording.endStatus = status
	}
	audioConnection := recording.audioConnection
	recording.mu.Unlock()
	recording.stopRecording(status)
	if audioConnection != nil {
		writeAudioStatus(recording.session.ctx, audioConnection, status)
		audioConnection.CloseNow()
	}
	if consumerDone != nil {
		<-consumerDone
	}
	recording.session.mu.Lock()
	if recording.session.activeRecording == recording {
		recording.session.activeRecording = nil
	}
	recording.session.mu.Unlock()
	ended := voice.RecordingEnded{Recording: recording.authority.Recording(), Failed: status != audioStatusStopped}
	select {
	case recording.session.ended <- ended:
	case <-recording.session.ctx.Done():
	}
}
