package visual

import (
	"bytes"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestVideoQuality_EncodedStreams(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("video tools not installed")
		}
	}
	cases := []struct {
		name, size, rate     string
		quality              VideoQuality
		width, height, level int
	}{
		{"480p24", "854x480", "24", VideoQualityEfficient, 854, 480, 31},
		{"720p30", "1280x720", "30", VideoQualityEfficient, 1280, 720, 31},
		{"720p60", "1280x720", "60", VideoQualityMaximum, 1280, 720, 32},
		{"900p25", "1600x900", "25", VideoQualityHigh, 1600, 900, 40},
		{"1080p29.97", "1920x1080", "30000/1001", VideoQualityHigh, 1920, 1080, 40},
		{"1080p59.94", "1920x1080", "60000/1001", VideoQualityMaximum, 1920, 1080, 42},
		{"1080p60", "1920x1080", "60", VideoQualityMaximum, 1920, 1080, 42},
		{"portrait", "1080x1920", "30", VideoQualityHigh, 1080, 1920, 40},
		{"efficient ceiling", "1920x1080", "60", VideoQualityEfficient, 1280, 720, 31},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "source.mp4")
			command := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size="+tc.size+":rate="+tc.rate+":duration=1.25", "-c:v", "libx264", "-threads", "1", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-color_primaries", "bt709", "-color_trc", "bt709", "-colorspace", "bt709", "-color_range", "tv", "-movflags", "+faststart", "-y", path)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("fixture: %v %s", err, output)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source, err := StageVideo(t.Context(), bytes.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			defer source.Close()
			prepared, err := source.Prepare(t.Context(), VideoTrim{EndMS: 1000}, tc.quality)
			if err != nil {
				t.Fatal(err)
			}
			defer prepared.Close()
			info, err := probeVideo(t.Context(), "ffprobe", filepath.Join(prepared.directory, "display.mp4"))
			if err != nil {
				t.Fatal(err)
			}
			stream, ok := info.video()
			if !ok {
				t.Fatal("missing video stream")
			}
			if stream.Width != tc.width || stream.Height != tc.height || stream.Level != tc.level || stream.Profile != "Constrained Baseline" || stream.Refs != 1 {
				t.Fatalf("stream=%+v, want %dx%d baseline level %d refs=1", stream, tc.width, tc.height, tc.level)
			}
			expectedRate := tc.rate
			if tc.name == "efficient ceiling" {
				expectedRate = "30"
			}
			if !bytes.Contains([]byte(expectedRate), []byte("/")) {
				expectedRate += "/1"
			}
			if stream.AvgFrameRate != expectedRate {
				t.Fatalf("FPS=%s, want %s", stream.AvgFrameRate, expectedRate)
			}
			descriptor := prepared.Info()
			if descriptor.VideoQuality != tc.quality || descriptor.Width != tc.width || descriptor.Height != tc.height || descriptor.FrameRate.String() != expectedRate {
				t.Fatalf("descriptor=%+v", descriptor)
			}
			poster, err := prepared.Open(t.Context(), true)
			if err != nil {
				t.Fatal(err)
			}
			defer poster.Close()
			geometry, _, err := image.DecodeConfig(poster)
			if err != nil {
				t.Fatal(err)
			}
			if geometry.Width != tc.width || geometry.Height != tc.height {
				t.Fatalf("poster=%dx%d, want %dx%d", geometry.Width, geometry.Height, tc.width, tc.height)
			}
		})
	}
}

func TestVideoQuality_LongSource(t *testing.T) {
	t.Run("stages two minute 1080p60 source and prepares late short interval", func(t *testing.T) {
		for _, tool := range []string{"ffmpeg", "ffprobe"} {
			if _, err := exec.LookPath(tool); err != nil {
				t.Skip("video tools not installed")
			}
		}
		directory := t.TempDir()
		segment := filepath.Join(directory, "segment.mp4")
		path := filepath.Join(directory, "source.mp4")
		args := []string{"-v", "error", "-f", "lavfi", "-i", "color=c=blue:s=1920x1080:r=60:d=2", "-c:v", "libx264", "-threads", "1", "-preset", "ultrafast", "-g", "120", "-pix_fmt", "yuv420p", "-movflags", "+faststart", "-y", segment}
		if output, err := exec.CommandContext(t.Context(), "ffmpeg", args...).CombinedOutput(); err != nil {
			t.Fatalf("fixture: %v %s", err, output)
		}
		if output, err := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-stream_loop", "-1", "-i", segment, "-t", "120", "-c", "copy", "-movflags", "+faststart", "-y", path).CombinedOutput(); err != nil {
			t.Fatalf("long fixture: %v %s", err, output)
		}
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		source, err := StageVideo(t.Context(), file)
		if err != nil {
			t.Fatal(err)
		}
		defer source.Close()
		trim := VideoTrim{StartMS: 117250, EndMS: 118500}
		prepared, err := source.Prepare(t.Context(), trim, VideoQualityMaximum)
		if err != nil {
			t.Fatal(err)
		}
		defer prepared.Close()
		if got := prepared.Info(); got.Trim != trim || got.Width != 1920 || got.Height != 1080 || got.FrameRate.String() != "60/1" {
			t.Fatalf("prepared=%+v", got)
		}
	})
}
