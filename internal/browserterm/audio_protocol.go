package browserterm

import (
	"encoding/binary"
	"errors"
	"time"
)

const (
	audioProtocolVersion      = 1
	audioFrameHandshake       = 1
	audioFrameStatus          = 3
	audioHandshakeBytes       = 36
	audioSampleRateHz         = 16_000
	pcmFrameSamples           = audioSampleRateHz / 10
	pcmFrameBytes             = pcmFrameSamples * 2
	maximumAudioMessageBytes  = pcmFrameBytes
	maximumRecordingSamples   = 19_200_000
	maximumPendingAudioChunks = 20
	maximumRecordingDuration  = 20 * time.Minute
	audioHandshakeTimeout     = 5 * time.Second
	audioCapabilityLifetime   = 60 * time.Second
	audioInactivityTimeout    = 10 * time.Second
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

func hasAudioHeader(frame []byte, frameType byte) bool {
	return len(frame) >= 4 && frame[0] == 'T' && frame[1] == 'A' && frame[2] == audioProtocolVersion && frame[3] == frameType
}

func decodeAudioHandshake(frame []byte) ([32]byte, error) {
	if len(frame) != audioHandshakeBytes || !hasAudioHeader(frame, audioFrameHandshake) {
		return [32]byte{}, errAudioInput
	}
	var recordingCapability [32]byte
	copy(recordingCapability[:], frame[4:])
	return recordingCapability, nil
}

// Each signed sample occupies two bytes, least significant byte first. For
// example, 00 80 represents -32768, not 128. Every frame holds exactly 100 ms;
// WebSocket already preserves ordering. See docs/browser-audio.md.
func decodePCMFrame(frame []byte) ([]int16, error) {
	if len(frame) != pcmFrameBytes {
		return nil, errAudioInput
	}
	pcmSamples := make([]int16, pcmFrameSamples)
	for sampleIndex := range pcmSamples {
		pcmSamples[sampleIndex] = int16(binary.LittleEndian.Uint16(frame[sampleIndex*2:]))
	}
	return pcmSamples, nil
}
