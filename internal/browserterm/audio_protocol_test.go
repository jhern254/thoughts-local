package browserterm

import (
	"bytes"
	"testing"
)

func TestAudioProtocol(t *testing.T) {
	t.Run("handshake contains only the one use capability", func(t *testing.T) {
		frame := append([]byte{'T', 'A', 1, audioFrameHandshake}, bytes.Repeat([]byte{85}, 32)...)
		handshake, err := decodeAudioHandshake(frame)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(handshake[:], frame[4:]) {
			t.Fatal("capability bytes changed")
		}
	})
	t.Run("decodes one fixed frame of signed little endian samples", func(t *testing.T) {
		frame := make([]byte, 3200)
		copy(frame, []byte{0, 128, 255, 127})
		samples, err := decodePCMFrame(frame)
		if err != nil {
			t.Fatal(err)
		}
		if len(samples) != 1600 || samples[0] != -32768 || samples[1] != 32767 {
			t.Fatal("incorrect PCM conversion")
		}
	})
	t.Run("rejects every other frame length before allocating samples", func(t *testing.T) {
		for _, sizeBytes := range []int{0, 1, 3199, 3201, 6400, 8193} {
			samples, err := decodePCMFrame(make([]byte, sizeBytes))
			if err == nil || samples != nil {
				t.Fatalf("accepted frame length %d", sizeBytes)
			}
		}
	})
	t.Run("rejects terminal control and malformed handshakes", func(t *testing.T) {
		for _, frame := range [][]byte{[]byte(`v{"action":"start"}`), []byte("0terminal"), make([]byte, 36), {'T', 'A', 2, 1}, {'T', 'A', 1, 3}} {
			if _, err := decodeAudioHandshake(frame); err == nil {
				t.Fatal("accepted malformed handshake")
			}
		}
	})
}
