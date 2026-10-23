package browserterm

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/visual"
)

func TestVisual_Accessibility(t *testing.T) {
	for _, scenario := range []struct {
		name, body string
		fail       bool
		status     int
	}{
		{name: "saves both flags", body: `{"borderWeight":"heavy","heavierText":false,"textSize":"standard","highContrast":false}`, status: 200},
		{name: "requires both flags", body: `{"heavierText":true,"textSize":"standard","highContrast":false}`, status: 400},
		{name: "rejects null", body: `{"borderWeight":null,"heavierText":true,"textSize":"standard","highContrast":false}`, status: 400},
		{name: "rejects unknown fields", body: `{"borderWeight":"heavy","heavierText":true,"textSize":"standard","highContrast":false,"other":true}`, status: 400},
		{name: "rejects trailing JSON", body: `{"borderWeight":"heavy","heavierText":true,"textSize":"standard","highContrast":false}{}`, status: 400},
		{name: "rejects unsupported border", body: `{"borderWeight":"wide","heavierText":false,"textSize":"standard","highContrast":false}`, status: 400},
		{name: "rejects unsupported text size", body: `{"borderWeight":"heavy","heavierText":false,"textSize":"huge","highContrast":false}`, status: 400},
		{name: "requires contrast", body: `{"borderWeight":"heavy","heavierText":false,"textSize":"standard"}`, status: 400},
		{name: "save failure", body: `{"borderWeight":"heavy","heavierText":true,"textSize":"standard","highContrast":false}`, fail: true, status: 500},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store := &visualStore{
				settings: data.Visual{
					Darkness:     42,
					Framing:      data.DefaultBackgroundFraming(),
					SpeedPercent: 100,
					VideoQuality: data.DefaultVideoQuality,
				},
				fail: scenario.fail,
			}
			s := &server{authority: "127.0.0.1:7777", visual: visual.NewService(store, "u", t.TempDir())}
			r := httptest.NewRequest("PUT", "http://127.0.0.1:7777/visual/accessibility", strings.NewReader(scenario.body))
			r.Header.Set("Origin", "http://127.0.0.1:7777")
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != scenario.status {
				t.Fatalf("got status %d body %q, want %d", w.Code, w.Body.String(), scenario.status)
			}
			if strings.Contains(w.Body.String(), "PRIVATE") {
				t.Fatal("response exposed raw error")
			}
			if w.Code == 200 && (store.settings.BorderWeight != data.BorderHeavy || store.settings.HeavierText || store.settings.Darkness != 42) {
				t.Fatalf("got settings %+v, want independent flags and unchanged darkness", store.settings)
			}
		})
	}
}
