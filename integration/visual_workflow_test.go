//go:build integration

package integration_test

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/visual"
)

func TestVisualWorkflow_SQLite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Run("stores new imports in Documents Thoughts and keeps database selections independent", func(t *testing.T) {
		_, firstDSN := openMigratedSQLite(t)
		first, err := application.Open(t.Context(), firstDSN)
		if err != nil {
			t.Fatal(err)
		}
		defer first.Close()
		firstVisual, err := first.BrowserVisual(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		var source bytes.Buffer
		if err := png.Encode(&source, image.NewRGBA(image.Rect(0, 0, 12, 8))); err != nil {
			t.Fatal(err)
		}
		selected, warning, err := firstVisual.Import(t.Context(), bytes.NewReader(source.Bytes()), 42, data.DefaultBackgroundFraming())
		if err != nil || warning {
			t.Fatalf("import error = %v, warning = %v; want success", err, warning)
		}
		wantPath := filepath.Join(home, "Documents", "Thoughts", "backgrounds", selected.BackgroundAsset)
		if _, err := os.Stat(wantPath); err != nil {
			t.Fatalf("imported asset in shared application folder: %v", err)
		}
		_, secondDSN := openMigratedSQLite(t)
		second, err := application.Open(t.Context(), secondDSN)
		if err != nil {
			t.Fatal(err)
		}
		defer second.Close()
		secondVisual, err := second.BrowserVisual(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		settings, err := secondVisual.Load(t.Context())
		if err != nil || settings.BackgroundAsset != "" {
			t.Fatalf("second selection = %q, error = %v; want empty", settings.BackgroundAsset, err)
		}
		if file, _, err := secondVisual.OpenBackground(t.Context()); !errors.Is(err, os.ErrNotExist) {
			if file != nil {
				file.Close()
			}
			t.Fatalf("unselected background error = %v, want not exist", err)
		}
		other, warning, err := secondVisual.Import(t.Context(), bytes.NewReader(source.Bytes()), 70, data.DefaultBackgroundFraming())
		if err != nil || warning {
			t.Fatalf("second import error = %v, warning = %v; want success", err, warning)
		}
		if selected.BackgroundAsset == other.BackgroundAsset {
			t.Fatal("independent imports share an asset identifier")
		}
		if _, warning, err := secondVisual.Remove(t.Context()); err != nil || warning {
			t.Fatalf("second remove error = %v, warning = %v; want success", err, warning)
		}
		file, _, err := firstVisual.OpenBackground(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		file.Close()
	})
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
		staged, err := visual.StageVideo(t.Context(), source)
		source.Close()
		if err != nil {
			runtime.Close()
			t.Fatal(err)
		}
		defer staged.Close()
		prepared, err := staged.Prepare(t.Context(), visual.VideoTrim{EndMS: 2000}, visual.VideoQualityEfficient)
		if err != nil {
			runtime.Close()
			t.Fatal(err)
		}
		defer prepared.Close()
		first, warning, err := service.ApplyPreparedVideo(t.Context(), prepared, 42, data.DefaultBackgroundFraming(), 50, true)
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
