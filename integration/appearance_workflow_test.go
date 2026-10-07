//go:build integration

package integration_test

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"testing"

	"github.com/jhern254/go-thoughts/internal/application"
)

func TestAppearanceWorkflow_SQLite(t *testing.T) {
	t.Run("restores native image and darkness after application restart", func(t *testing.T) {
		_, dsn := openMigratedSQLite(t)
		runtime, err := application.Open(t.Context(), dsn)
		if err != nil {
			t.Fatal(err)
		}
		service, err := runtime.BrowserAppearance(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		var body bytes.Buffer
		_ = png.Encode(&body, image.NewRGBA(image.Rect(0, 0, 8, 6)))
		first, warning, err := service.Import(t.Context(), bytes.NewReader(body.Bytes()), 42)
		if err != nil || warning {
			t.Fatalf("import = %v, warning %v", err, warning)
		}
		if err = runtime.Close(); err != nil {
			t.Fatal(err)
		}
		runtime, err = application.Open(t.Context(), dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer runtime.Close()
		service, err = runtime.BrowserAppearance(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		got, err := service.Load(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if got != first {
			t.Fatalf("restored = %+v, want %+v", got, first)
		}
		if _, _, err = service.Import(t.Context(), bytes.NewReader([]byte("invalid replacement")), 10); err == nil {
			t.Fatal("invalid replacement accepted")
		}
		file, _, err := service.OpenBackground(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(loaded, body.Bytes()) {
			t.Fatal("restored image differs from imported image")
		}
		if _, _, err = service.Remove(t.Context()); err != nil {
			t.Fatal(err)
		}
		got, err = service.Load(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if got.BackgroundAsset != "" || got.Darkness != 42 {
			t.Fatalf("removed settings = %+v", got)
		}
	})
}
