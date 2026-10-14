//go:build integration

package integration_test

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/data"
)

func TestVisualWorkflow_SQLite(t *testing.T) {
	t.Run("restores prepared video poster and deliberate playback preferences after restart", func(t *testing.T) {
		if _, err := exec.LookPath("ffmpeg"); err != nil {
			t.Skip("FFmpeg is not installed")
		}
		if _, err := exec.LookPath("ffprobe"); err != nil {
			t.Skip("ffprobe is not installed")
		}
		_, dsn := openMigratedSQLite(t)
		runtime, err := application.Open(t.Context(), dsn)
		if err != nil {
			t.Fatal(err)
		}
		service, err := runtime.BrowserVisual(t.Context())
		if err != nil {
			runtime.Close()
			t.Fatal(err)
		}
		source, err := os.Open("../internal/browserterm/client/fixtures/visual-video.mp4")
		if err != nil {
			runtime.Close()
			t.Fatal(err)
		}
		first, warning, err := service.ImportVideo(t.Context(), source, 42, data.DefaultBackgroundFraming(), 50, true)
		source.Close()
		if err != nil || warning {
			runtime.Close()
			t.Fatalf("video import: %v warning=%v", err, warning)
		}
		if err := runtime.Close(); err != nil {
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
		restored, err := service.Load(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if restored != first || restored.SpeedPercent != 50 || !restored.UserPaused {
			t.Fatalf("restored=%+v, want %+v", restored, first)
		}
		poster, kind, err := service.OpenPoster(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer poster.Close()
		config, _, err := image.DecodeConfig(poster)
		if err != nil {
			t.Fatal(err)
		}
		if kind != "image/jpeg" || config.Width != 96 || config.Height != 64 {
			t.Fatalf("poster=%s %dx%d, want JPEG 96x64", kind, config.Width, config.Height)
		}
	})
	t.Run("restores optimized background and framing after restart", func(t *testing.T) {
		_, dsn := openMigratedSQLite(t)
		runtime, err := application.Open(t.Context(), dsn)
		if err != nil {
			t.Fatal(err)
		}
		service, err := runtime.BrowserVisual(t.Context())
		if err != nil {
			runtime.Close()
			t.Fatal(err)
		}
		var source bytes.Buffer
		if err := png.Encode(&source, image.NewNRGBA(image.Rect(0, 0, 6000, 4000))); err != nil {
			runtime.Close()
			t.Fatal(err)
		}
		framing := data.BackgroundFraming{Fit: "fill", Zoom: 175, PositionX: 1200, PositionY: 9000}
		first, warning, err := service.Import(t.Context(), bytes.NewReader(source.Bytes()), 42, framing)
		if err != nil || warning {
			runtime.Close()
			t.Fatalf("import = %v, warning %v; want success", err, warning)
		}
		if err := runtime.Close(); err != nil {
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
		if err != nil || got != first {
			t.Fatalf("restored = %+v, %v; want %+v", got, err, first)
		}
		file, _, err := service.OpenBackground(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		config, format, err := image.DecodeConfig(file)
		if err != nil || format != "png" || config.Width != 2449 || config.Height != 1632 {
			t.Fatalf("stored = %+v, %s, %v; want 2449x1632 PNG", config, format, err)
		}
	})
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
			SpeedPercent:    100,
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
		importedFile, _, err := service.OpenBackground(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		importedBytes, err := io.ReadAll(importedFile)
		importedFile.Close()
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := png.Decode(bytes.NewReader(importedBytes))
		if err != nil || decoded.Bounds().Size() != (image.Point{8, 6}) {
			t.Fatalf("imported image = %v, %v", decoded, err)
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
		if !bytes.Equal(loaded, importedBytes) {
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
