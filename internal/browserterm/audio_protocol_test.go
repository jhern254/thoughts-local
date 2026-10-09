package browserterm

import (
	"encoding/binary"
	"testing"
)

func TestAudioProtocol(t *testing.T) {
	t.Run("decodes signed little endian samples", func(t *testing.T) {
		frame := []byte{'T', 'A', 1, audioFramePCM, 0, 0, 0, 0, 2, 0, 0, 0, 0, 128, 255, 127}
		sequence, samples, err := decodePCMFrame(frame)
		if err != nil {
			t.Fatal(err)
		}
		if sequence != 0 || len(samples) != 2 || samples[0] != -32768 || samples[1] != 32767 {
			t.Fatalf("unexpected PCM: %v", samples)
		}
	})
	t.Run("rejects malformed and non audio frames", func(t *testing.T) {
		for _, frame := range [][]byte{
			[]byte(`v{"action":"start"}`), []byte("0terminal"), {'T', 'A', 2, 2},
			{'T', 'A', 1, 2, 0, 0, 0, 0, 1, 0, 0, 0, 1},
			{'T', 'A', 1, 2, 0, 0, 0, 0, 2, 0, 0, 0, 1, 0},
			make([]byte, maximumAudioMessageBytes+1),
		} {
			if _, _, err := decodePCMFrame(frame); err == nil {
				t.Fatal("accepted malformed frame")
			}
		}
	})
	t.Run("rejects excessive sample count before allocation", func(t *testing.T) {
		frame := []byte{'T', 'A', 1, audioFramePCM, 0, 0, 0, 0, 0, 0, 0, 0}
		binary.LittleEndian.PutUint32(frame[8:], ^uint32(0))
		if _, _, err := decodePCMFrame(frame); err == nil {
			t.Fatal("accepted excessive count")
		}
	})
}
