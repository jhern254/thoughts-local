package browserterm

import (
	"context"
	"encoding/hex"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/voice"
)

// This fixture isolates HTTP ingestion from the editor. Timeout registration
// gives tests an explicit barrier before interrupting a blocked socket read.
func newIngestionTestServer(t *testing.T, consumer PCMConsumer, configure func(*audioSettings)) (string, *audioRecording) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	session := newAudioSession(ctx, consumer)
	if configure != nil {
		configure(&session.settings)
	}
	authority := voice.NewTranscriptAuthority(voice.RecordingKey{SessionID: session.sessionID, DraftID: 1, RecordingID: 1})
	recording := newAudioRecording(session, authority)
	session.activeRecording = recording
	handler := &server{ctx: ctx, audioSession: session, logger: logging.Nop()}
	httpServer := httptest.NewUnstartedServer(handler)
	handler.authority = httpServer.Listener.Addr().String()
	httpServer.Start()
	t.Cleanup(func() {
		cancel()
		session.close()
		handler.sessions.Wait()
		httpServer.Close()
	})
	return httpServer.URL, recording
}

func authorizeTestRecording(t *testing.T, address string, recording *audioRecording) *websocket.Conn {
	t.Helper()
	grant := recordingGrant{}
	grant.SessionID = recording.session.sessionID
	grant.DraftID = 1
	grant.RecordingID = 1
	recording.mu.Lock()
	grant.AudioCapability = hex.EncodeToString(recording.recordingCapability[:])
	recording.mu.Unlock()
	audioConnection := dialAudio(t, address)
	t.Cleanup(func() { audioConnection.CloseNow() })
	writeAudioFrame(t, audioConnection, handshakeForGrant(t, grant))
	if got := readAudioStatus(t, audioConnection); got != audioStatusReady {
		t.Fatalf("authorization got %d, want Ready", got)
	}
	return audioConnection
}

func TestAudioTransport_LimitsAndTimeouts(t *testing.T) {
	t.Run("simultaneous handshakes attach only one connection", func(t *testing.T) {
		address, recording := newIngestionTestServer(t, discardPCM, nil)
		recording.mu.Lock()
		grant := recordingGrant{AudioCapability: hex.EncodeToString(recording.recordingCapability[:])}
		recording.mu.Unlock()
		first := dialAudio(t, address)
		defer first.CloseNow()
		second := dialAudio(t, address)
		defer second.CloseNow()
		// Both sockets exist before either presents the capability. Attachment,
		// rather than a separate handshake reservation, decides the winner.
		writeAudioFrame(t, first, handshakeForGrant(t, grant))
		writeAudioFrame(t, second, handshakeForGrant(t, grant))
		firstStatus, secondStatus := readAudioStatus(t, first), readAudioStatus(t, second)
		if (firstStatus != audioStatusReady || secondStatus != audioStatusBusy) && (firstStatus != audioStatusBusy || secondStatus != audioStatusReady) {
			t.Fatalf("statuses got %d and %d, want one Ready and one Busy", firstStatus, secondStatus)
		}
		if !recording.authority.Active() {
			t.Fatal("losing connection revoked the winner")
		}
	})
	t.Run("overflow ends recording even while consumption is held", func(t *testing.T) {
		entered := make(chan struct{})
		address, recording := newIngestionTestServer(t, func(ctx context.Context, _ voice.RecordingKey, _ []int16, _ func(voice.TranscriptUpdate) bool) error {
			close(entered)
			<-ctx.Done()
			return ctx.Err()
		}, nil)
		audioConnection := authorizeTestRecording(t, address, recording)
		writeAudioFrame(t, audioConnection, pcmFrame())
		<-entered
		for range maximumPendingAudioChunks + 1 {
			writeAudioFrame(t, audioConnection, pcmFrame())
		}
		<-recording.done
		if recording.authority.Active() || len(recording.pendingAudio) != 0 {
			t.Fatal("overflow retained authority or queued PCM")
		}
	})
	for _, scenario := range []struct {
		name  string
		kind  websocket.MessageType
		frame []byte
	}{
		{"text PCM", websocket.MessageText, pcmFrame()},
		{"terminal control", websocket.MessageBinary, []byte(`v{"action":"stop"}`)},
		{"long PCM frame", websocket.MessageBinary, append(pcmFrame(), 42)},
		{"short PCM frame", websocket.MessageBinary, make([]byte, pcmFrameBytes-2)},
		{"oversized message", websocket.MessageBinary, make([]byte, maximumAudioMessageBytes+1)},
	} {
		t.Run("rejects "+scenario.name+" and revokes the recording", func(t *testing.T) {
			address, recording := newIngestionTestServer(t, func(context.Context, voice.RecordingKey, []int16, func(voice.TranscriptUpdate) bool) error {
				t.Error("invalid frame reached consumer")
				return nil
			}, nil)
			audioConnection := authorizeTestRecording(t, address, recording)
			if err := audioConnection.Write(t.Context(), scenario.kind, scenario.frame); err != nil {
				t.Fatal(err)
			}
			<-recording.done
			if recording.authority.Active() {
				t.Fatal("invalid frame retained transcript authority")
			}
		})
	}
	for _, timeoutDuration := range []time.Duration{audioHandshakeTimeout, audioInactivityTimeout} {
		t.Run("bounds blocked read "+timeoutDuration.String(), func(t *testing.T) {
			readStarted := make(chan context.CancelFunc, 1)
			address, recording := newIngestionTestServer(t, discardPCM, func(settings *audioSettings) {
				settings.withTimeout = func(parent context.Context, duration time.Duration) (context.Context, context.CancelFunc) {
					if duration != timeoutDuration {
						return context.WithTimeout(parent, duration)
					}
					ctx, cancel := context.WithCancel(parent)
					readStarted <- cancel
					return ctx, cancel
				}
			})
			var audioConnection *websocket.Conn
			if timeoutDuration == audioHandshakeTimeout {
				audioConnection = dialAudio(t, address)
				t.Cleanup(func() { audioConnection.CloseNow() })
			} else {
				audioConnection = authorizeTestRecording(t, address, recording)
			}
			cancelRead := <-readStarted
			cancelRead()
			readCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			for {
				if _, _, err := audioConnection.Read(readCtx); err != nil {
					break
				}
			}
			if timeoutDuration == audioInactivityTimeout {
				<-recording.done
				if recording.authority.Active() {
					t.Fatal("inactivity retained authority")
				}
			} else if !recording.authority.Active() {
				t.Fatal("unauthorized handshake timeout revoked the legitimate recording")
			}
		})
	}
	t.Run("duplicate connection cannot consume or cancel the authorized connection", func(t *testing.T) {
		address, recording := newIngestionTestServer(t, discardPCM, nil)
		recording.mu.Lock()
		grant := recordingGrant{AudioCapability: hex.EncodeToString(recording.recordingCapability[:])}
		recording.mu.Unlock()
		grant.SessionID, grant.DraftID, grant.RecordingID = recording.session.sessionID, 1, 1
		authorizeTestRecording(t, address, recording)
		duplicate := dialAudio(t, address)
		defer duplicate.CloseNow()
		writeAudioFrame(t, duplicate, handshakeForGrant(t, grant))
		if got := readAudioStatus(t, duplicate); got != audioStatusBusy {
			t.Fatalf("got %d, want Busy", got)
		}
		if !recording.authority.Active() {
			t.Fatal("duplicate revoked the original")
		}
	})
}

