package browserterm

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestInputMessages(t *testing.T) {
	t.Run("bounds appearance metadata and rejects unknown actions", func(t *testing.T) {
		for _, frame := range []string{`o{"action":"open"}`, `o{"action":"loaded","id":1,"darkness":70}`} {
			if _, err := inputMessages([]byte(frame)); err != nil {
				t.Fatal(err)
			}
		}
		for _, frame := range []string{`o{"action":"open","id":1}`, `o{"action":"loaded"}`, `o{"action":"saved","id":9007199254740992}`, `o{"action":"saved","id":1,"darkness":96}`, `o{"action":"path"}`, "o" + strings.Repeat(" ", 257)} {
			if _, err := inputMessages([]byte(frame)); err == nil {
				t.Fatal("accepted invalid appearance input")
			}
		}
	})
	t.Run("preserves complete paste for existing validation", func(t *testing.T) {
		for _, text := range []string{"first\n界 👩‍💻", "bad\r\ninput", "bad\tinput", "bad\ufffdinput", "\x1b[201~\x13", ""} {
			got, err := inputMessages(append([]byte{'p'}, []byte(text)...))
			want := []tea.Msg{tea.PasteMsg{Content: text}}
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("paste got %#v, %v; want %#v, nil", got, err, want)
			}
		}
	})
	t.Run("decodes terminal keys with the existing Charm decoder", func(t *testing.T) {
		got, err := inputMessages([]byte("0a\x1b[A\x13\x03"))
		if err != nil {
			t.Fatal(err)
		}
		var keys []string
		for _, msg := range got {
			keys = append(keys, msg.(tea.KeyPressMsg).String())
		}
		if want := []string{"a", "up", "ctrl+s", "ctrl+c"}; !reflect.DeepEqual(keys, want) {
			t.Fatalf("got keys %v, want %v", keys, want)
		}
	})
	t.Run("never invokes the host clipboard", func(t *testing.T) {
		got, err := inputMessages([]byte("0\x16\x1b[2;2~"))
		if err != nil || len(got) != 0 {
			t.Fatalf("got %#v, %v; want no messages", got, err)
		}
	})
	t.Run("bounds input and rejects incomplete or raw bracketed paste", func(t *testing.T) {
		for _, frame := range [][]byte{nil, []byte("xunknown"), {'p', 0xff}, []byte("0\x1b[200~hello"), []byte("0\x1b[999"), []byte("0" + strings.Repeat("a", 4097)), []byte("p" + strings.Repeat("a", 1<<20+1))} {
			if _, err := inputMessages(frame); err == nil {
				t.Fatalf("accepted invalid frame (%d bytes)", len(frame))
			}
		}
	})
	t.Run("accepts only bounded resize messages", func(t *testing.T) {
		got, err := inputMessages([]byte(`2{"cols":120,"rows":40}`))
		if err != nil || !reflect.DeepEqual(got, []tea.Msg{tea.WindowSizeMsg{Width: 120, Height: 40}}) {
			t.Fatalf("got %#v, %v", got, err)
		}
		for _, size := range []string{`{"cols":0,"rows":24}`, `{"cols":513,"rows":24}`, `{"cols":80,"rows":129}`, `{"cols":80,"rows":-1}`, `{}`} {
			if _, err := inputMessages([]byte("2" + size)); err == nil {
				t.Fatalf("accepted size %s", size)
			}
		}
	})
}
