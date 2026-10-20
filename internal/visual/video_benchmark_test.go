package visual

import (
	"bytes"
	"crypto/sha256"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Opt-in one-time import measurement; generate synthetic media before the timed
// loop. Subprocess CPU/RSS is reported separately by the existing OS tools.
func BenchmarkPrepareVideo(b *testing.B) {
	path := os.Getenv("THOUGHTS_VIDEO_BENCH_FIXTURE")
	if path == "" {
		b.Skip("set THOUGHTS_VIDEO_BENCH_FIXTURE to a synthetic SDR clip")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		b.Skip("FFmpeg is not installed")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	quality := VideoQuality(os.Getenv("THOUGHTS_VIDEO_BENCH_QUALITY"))
	if quality == "" {
		quality = VideoQualityEfficient
	}
	if !quality.Valid() {
		b.Fatal(ErrVideoQuality)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		source, err := StageVideo(b.Context(), bytes.NewReader(body))
		if err != nil {
			b.Fatal(err)
		}
		prepared, err := source.Prepare(b.Context(), VideoTrim{EndMS: min(30000, source.Info().DurationMS)}, quality)
		if err != nil {
			source.Close()
			b.Fatal(err)
		}
		b.StopTimer()
		encoded, err := os.ReadFile(filepath.Join(prepared.directory, "display.mp4"))
		if err != nil {
			b.Fatal(err)
		}
		info := prepared.Info()
		b.ReportMetric(float64(len(encoded)), "video-bytes")
		b.ReportMetric(float64(info.Width), "width")
		b.ReportMetric(float64(info.Height), "height")
		b.ReportMetric(float64(info.FrameCount), "frames")
		b.Logf("quality=%s FPS=%s SHA256=%x", quality, info.FrameRate, sha256.Sum256(encoded))
		if directory := os.Getenv("THOUGHTS_VIDEO_BENCH_ARTIFACTS"); directory != "" {
			if err := os.MkdirAll(directory, 0700); err != nil {
				b.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, string(quality)+".mp4"), encoded, 0600); err != nil {
				b.Fatal(err)
			}
			poster, err := os.ReadFile(filepath.Join(prepared.directory, "poster.jpg"))
			if err != nil {
				b.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, string(quality)+".jpg"), poster, 0600); err != nil {
				b.Fatal(err)
			}
		}
		b.StartTimer()
		if err := prepared.Close(); err != nil {
			b.Fatal(err)
		}
		if err := source.Close(); err != nil {
			b.Fatal(err)
		}
	}
}