func TestAudioRecording_Deadline(t *testing.T) {
	t.Run("revokes and closes transport before a canceled consumer returns", func(t *testing.T) {
		entered := make(chan struct{})
		release := make(chan struct{})
		var expireRecording context.CancelFunc
		address, recording := newIngestionTestServer(t, func(ctx context.Context, recordingKey voice.RecordingKey, _ []int16, publish func(voice.TranscriptUpdate) bool) error {
			close(entered)
			<-ctx.Done()
			<-release
			if publish(voice.TranscriptUpdate{Recording: recordingKey, Revision: 1, Text: "late"}) {
				t.Error("deadline accepted a late result")
			}
			return ctx.Err()
		}, func(settings *audioSettings) {
			settings.withTimeout = func(parent context.Context, duration time.Duration) (context.Context, context.CancelFunc) {
				if duration == maximumRecordingDuration {
					ctx, cancel := context.WithCancel(parent)
					expireRecording = cancel
					return ctx, cancel
				}
				return context.WithTimeout(parent, duration)
			}
		})
		defer close(release)
		audioConnection := authorizeTestRecording(t, address, recording)
		writeAudioFrame(t, audioConnection, pcmFrame())
		<-entered
		expireRecording()
		readCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		for {
			if _, _, err := audioConnection.Read(readCtx); err != nil {
				break
			}
		}
		if recording.authority.Active() {
			t.Error("deadline retained transcript authority")
		}
		select {
		case <-recording.done:
			t.Error("completion preceded consumer return")
		default:
		}
	})
}

func TestAudioTransport_OwnedWork(t *testing.T) {
	for _, scenario := range []string{"audio disconnect", "terminal disconnect", "server shutdown"} {
		t.Run(scenario+" revokes results and joins held consumer", func(t *testing.T) {
			entered := make(chan struct{})
			canceled := make(chan struct{})
			release := make(chan struct{})
			lateAccepted := make(chan bool, 1)
			address, stopServer := newAudioTestServer(t, func(ctx context.Context, recording voice.RecordingKey, samples []int16, publish func(voice.TranscriptUpdate) bool) error {
				close(entered)
				<-ctx.Done()
				close(canceled)
				<-release
				lateAccepted <- publish(voice.TranscriptUpdate{Recording: recording, Revision: 999, Text: "late synthetic"})
				return ctx.Err()
			})
			terminal, grant := startAudioDraft(t, address)
			defer terminal.CloseNow()
			audioConnection := dialAudio(t, address)
			defer audioConnection.CloseNow()
			writeAudioFrame(t, audioConnection, handshakeForGrant(t, grant))
			readAudioStatus(t, audioConnection)
			writeAudioFrame(t, audioConnection, pcmFrame())
			<-entered
			shutdownDone := make(chan struct{})
			switch scenario {
			case "audio disconnect":
				audioConnection.CloseNow()
			case "terminal disconnect":
				terminal.CloseNow()
			case "server shutdown":
				go func() { stopServer(); close(shutdownDone) }()
			}
			<-canceled
			if scenario == "server shutdown" {
				select {
				case <-shutdownDone:
					t.Error("server returned before consumer joined")
				default:
				}
			}
			close(release)
			if <-lateAccepted {
				t.Error("accepted transcript after disconnect or shutdown")
			}
			if scenario == "server shutdown" {
				<-shutdownDone
			} else {
				stopServer()
			}
		})
	}
}
