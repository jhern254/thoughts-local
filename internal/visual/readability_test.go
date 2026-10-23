package visual

import (
	"github.com/jhern254/go-thoughts/internal/data"
	"testing"
)

func TestService_Readability(t *testing.T) {
	t.Run("rejects invalid preferences before saving", func(t *testing.T) {
		for _, settings := range []data.AccessibilityPreferences{
			{BorderWeight: "unknown", TextSize: data.TextStandard},
			{BorderWeight: data.BorderNormal, TextSize: "huge"},
		} {
			store := &memoryStore{}
			service := NewService(store, "u", t.TempDir())
			if _, err := service.UpdateAccessibility(t.Context(), settings); err == nil {
				t.Fatalf("accepted %+v", settings)
			}
		}
	})
	t.Run("saves every supported border and text size without changing media", func(t *testing.T) {
		store := &memoryStore{settings: data.Visual{Darkness: 42, SpeedPercent: 50, VideoQuality: data.DefaultVideoQuality, Framing: data.DefaultBackgroundFraming()}}
		service := NewService(store, "u", t.TempDir())
		for _, border := range []data.BorderWeight{data.BorderNormal, data.BorderHeavy, data.BorderExtraHeavy} {
			for _, size := range []data.TextSize{data.TextStandard, data.TextLarger, data.TextLargest} {
				got, err := service.UpdateAccessibility(t.Context(), data.AccessibilityPreferences{BorderWeight: border, TextSize: size, HeavierText: true, HighContrast: true})
				if err != nil {
					t.Fatal(err)
				}
				if got.BorderWeight != border || got.TextSize != size || !got.HeavierText || !got.HighContrast || got.Darkness != 42 || got.SpeedPercent != 50 {
					t.Fatalf("unexpected settings %+v", got)
				}
			}
		}
	})
}
