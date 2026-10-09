package browserterm

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/coder/websocket"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
	"github.com/jhern254/go-thoughts/internal/voice"
)

type audioSubjectReader struct{}

func (audioSubjectReader) List(context.Context, string) ([]data.Subject, error) { return nil, nil }

type audioDraftModel struct{ draft thoughts.Model }

func (model audioDraftModel) Init() tea.Cmd                   { return nil }
func (model audioDraftModel) View() tea.View                  { return tea.NewView(model.draft.View()) }
func (model audioDraftModel) VoiceState() thoughts.VoiceState { return model.draft.VoiceState() }
func (model audioDraftModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	var command tea.Cmd
	model.draft, command = model.draft.Update(message)
	return model, command
}

type recordingGrant struct {
	thoughts.VoiceState
	AudioCapability string `json:"audioCapability"`
}

func newAudioTestServer(t *testing.T, consumer PCMConsumer) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- ServeWithAudioConsumer(ctx, listener, func(sessionCtx context.Context) tea.Model {
			draft := thoughts.New(sessionCtx, "test-user", thought.NewService(testutils.NewFakeThoughtStore()), logging.Nop())
			draft.OpenVoice(1, audioSubjectReader{})
			return audioDraftModel{draft: draft}
		}, logging.Nop(), consumer)
	}()
	return "http://" + listener.Addr().String(), func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("audio work did not join")
		}
	}
}

func readRecordingGrant(t *testing.T, terminal *websocket.Conn, predicate func(recordingGrant) bool) recordingGrant {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for {
		_, frame, err := terminal.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(frame) == 0 || frame[0] != 'v' {
			continue
		}
		var grant recordingGrant
		if err := json.Unmarshal(frame[1:], &grant); err != nil {
			t.Fatal("invalid recording metadata")
		}
		if predicate(grant) {
			return grant
		}
	}
}

func startAudioDraft(t *testing.T, address string) (*websocket.Conn, recordingGrant) {
	t.Helper()
	terminal := connect(t, address)
	writeFrame(t, terminal, `2{"cols":80,"rows":30}`)
	readRecordingGrant(t, terminal, func(grant recordingGrant) bool { return grant.SessionID != "" && grant.RecordingStatus == "idle" })
	writeFrame(t, terminal, `v{"action":"start","draftID":1}`)
	grant := readRecordingGrant(t, terminal, func(grant recordingGrant) bool { return grant.AudioCapability != "" })
	return terminal, grant
}

func handshakeForGrant(t *testing.T, grant recordingGrant) []byte {
	t.Helper()
	frame := make([]byte, audioHandshakeBytes)
	copy(frame, []byte{'T', 'A', 1, audioFrameHandshake})
	capability, err := hex.DecodeString(grant.AudioCapability)
	if err != nil {
		t.Fatal(err)
	}
	copy(frame[4:], capability)
	return frame
}

func dialAudio(t *testing.T, address string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	audioConnection, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(address, "http")+"/voice/audio", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {address}}})
	if err != nil {
		t.Fatal(err)
	}
	return audioConnection
}
func readAudioStatus(t *testing.T, audioConnection *websocket.Conn) byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	kind, frame, err := audioConnection.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if kind != websocket.MessageBinary || len(frame) != 5 || !hasAudioHeader(frame, audioFrameStatus) {
		t.Fatal("invalid audio status")
	}
	return frame[4]
}
func writeAudioFrame(t *testing.T, audioConnection *websocket.Conn, frame []byte) {
	t.Helper()
	if err := audioConnection.Write(t.Context(), websocket.MessageBinary, frame); err != nil {
		t.Fatal(err)
	}
}
func pcmFrame() []byte { return make([]byte, pcmFrameBytes) }

