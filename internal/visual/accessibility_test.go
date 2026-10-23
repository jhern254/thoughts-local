package visual

import (
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
)

func TestService_UpdateAccessibility(t *testing.T) {
	t.Run("saves independent preferences without changing background settings", func(t *testing.T) {
		store := &memoryStore{settings: data.Visual{
			Darkness:     42,
			Framing:      data.DefaultBackgroundFraming(),
			SpeedPercent: 50,
			VideoQuality: data.DefaultVideoQuality,
		}}
		service := NewService(store, "u", t.TempDir())
		before := store.settings
		for _, flags := range [][2]bool{{true, false}, {false, true}, {true, true}, {false, false}} {
			got, err := service.UpdateAccessibility(t.Context(), flags[0], flags[1])
			if err != nil {
				t.Fatal(err)
			}
			want := before
			want.ThickerCardBorders, want.HeavierText = flags[0], flags[1]
			if got != want || store.settings != want {
				t.Fatalf("got %+v, want %+v", got, want)
			}
		}
	})
	t.Run("returns save errors without publishing preferences", func(t *testing.T) {
		store := &memoryStore{fail: true}
		service := NewService(store, "u", t.TempDir())
		if _, err := service.UpdateAccessibility(t.Context(), true, true); err == nil {
			t.Fatal("got nil error, want save failure")
		}
		if store.settings.ThickerCardBorders || store.settings.HeavierText {
			t.Fatal("failed save changed preferences")
		}
	})
}
