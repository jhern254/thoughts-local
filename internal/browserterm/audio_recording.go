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
	session            *audioSession
	authority          *voice.TranscriptAuthority
	ctx                context.Context
	cancel             context.CancelFunc
	mu                 sync.Mutex
	capability         [32]byte
	created            time.Time
	attached           bool
	attachedSignal     chan struct{}
	audioConnection    *websocket.Conn
	pendingAudio       chan []int16
	outstandingSamples int
	totalBytes         uint64
	totalSamples       uint64
	expectedSequence   uint32
	sampleCredit       float64
	messageCredit      float64
	lastIngress        time.Time
	endStatus          byte
	done               chan struct{}
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
		sampleCredit:   maximumPendingAudioSamples,
		messageCredit:  maximumAudioMessageBurst,
		lastIngress:    created,
		endStatus:      audioStatusStopped,
		done:           make(chan struct{}),
	}
	rand.Read(recording.capability[:])
	session.recordings.Add(1)
	go recording.runRecording()
	return recording
}

func (recording *audioRecording) authorizeAudioConnection(handshake audioHandshake, audioConnection *websocket.Conn) byte {
	recording.mu.Lock()
	defer recording.mu.Unlock()
	if handshake.recording != recording.authority.Recording() || recording.ctx.Err() != nil || !recording.authority.Active() {
		return audioStatusDenied
	}
	if recording.attached {
		return audioStatusBusy
	}
	if recording.session.settings.now().Sub(recording.created) >= recording.session.settings.capabilityLifetime {
		return audioStatusDenied
	}
	if subtle.ConstantTimeCompare(handshake.capability[:], recording.capability[:]) != 1 {
		return audioStatusDenied
	}
	recording.attached = true
	clear(recording.capability[:])
	recording.audioConnection = audioConnection
	close(recording.attachedSignal)
	return audioStatusReady
}

// enqueue takes ownership even when rejecting a chunk. Independent server
// counters prevent a fast client or tiny frames from bypassing queue bounds.
func (recording *audioRecording) enqueuePCMChunk(sequence uint32, pcmSamples []int16) byte {
	recording.mu.Lock()
	defer recording.mu.Unlock()
	reject := func(status byte) byte {
		clear(pcmSamples)
		return status
	}
	if recording.ctx.Err() != nil || !recording.authority.Active() || !recording.attached {
		return reject(audioStatusDenied)
	}
	if sequence != recording.expectedSequence || len(pcmSamples) == 0 || len(pcmSamples) > maximumPCMChunkSamples {
		return reject(audioStatusInvalid)
	}
	now := recording.session.settings.now()
	if now.Sub(recording.created) >= recording.session.settings.maximumDuration {
		return reject(audioStatusLimit)
	}
	elapsed := now.Sub(recording.lastIngress).Seconds()
	if elapsed < 0 {
		return reject(audioStatusLimit)
	}
	recording.sampleCredit = min(float64(maximumPendingAudioSamples), recording.sampleCredit+elapsed*audioSampleRateHz)
	recording.messageCredit = min(float64(maximumAudioMessageBurst), recording.messageCredit+elapsed*maximumAudioMessagesPerSecond)
	recording.lastIngress = now
	sampleCount := uint64(len(pcmSamples))
	byteCount := sampleCount * 2
	if byteCount > recording.session.settings.maximumBytes-recording.totalBytes {
		return reject(audioStatusLimit)
	}
	if sampleCount > recording.session.settings.maximumSamples-recording.totalSamples {
		return reject(audioStatusLimit)
	}
	if float64(sampleCount) > recording.sampleCredit || recording.messageCredit < 1 {
		return reject(audioStatusLimit)
	}
	if recording.outstandingSamples+len(pcmSamples) > maximumPendingAudioSamples {
		return reject(audioStatusLimit)
	}
	select {
	case recording.pendingAudio <- pcmSamples:
		recording.outstandingSamples += len(pcmSamples)
		recording.totalBytes += byteCount
		recording.totalSamples += sampleCount
		recording.expectedSequence++
		recording.sampleCredit -= float64(sampleCount)
		recording.messageCredit--
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
	clear(recording.capability[:])
	recording.cancel()
	recording.discardPendingAudio()
	recording.mu.Unlock()
}

func (recording *audioRecording) discardPendingAudio() {
	for {
		select {
		case samples := <-recording.pendingAudio:
			recording.outstandingSamples -= len(samples)
			clear(samples)
		default:
			return
		}
	}
}

func (recording *audioRecording) consumePCMChunk(samples []int16) (consumeErr error) {
	defer func() {
		clear(samples)
		recording.mu.Lock()
		recording.outstandingSamples -= len(samples)
		recording.mu.Unlock()
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
	if recording.ctx.Err() == nil {
		consumerDone := make(chan struct{})
		go func() { defer close(consumerDone); recording.consumePendingAudio() }()
		<-recording.ctx.Done()
		recording.authority.Revoke()
		recording.mu.Lock()
		audioConnection := recording.audioConnection
		status := recording.endStatus
		if errors.Is(recording.ctx.Err(), context.DeadlineExceeded) {
			status = audioStatusLimit
			recording.endStatus = status
		}
		recording.mu.Unlock()
		if audioConnection != nil {
			writeAudioStatus(recording.session.ctx, audioConnection, status)
			audioConnection.CloseNow()
		}
		<-consumerDone
	}
	recording.mu.Lock()
	endStatus := recording.endStatus
	recording.mu.Unlock()
	recording.stopRecording(endStatus)
	recording.mu.Lock()
	audioConnection := recording.audioConnection
	status := recording.endStatus
	recording.mu.Unlock()
	if audioConnection != nil {
		audioConnection.CloseNow()
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
