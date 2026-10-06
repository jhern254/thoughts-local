package browserterm

import (
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
	"testing"
)

func TestInputMessages_Voice(t *testing.T) {
	t.Run("accepts bounded recording metadata", func(t *testing.T) {
		msgs, err := inputMessages([]byte(`v{"action":"recording","draftID":2,"recordingID":3}`))
		if err != nil {
			t.Fatal(err)
		}
		got, ok := msgs[0].(thoughts.VoiceAction)
		if !ok || got.Action != "recording" || got.DraftID != 2 || got.RecordingID != 3 {
			t.Fatalf("got %+v, want recording 2/3", msgs)
		}
	})
	for _, frame := range []string{`v{"action":"audio","draftID":2}`, `v{"action":"open","draftID":2}`, `v{"action":"start","draftID":-1}`, `v{"action":"stopped","draftID":0}`, `v{"action":"failed","draftID":9007199254740992}`, `v{`, "v" + string(make([]byte, 257))} {
		t.Run("rejects "+frame[:min(len(frame), 30)], func(t *testing.T) {
			if _, err := inputMessages([]byte(frame)); err == nil {
				t.Fatal("accepted invalid voice control")
			}
		})
	}
}