func TestAudioTransport(t *testing.T) {
	t.Run("authorized binary PCM reaches only the consumer and Stop joins it", func(t *testing.T) {
		entered := make(chan struct{})
		canceled := make(chan struct{})
		address, stopServer := newAudioTestServer(t, func(ctx context.Context, recording voice.RecordingKey, samples []int16, publish func(voice.TranscriptUpdate) bool) error {
			if len(samples) != 1600 {
				t.Error("wrong sample count")
			}
			publish(voice.TranscriptUpdate{Recording: recording, Revision: 1, Text: "synthetic transcript"})
			close(entered)
			<-ctx.Done()
			close(canceled)
			return ctx.Err()
		})
		defer stopServer()
		terminal, grant := startAudioDraft(t, address)
		defer terminal.CloseNow()
		audioConnection := dialAudio(t, address)
		defer audioConnection.CloseNow()
		writeAudioFrame(t, audioConnection, handshakeForGrant(t, grant))
		if status := readAudioStatus(t, audioConnection); status != audioStatusReady {
			t.Fatalf("got %d, want Ready", status)
		}
		writeAudioFrame(t, audioConnection, pcmFrame())
		<-entered
		writeFrame(t, terminal, `v{"action":"stop","draftID":1,"recordingID":1}`)
		readRecordingGrant(t, terminal, func(grant recordingGrant) bool { return grant.RecordingStatus == "idle" })
		<-canceled
	})
	t.Run("rejects an unknown capability without invoking the consumer", func(t *testing.T) {
		address, stopServer := newAudioTestServer(t, func(context.Context, voice.RecordingKey, []int16, func(voice.TranscriptUpdate) bool) error {
			t.Error("unauthorized consumer invoked")
			return nil
		})
		defer stopServer()
		terminal, grant := startAudioDraft(t, address)
		defer terminal.CloseNow()
		audioConnection := dialAudio(t, address)
		defer audioConnection.CloseNow()
		frame := handshakeForGrant(t, grant)
		frame[4] ^= 1
		writeAudioFrame(t, audioConnection, frame)
		if got := readAudioStatus(t, audioConnection); got != audioStatusDenied {
			t.Fatalf("got %d, want Denied", got)
		}
	})
	t.Run("audio endpoint rejects terminal and text handshake", func(t *testing.T) {
		for _, kind := range []websocket.MessageType{websocket.MessageBinary, websocket.MessageText} {
			address, stopServer := newAudioTestServer(t, discardPCM)
			terminal, _ := startAudioDraft(t, address)
			audioConnection := dialAudio(t, address)
			if err := audioConnection.Write(t.Context(), kind, []byte(`v{"action":"stop"}`)); err != nil {
				t.Fatal(err)
			}
			if status := readAudioStatus(t, audioConnection); status != audioStatusInvalid {
				t.Fatal("accepted terminal frame")
			}
			audioConnection.CloseNow()
			terminal.CloseNow()
			stopServer()
		}
	})
	t.Run("terminal rejects audio protocol and unknown voice payload fields", func(t *testing.T) {
		for _, frame := range [][]byte{append([]byte{'T', 'A', 1, 1}, make([]byte, 32)...), pcmFrame(), []byte(`v{"action":"start","draftID":1,"audio":"PRIVATE-PCM"}`)} {
			address, stopServer := newAudioTestServer(t, discardPCM)
			terminal, _ := startAudioDraft(t, address)
			writeAudioFrame(t, terminal, frame)
			readUntil(t, terminal, "7input")
			terminal.CloseNow()
			stopServer()
		}
	})
}

func TestAudioTransport_RecordingReplacement(t *testing.T) {
	t.Run("restart rejects revoked capability without changing the new recording", func(t *testing.T) {
		address, stopServer := newAudioTestServer(t, discardPCM)
		defer stopServer()
		terminal, previous := startAudioDraft(t, address)
		defer terminal.CloseNow()
		writeFrame(t, terminal, `v{"action":"stop","draftID":1,"recordingID":1}`)
		readRecordingGrant(t, terminal, func(grant recordingGrant) bool { return grant.RecordingStatus == "idle" })
		writeFrame(t, terminal, `v{"action":"start","draftID":1,"recordingID":1}`)
		current := readRecordingGrant(t, terminal, func(grant recordingGrant) bool { return grant.AudioCapability != "" && grant.RecordingID == 2 })
		if current.AudioCapability == previous.AudioCapability {
			t.Fatal("restart reused capability")
		}
		obsolete := dialAudio(t, address)
		writeAudioFrame(t, obsolete, handshakeForGrant(t, previous))
		if got := readAudioStatus(t, obsolete); got != audioStatusDenied {
			t.Fatalf("got %d, want Denied", got)
		}
		obsolete.CloseNow()
		audioConnection := dialAudio(t, address)
		defer audioConnection.CloseNow()
		writeAudioFrame(t, audioConnection, handshakeForGrant(t, current))
		if got := readAudioStatus(t, audioConnection); got != audioStatusReady {
			t.Fatalf("new recording got %d, want Ready", got)
		}
	})
	t.Run("independent sessions reject old session capabilities", func(t *testing.T) {
		address, stopServer := newAudioTestServer(t, discardPCM)
		defer stopServer()
		terminal, previous := startAudioDraft(t, address)
		terminal.CloseNow()
		secondAddress, stopSecond := newAudioTestServer(t, discardPCM)
		defer stopSecond()
		secondTerminal, current := startAudioDraft(t, secondAddress)
		defer secondTerminal.CloseNow()
		if current.SessionID == previous.SessionID {
			t.Fatal("new session reused identity")
		}
		obsolete := dialAudio(t, secondAddress)
		defer obsolete.CloseNow()
		writeAudioFrame(t, obsolete, handshakeForGrant(t, previous))
		if got := readAudioStatus(t, obsolete); got != audioStatusDenied {
			t.Fatalf("old session got %d, want Denied", got)
		}
	})
}
