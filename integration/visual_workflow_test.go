//go:build integration

package integration_test

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/data"
)

func TestVisualWorkflow_SQLite(t *testing.T) {
	t.Run("opens the selected image from the application asset directory", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		runtime, err := application.Open(t.Context(), dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer runtime.Close()
		var filename string
		if err := db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&filename); err != nil {
			t.Fatal(err)
		}
		directory := filepath.Join(filename+".assets", "appearance")
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		const asset = "0123456789abcdef0123456789abcdef.png"
		var body bytes.Buffer
		if err := png.Encode(&body, image.NewRGBA(image.Rect(0, 0, 8, 6))); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, asset), body.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO visual
 (user_id, background_asset, darkness, fit, zoom, position_x, position_y)
 VALUES (?, ?, 42, 'fit', 175, 1200, 9000)`, runtime.LocalUser().UserID, asset); err != nil {
			t.Fatal(err)
		}
		service, err := runtime.BrowserVisual(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		settings, err := service.Load(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		wantSettings := data.Visual{
			BackgroundAsset: asset,
			Darkness:        42,
			Framing: data.BackgroundFraming{
				Fit:       "fit",
				Zoom:      175,
				PositionX: 1200,
				PositionY: 9000,
			},
		}
		if settings != wantSettings {
			t.Fatalf("settings = %+v, want %+v", settings, wantSettings)
		}
		file, _, err := service.OpenBackground(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		loaded, err := io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(loaded, body.Bytes()) {
			t.Fatal("loaded image differs from the existing background")
		}
	})

	t.Run("restores native image and darkness after application restart", func(t *testing.T) {
		_, dsn := openMigratedSQLite(t)
		runtime, err := application.Open(t.Context(), dsn)
		if err != nil {
			t.Fatal(err)
		}
		service, err := runtime.BrowserVisual(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		var body bytes.Buffer
		_ = png.Encode(&body, image.NewRGBA(image.Rect(0, 0, 8, 6)))
		framing := data.BackgroundFraming{Fit: "fill", Zoom: 175, PositionX: 1200, PositionY: 9000}
		first, warning, err := service.Import(t.Context(), bytes.NewReader(body.Bytes()), 42, framing)
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
		service, err = runtime.BrowserVisual(t.Context())
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
		if _, _, err = service.Import(t.Context(), bytes.NewReader([]byte("invalid replacement")), 10, data.DefaultBackgroundFraming()); err == nil {
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
