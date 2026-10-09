package browserterm

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jhern254/go-thoughts/internal/voice"
)

const (
	audioProtocolVersion          = 1
	audioFrameHandshake           = 1
	audioFramePCM                 = 2
	audioFrameStatus              = 3
	audioHandshakeBytes           = 76
	audioPCMHeaderBytes           = 12
	audioSampleRateHz             = 16_000
	maximumAudioMessageBytes      = 8192
	maximumPCMChunkSamples        = 3200
	maximumRecordingAudioBytes    = 38_400_000
	maximumRecordingSamples       = 19_200_000
	maximumPendingAudioSamples    = 32_000
	maximumPendingAudioChunks     = 20
	maximumAudioMessagesPerSecond = 50
	maximumAudioMessageBurst      = 20
	maximumRecordingDuration      = 20 * time.Minute
	audioHandshakeTimeout         = 5 * time.Second
	audioCapabilityLifetime       = 60 * time.Second
	audioInactivityTimeout        = 10 * time.Second
)

const (
	audioStatusReady byte = iota
	audioStatusStopped
	audioStatusDenied
	audioStatusInvalid
	audioStatusLimit
	audioStatusBusy
	audioStatusFailed
)

var errAudioInput = errors.New("invalid browser audio input")

type audioHandshake struct {
	recording  voice.RecordingKey
	capability [32]byte
}

func hasAudioHeader(frame []byte, frameType byte) bool {
	return len(frame) >= 4 && frame[0] == 'T' && frame[1] == 'A' && frame[2] == audioProtocolVersion && frame[3] == frameType
}

func decodeAudioHandshake(frame []byte) (audioHandshake, error) {
	if len(frame) != audioHandshakeBytes || !hasAudioHeader(frame, audioFrameHandshake) {
		return audioHandshake{}, errAudioInput
	}
	if binary.LittleEndian.Uint32(frame[68:72]) != audioSampleRateHz || frame[72] != 1 || frame[73] != 1 || frame[74] != 0 || frame[75] != 0 {
		return audioHandshake{}, errAudioInput
	}
	handshake := audioHandshake{recording: voice.RecordingKey{
		SessionID:   hex.EncodeToString(frame[4:20]),
		DraftID:     binary.LittleEndian.Uint64(frame[20:28]),
		RecordingID: binary.LittleEndian.Uint64(frame[28:36]),
	}}
	copy(handshake.capability[:], frame[36:68])
	return handshake, nil
}

// Each signed sample occupies two bytes, least significant byte first. For
// example, 00 80 represents -32768, not 128. Actual bytes determine allocation;
// the declared count must agree. See docs/browser-audio.md for the wire contract.
func decodePCMFrame(frame []byte) (uint32, []int16, error) {
	if len(frame) < audioPCMHeaderBytes || len(frame) > maximumAudioMessageBytes || !hasAudioHeader(frame, audioFramePCM) {
		return 0, nil, errAudioInput
	}
	payload := frame[audioPCMHeaderBytes:]
	if len(payload) == 0 || len(payload)%2 != 0 {
		return 0, nil, errAudioInput
	}
	sampleCount := binary.LittleEndian.Uint32(frame[8:12])
	if sampleCount > maximumPCMChunkSamples || uint64(sampleCount)*2 != uint64(len(payload)) {
		return 0, nil, errAudioInput
	}
	pcmSamples := make([]int16, int(sampleCount))
	for sampleIndex := range pcmSamples {
		pcmSamples[sampleIndex] = int16(binary.LittleEndian.Uint16(payload[sampleIndex*2:]))
	}
	return binary.LittleEndian.Uint32(frame[4:8]), pcmSamples, nil
}
