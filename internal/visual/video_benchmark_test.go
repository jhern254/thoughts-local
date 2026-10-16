package visual

import (
	"bytes"
	"os"
	"os/exec"
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
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, _, err := prepareVideo(b.Context(), bytes.NewReader(body)); err != nil {
			b.Fatal(err)
		}
	}
}
