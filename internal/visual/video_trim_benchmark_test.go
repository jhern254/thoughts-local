package visual

import (
	"bytes"
	"os"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
)

// Opt-in measurements keep fixture creation outside the clock. Staging is paid
// once per upload; each explicit Preview pays preparation; Apply copies only the
// already validated loop. No processor work occurs during cut-point edits.
func BenchmarkVideoTrim(b *testing.B) {
	path := os.Getenv("THOUGHTS_VIDEO_BENCH_FIXTURE")
	if path == "" {
		b.Skip("set THOUGHTS_VIDEO_BENCH_FIXTURE to a synthetic 60–120s SDR clip")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("Stage", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			source, err := StageVideo(b.Context(), bytes.NewReader(body))
			if err != nil {
				b.Fatal(err)
			}
			if err := source.Close(); err != nil {
				b.Fatal(err)
			}
		}
	})
	source, err := StageVideo(b.Context(), bytes.NewReader(body))
	if err != nil {
		b.Fatal(err)
	}
	defer source.Close()
	duration := source.Info().DurationMS
	if duration < 30000 {
		b.Fatal("trim benchmark needs at least 30s")
	}
	for _, cut := range []struct {
		name  string
		start int64
	}{{"First", 0}, {"Middle", duration / 2}, {"End", duration - 10000}} {
		b.Run("Preview/"+cut.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				loop, err := source.Prepare(b.Context(), VideoTrim{StartMS: cut.start, EndMS: cut.start + 10000}, VideoQualityEfficient)
				if err != nil {
					b.Fatal(err)
				}
				if err := loop.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
	loop, err := source.Prepare(b.Context(), VideoTrim{EndMS: 10000}, VideoQualityEfficient)
	if err != nil {
		b.Fatal(err)
	}
	defer loop.Close()
	service := NewService(&memoryStore{}, "user", b.TempDir())
	b.Run("Apply", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			if _, warning, err := service.ApplyPreparedVideo(b.Context(), loop, 70, data.DefaultBackgroundFraming(), 100, false); err != nil || warning {
				b.Fatalf("apply: %v warning=%v", err, warning)
			}
		}
	})
}
