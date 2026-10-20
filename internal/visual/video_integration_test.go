package visual

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
)

func syntheticVideo(t *testing.T) []byte {
	t.Helper()
	tool, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("FFmpeg is not installed")
	}
	if _, err = exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe is not installed")
	}
	path := filepath.Join(t.TempDir(), "synthetic.mp4")
	command := exec.CommandContext(t.Context(), tool, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=64x48:rate=24:duration=2", "-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-c:v", "libx264", "-threads", "1", "-pix_fmt", "yuv420p", "-color_primaries", "bt709", "-color_trc", "bt709", "-colorspace", "bt709", "-color_range", "tv", "-c:a", "aac", "-metadata", "comment=PRIVATE-SOURCE-MARKER", "-movflags", "+faststart", "-y", path)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("synthetic fixture: %v, %s", err, output)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestService_Video(t *testing.T) {
	t.Run("stores bounded silent video and sanitized poster before committing selection", func(t *testing.T) {
		store := &memoryStore{}
		service := NewService(store, "user", t.TempDir())
		body := syntheticVideo(t)
		source, err := StageVideo(t.Context(), bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer source.Close()
		prepared, err := source.Prepare(t.Context(), VideoTrim{EndMS: 2000}, VideoQualityEfficient)
		if err != nil {
			t.Fatal(err)
		}
		defer prepared.Close()
		settings, warning, err := service.ApplyPreparedVideo(t.Context(), prepared, 70, data.DefaultBackgroundFraming(), 50, false)
		if err != nil || warning {
			t.Fatalf("import: warning=%v err=%v", warning, err)
		}
		if filepath.Ext(settings.BackgroundAsset) != ".mp4" || settings.SpeedPercent != 50 {
			t.Fatalf("settings: %+v", settings)
		}
		file, kind, err := service.OpenBackground(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		saved, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
		if kind != "video/mp4" || bytes.Contains(saved, []byte("PRIVATE-SOURCE-MARKER")) {
			t.Fatal("source metadata retained or wrong media type")
		}
		poster, kind, err := service.OpenPoster(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		poster.Close()
		if kind != "image/jpeg" {
			t.Fatalf("poster type: %s", kind)
		}
		before := store.settings
		if _, err := StageVideo(t.Context(), bytes.NewReader([]byte("PRIVATE invalid media"))); !errors.Is(err, ErrVideo) {
			t.Fatalf("got %v, want ErrVideo", err)
		}
		if store.settings != before {
			t.Fatal("failed replacement changed selection")
		}
		if _, _, err := service.Remove(t.Context()); err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(service.directory)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Fatalf("remaining assets=%d, want 0", len(entries))
		}
	})
	t.Run("cancelled processing leaves selected image and no staged assets", func(t *testing.T) {
		service := NewService(&memoryStore{}, "user", t.TempDir())
		first, _, err := service.Import(t.Context(), bytes.NewReader(samplePNG(t)), 70, data.DefaultBackgroundFraming())
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := StageVideo(ctx, bytes.NewReader(nil)); !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want cancellation", err)
		}
		current, err := service.Load(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if current != first {
			t.Fatal("cancel changed selection")
		}
	})
}

func TestVideoOrientation(t *testing.T) {
	for _, rotation := range []int{0, 90, 180, 270} {
		t.Run(fmt.Sprintf("normalizes rotation %d into physical dimensions", rotation), func(t *testing.T) {
			body := syntheticVideo(t)
			directory := t.TempDir()
			source := filepath.Join(directory, "source.mp4")
			rotated := filepath.Join(directory, "rotated.mp4")
			if err := os.WriteFile(source, body, 0600); err != nil {
				t.Fatal(err)
			}
			tool, _ := exec.LookPath("ffmpeg")
			command := exec.CommandContext(t.Context(), tool, "-v", "error", "-display_rotation", strconv.Itoa(rotation), "-i", source, "-c", "copy", "-y", rotated)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("rotation fixture: %v %s", err, output)
			}
			input, err := os.Open(rotated)
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			staged, err := StageVideo(t.Context(), input)
			if err != nil {
				t.Fatal(err)
			}
			defer staged.Close()
			prepared, err := staged.Prepare(t.Context(), VideoTrim{EndMS: 2000}, VideoQualityEfficient)
			if err != nil {
				t.Fatal(err)
			}
			defer prepared.Close()
			poster, err := prepared.Open(t.Context(), true)
			if err != nil {
				t.Fatal(err)
			}
			config, _, err := image.DecodeConfig(poster)
			if err != nil {
				t.Fatal(err)
			}
			width, height := 64, 48
			if rotation == 90 || rotation == 270 {
				width, height = 48, 64
			}
			if config.Width != width || config.Height != height {
				t.Fatalf("poster %dx%d, want %dx%d", config.Width, config.Height, width, height)
			}
		})
	}
}
