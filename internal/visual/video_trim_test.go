package visual

import (
	"bytes"
	"errors"
	"image"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
)

func TestVideoTrim_Valid(t *testing.T) {
	for _, test := range []struct {
		name     string
		trim     VideoTrim
		duration int64
		valid    bool
	}{
		{"accepts a late interval in a two hour source", VideoTrim{7170000, 7200000}, 7200000, true},
		{"accepts one second", VideoTrim{1250, 2250}, 60000, true},
		{"rejects negative start", VideoTrim{-1, 1000}, 60000, false},
		{"rejects reversed range", VideoTrim{2000, 1000}, 60000, false},
		{"rejects less than one second", VideoTrim{0, 999}, 60000, false},
		{"rejects more than thirty seconds", VideoTrim{0, 30001}, 60000, false},
		{"rejects past the source", VideoTrim{29000, 31000}, 30000, false},
		{"rejects overflowing timestamps", VideoTrim{math.MinInt64, math.MaxInt64}, 120000, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.trim.Valid(test.duration); got != test.valid {
				t.Fatalf("valid = %v, want %v", got, test.valid)
			}
		})
	}
}

func TestVideoSource_Prepare(t *testing.T) {
	t.Run("keeps a late non-keyframe cut and applies identical validated bytes", func(t *testing.T) {
		tool, err := exec.LookPath("ffmpeg")
		if err != nil {
			t.Skip("FFmpeg not installed")
		}
		if _, err := exec.LookPath("ffprobe"); err != nil {
			t.Skip("ffprobe not installed")
		}
		path := filepath.Join(t.TempDir(), "source.mp4")
		command := exec.CommandContext(t.Context(), tool, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=64x48:rate=24:duration=60", "-c:v", "libx264", "-threads", "1", "-g", "120", "-pix_fmt", "yuv420p", "-movflags", "+faststart", path)
		if err := command.Run(); err != nil {
			t.Fatal(err)
		}
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		source, err := StageVideo(t.Context(), file)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
		defer source.Close()
		if got := source.Info().DurationMS; got != 60000 {
			t.Fatalf("duration = %d, want 60000", got)
		}
		prepared, err := source.Prepare(t.Context(), VideoTrim{StartMS: 57250, EndMS: 59500}, VideoQualityEfficient)
		if err != nil {
			t.Fatal(err)
		}
		defer prepared.Close()
		if got := prepared.Info().DurationMS; got != 2250 {
			t.Fatalf("prepared duration = %d, want 2250", got)
		}
		video, err := prepared.Open(t.Context(), false)
		if err != nil {
			t.Fatal(err)
		}
		want, err := io.ReadAll(video)
		video.Close()
		if err != nil {
			t.Fatal(err)
		}
		poster, err := prepared.Open(t.Context(), true)
		if err != nil {
			t.Fatal(err)
		}
		wantPoster, err := io.ReadAll(poster)
		poster.Close()
		if err != nil {
			t.Fatal(err)
		}
		// Applying must not invoke either tool or interpret the source again.
		source.ffmpeg = "unavailable"
		source.ffprobe = "unavailable"
		store := &memoryStore{}
		service := NewService(store, "user", t.TempDir())
		if _, warning, err := service.ApplyPreparedVideo(t.Context(), prepared, 70, data.DefaultBackgroundFraming(), 25, false); err != nil || warning {
			t.Fatalf("apply = %v, warning %v", err, warning)
		}
		saved, _, err := service.OpenBackground(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(saved)
		saved.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatal("applied video differs from prepared preview")
		}
		savedPoster, _, err := service.OpenPoster(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		gotPoster, err := io.ReadAll(savedPoster)
		savedPoster.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(gotPoster, wantPoster) {
			t.Fatal("applied poster differs from preview poster")
		}
	})
}

func TestVideoSource_CutTiming(t *testing.T) {
	for _, offset := range []string{"0", "5"} {
		t.Run("normalizes source timestamp offset "+offset, func(t *testing.T) {
			tool, err := exec.LookPath("ffmpeg")
			if err != nil {
				t.Skip("FFmpeg not installed")
			}
			if _, err := exec.LookPath("ffprobe"); err != nil {
				t.Skip("ffprobe not installed")
			}
			directory := t.TempDir()
			path := filepath.Join(directory, "source.mp4")
			// Each source second changes brightness. The GOP spans five seconds, so
			// 6.25 is deliberately not a keyframe. Output must start at the cut rather
			// than at that GOP's keyframe or at the container's nonzero timestamp.
			command := exec.CommandContext(t.Context(), tool, "-v", "error", "-f", "lavfi", "-i", "nullsrc=size=64x48:rate=24:duration=12,geq=lum='32+12*T':cb=128:cr=128", "-c:v", "libx264", "-threads", "1", "-g", "120", "-pix_fmt", "yuv420p", "-output_ts_offset", offset, "-movflags", "+faststart", path)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("fixture: %v %s", err, output)
			}
			file, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			source, err := StageVideo(t.Context(), file)
			file.Close()
			if err != nil {
				t.Fatal(err)
			}
			defer source.Close()
			prepared, err := source.Prepare(t.Context(), VideoTrim{StartMS: 6250, EndMS: 7500}, VideoQualityEfficient)
			if err != nil {
				t.Fatal(err)
			}
			defer prepared.Close()
			if got := prepared.Info().DurationMS; got != 1250 {
				t.Fatalf("duration = %d, want 1250", got)
			}
			poster, err := prepared.Open(t.Context(), true)
			if err != nil {
				t.Fatal(err)
			}
			decoded, _, err := image.Decode(poster)
			poster.Close()
			if err != nil {
				t.Fatal(err)
			}
			r, _, _, _ := decoded.At(32, 24).RGBA()
			// Limited-range Y=107 maps to sRGB roughly 106 after the Rec.709/JPEG
			// path. Allow codec rounding; a preceding GOP or PTS mistake is >14 levels.
			if got := int(r >> 8); got < 100 || got > 112 {
				t.Fatalf("first-frame brightness=%d, want 100–112 at 6.25s", got)
			}
		})
	}
	t.Run("keeps fractional-frame-rate loops within one frame of the requested span", func(t *testing.T) {
		tool, err := exec.LookPath("ffmpeg")
		if err != nil {
			t.Skip("FFmpeg not installed")
		}
		path := filepath.Join(t.TempDir(), "source.mp4")
		command := exec.CommandContext(t.Context(), tool, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=64x48:rate=24000/1001:duration=32", "-c:v", "libx264", "-threads", "1", "-pix_fmt", "yuv420p", "-movflags", "+faststart", path)
		if err := command.Run(); err != nil {
			t.Fatal(err)
		}
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		source, err := StageVideo(t.Context(), file)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
		defer source.Close()
		for _, trim := range []VideoTrim{{StartMS: 1250, EndMS: 2250}, {StartMS: 1250, EndMS: 31250}} {
			loop, err := source.Prepare(t.Context(), trim, VideoQualityEfficient)
			if err != nil {
				t.Fatal(err)
			}
			got := loop.Info().DurationMS
			loop.Close()
			if math.Abs(float64(got-(trim.EndMS-trim.StartMS))) > 42 || got < 1000 || got > 30000 {
				t.Fatalf("%+v duration=%d, want within one frame and 1–30s", trim, got)
			}
		}
	})
}

func TestVideoProbe_ValidateLoop(t *testing.T) {
	t.Run("rejects incomplete output even when codec and absolute duration limits pass", func(t *testing.T) {
		info := videoProbe{Streams: []videoStream{{
			CodecName:         "h264",
			CodecType:         "video",
			Profile:           "Constrained Baseline",
			Level:             32,
			Refs:              1,
			FrameCount:        "24",
			Width:             1280,
			Height:            720,
			PixFmt:            "yuv420p",
			AvgFrameRate:      "24/1",
			SampleAspectRatio: "1:1",
			Duration:          "1",
			ColorSpace:        "bt709",
			ColorTransfer:     "bt709",
			ColorPrimaries:    "bt709",
			ColorRange:        "tv",
		}}}
		if err := info.validate(true); err != nil {
			t.Fatal(err)
		}
		plan, err := planVideoOutput(info.Streams[0], VideoTrim{EndMS: 2000}, VideoQualityMaximum)
		if err != nil {
			t.Fatal(err)
		}
		if err := info.validateLoop(VideoTrim{EndMS: 2000}, plan); !errors.Is(err, ErrVideo) {
			t.Fatalf("partial output = %v, want ErrVideo", err)
		}
		plan, err = planVideoOutput(info.Streams[0], VideoTrim{EndMS: 1000}, VideoQualityMaximum)
		if err != nil {
			t.Fatal(err)
		}
		if err := info.validateLoop(VideoTrim{EndMS: 1000}, plan); err != nil {
			t.Fatal(err)
		}
	})
}

func TestVideoSource_FragmentedMP4(t *testing.T) {
	t.Run("prepares a late cut from self contained fragmented H264", func(t *testing.T) {
		ffmpeg, err := exec.LookPath("ffmpeg")
		if err != nil {
			t.Skip("requires FFmpeg")
		}
		if _, err := exec.LookPath("ffprobe"); err != nil {
			t.Skip("requires ffprobe")
		}
		sourcePath := filepath.Join(t.TempDir(), "fragmented.mp4")
		command := exec.CommandContext(t.Context(), ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=96x64:rate=30000/1001:duration=32", "-c:v", "libx264", "-threads", "1", "-g", "60", "-pix_fmt", "yuv420p", "-movflags", "+frag_keyframe+empty_moov+default_base_moof+global_sidx", sourcePath)
		if err := command.Run(); err != nil {
			t.Fatal(err)
		}
		input, err := os.Open(sourcePath)
		if err != nil {
			t.Fatal(err)
		}
		defer input.Close()
		source, err := StageVideo(t.Context(), input)
		if err != nil {
			t.Fatalf("stage fragmented MP4: %v", err)
		}
		defer source.Close()
		trim := VideoTrim{StartMS: 10000, EndMS: 26000}
		prepared, err := source.Prepare(t.Context(), trim, VideoQualityHigh)
		if err != nil {
			t.Fatalf("prepare sixteen-second cut: %v", err)
		}
		defer prepared.Close()
		if prepared.Info().Trim != trim || math.Abs(float64(prepared.Info().DurationMS)-16000) > 34 {
			t.Fatalf("prepared interval = %+v, want sixteen seconds at the requested cut", prepared.Info())
		}
		output, err := prepared.Open(t.Context(), false)
		if err != nil {
			t.Fatal(err)
		}
		defer output.Close()
		stat, err := output.Stat()
		if err != nil {
			t.Fatal(err)
		}
		if err := inspectVideoContainer(output, stat.Size(), true); err != nil {
			t.Fatalf("prepared output must retain its independent nonfragmented contract: %v", err)
		}
	})
}
