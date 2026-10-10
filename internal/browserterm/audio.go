package browserterm

import (
	"context"
	"io"
	"net/http"

	"github.com/coder/websocket"
)

func writeAudioStatus(ctx context.Context, audioConnection *websocket.Conn, status byte) {
	writeCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	_ = audioConnection.Write(writeCtx, websocket.MessageBinary, []byte{'T', 'A', audioProtocolVersion, audioFrameStatus, status})
}

// A fixed read buffer bounds fragmented messages as well as ordinary chunks.
// Compression is disabled so no PCM enters a reusable compression dictionary.
func readAudioMessage(ctx context.Context, audioConnection *websocket.Conn, storage []byte) ([]byte, error) {
	messageType, messageReader, err := audioConnection.Reader(ctx)
	if err != nil {
		return nil, errAudioInput
	}
	if messageType != websocket.MessageBinary {
		return nil, errAudioInput
	}
	count, err := io.ReadFull(messageReader, storage)
	if err != io.ErrUnexpectedEOF || count > maximumAudioMessageBytes {
		return nil, errAudioInput
	}
	return storage[:count], nil
}

func (server *server) connectAudio(response http.ResponseWriter, request *http.Request) {
	server.mu.Lock()
	session := server.audioSession
	if server.closing || session == nil {
		server.mu.Unlock()
		http.Error(response, "Audio unavailable", http.StatusForbidden)
		return
	}
	session.mu.Lock()
	if session.closing || session.activeRecording == nil {
		session.mu.Unlock()
		server.mu.Unlock()
		http.Error(response, "Audio unavailable", http.StatusForbidden)
		return
	}
	server.sessions.Add(1)
	session.mu.Unlock()
	server.mu.Unlock()
	defer server.sessions.Done()
	audioConnection, err := websocket.Accept(&handshakeWriter{ResponseWriter: response}, request, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	defer audioConnection.CloseNow()
	audioConnection.SetReadLimit(maximumAudioMessageBytes)
	var storage [maximumAudioMessageBytes + 1]byte
	defer clear(storage[:])
	handshakeCtx, cancelHandshake := session.settings.withTimeout(session.ctx, audioHandshakeTimeout)
	frame, err := readAudioMessage(handshakeCtx, audioConnection, storage[:])
	cancelHandshake()
	if err != nil {
		writeAudioStatus(session.ctx, audioConnection, audioStatusInvalid)
		return
	}
	recordingCapability, err := decodeAudioHandshake(frame)
	clear(storage[:])
	if err != nil {
		writeAudioStatus(session.ctx, audioConnection, audioStatusInvalid)
		return
	}
	session.mu.Lock()
	recording := session.activeRecording
	session.mu.Unlock()
	if recording == nil {
		writeAudioStatus(session.ctx, audioConnection, audioStatusDenied)
		return
	}
	status := recording.authorizeAudioConnection(recordingCapability, audioConnection)
	clear(recordingCapability[:])
	if status == audioStatusReady {
		defer func() { recording.stopRecording(audioStatusFailed); <-recording.done }()
	}
	writeAudioStatus(session.ctx, audioConnection, status)
	if status != audioStatusReady {
		return
	}
	for {
		// Canceling a WebSocket read closes its connection. The recording owner
		// must revoke transcript authority before closing it on Stop or expiry.
		readCtx, cancelRead := session.settings.withTimeout(session.ctx, audioInactivityTimeout)
		frame, err = readAudioMessage(readCtx, audioConnection, storage[:])
		cancelRead()
		if err != nil {
			return
		}
		samples, decodeErr := decodePCMFrame(frame)
		clear(storage[:])
		if decodeErr != nil {
			recording.stopRecording(audioStatusInvalid)
			return
		}
		if status = recording.enqueuePCMChunk(samples); status != audioStatusReady {
			recording.stopRecording(status)
			return
		}
	}
}
