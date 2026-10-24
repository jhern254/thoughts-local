package visual

import (
	"bytes"
	"context"
	"errors"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func syntheticMatroska(t *testing.T, codec, pixels string) string {
	t.Helper()
	requireMatroskaTools(t)
	path := filepath.Join(t.TempDir(), "movie.mkv")
	args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=24:duration=3", "-c:v", codec, "-threads", "1", "-pix_fmt", pixels, "-color_primaries", "bt709", "-color_trc", "bt709", "-colorspace", "bt709", "-color_range", "tv"}
	if codec == "libx265" {
		args = append(args, "-x265-params", "pools=none:frame-threads=1:log-level=error")
	}
	if codec == "libaom-av1" {
		args = append(args, "-cpu-used", "8")
	}
	args = append(args, path)
	if output, err := exec.CommandContext(t.Context(), "ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("synthetic fixture: %v %s", err, output)
	}
	return path
}

func TestMatroskaSource(t *testing.T) {
	t.Run("stages H264 and prepares independently validated MP4", func(t *testing.T) {
		path := syntheticMatroska(t, "libx264", "yuv420p")
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
		if got := source.Info().DurationMS; got != 3000 {
			t.Fatalf("duration=%d, want 3000", got)
		}
		loop, err := source.Prepare(t.Context(), VideoTrim{StartMS: 1250, EndMS: 2750}, VideoQualityEfficient)
		if err != nil {
			t.Fatal(err)
		}
		defer loop.Close()
		if got := loop.Info().FrameCount; got != 36 {
			t.Fatalf("frames=%d, want 36", got)
		}
	})
}

func TestMatroskaTracks(t *testing.T) {
	stream := videoStream{Index: 2, CodecType: "video", CodecName: "h264", PixFmt: "yuv420p", Width: 160, Height: 90, AvgFrameRate: "24/1"}
	t.Run("accepts absent stream duration and many ignored streams", func(t *testing.T) {
		info := videoProbe{container: sourceMatroska, Streams: []videoStream{stream}}
		info.Format.Duration = "3"
		for range 12 {
			info.Streams = append(info.Streams, videoStream{CodecType: "audio"})
		}
		cover := videoStream{CodecType: "video", CodecName: "mjpeg"}
		cover.Disposition.AttachedPic = 1
		info.Streams = append(info.Streams, cover)
		if err := info.validate(false); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("requires an unambiguous moving track", func(t *testing.T) {
		second := stream
		second.Index = 3
		info := videoProbe{Streams: []videoStream{stream, second}}
		if _, ok := info.video(); ok {
			t.Fatal("ambiguous tracks accepted")
		}
		info.Streams[1].Disposition.Default = 1
		if got, ok := info.video(); !ok || got.Index != 3 {
			t.Fatalf("selected=%d,%v, want 3,true", got.Index, ok)
		}
	})
}

func TestVideoSource_PreviewFrame(t *testing.T) {
	file, err := os.Open(syntheticMatroska(t, "libx264", "yuv420p"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	source, err := StageVideo(t.Context(), file)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	t.Run("returns a bounded still without persisting frames", func(t *testing.T) {
		body, err := source.PreviewFrame(t.Context(), source.Info().DurationMS-1)
		if err != nil {
			t.Fatal(err)
		}
		frame, err := jpeg.Decode(bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if got := frame.Bounds().Size(); got.X != 160 || got.Y != 90 {
			t.Fatalf("size=%v, want 160x90", got)
		}
		files, err := os.ReadDir(source.directory)
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 1 {
			t.Fatalf("staging has %d files, want source only", len(files))
		}
	})
	t.Run("rejects invalid positions and canceled owners", func(t *testing.T) {
		for _, position := range []int64{-1, 3000, 1 << 62} {
			if _, err := source.PreviewFrame(t.Context(), position); !errors.Is(err, ErrVideo) {
				t.Fatalf("position %d: %v", position, err)
			}
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := source.PreviewFrame(ctx, 0); !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want canceled", err)
		}
	})
}

func TestMatroskaCodecs(t *testing.T) {
	for _, scenario := range []struct{ name, codec, pixels string }{
		{"HEVC Main", "libx265", "yuv420p"}, {"HEVC Main10 SDR", "libx265", "yuv420p10le"}, {"VP9 SDR", "libvpx-vp9", "yuv420p"}, {"AV1 SDR", "libaom-av1", "yuv420p"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			file, err := os.Open(syntheticMatroska(t, scenario.codec, scenario.pixels))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			source, err := StageVideo(t.Context(), file)
			if err != nil {
				t.Fatal(err)
			}
			defer source.Close()
			body, err := source.PreviewFrame(t.Context(), 1250)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := jpeg.Decode(bytes.NewReader(body)); err != nil {
				t.Fatal(err)
			}
			loop, err := source.Prepare(t.Context(), VideoTrim{StartMS: 500, EndMS: 2000}, VideoQualityEfficient)
			if err != nil {
				t.Fatal(err)
			}
			defer loop.Close()
			mp4 := filepath.Join(t.TempDir(), "same-codec.mp4")
			if output, err := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-i", file.Name(), "-c", "copy", "-movflags", "+faststart", mp4).CombinedOutput(); err != nil {
				t.Fatalf("MP4 fixture: %v %s", err, output)
			}
			compatible, err := os.Open(mp4)
			if err != nil {
				t.Fatal(err)
			}
			defer compatible.Close()
			other, err := StageVideo(t.Context(), compatible)
			if err != nil {
				t.Fatal(err)
			}
			defer other.Close()
			prepared, err := other.Prepare(t.Context(), VideoTrim{EndMS: 1500}, VideoQualityEfficient)
			if err != nil {
				t.Fatal(err)
			}
			defer prepared.Close()
		})
	}
}

func TestMatroskaMetadataSafety(t *testing.T) {
	path := syntheticMatroska(t, "libx264", "yuv420p")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("rejects wrong document type and malformed sizes", func(t *testing.T) {
		for _, bad := range [][]byte{bytes.Replace(body, []byte("matroska"), []byte("not-mkv!"), 1), body[:24], append([]byte{0x1a, 0x45, 0xdf, 0xa3, 0}, body...)} {
			if _, err := identifyVideoContainer(t.Context(), bytes.NewReader(bad), int64(len(bad))); !errors.Is(err, ErrVideo) {
				t.Fatalf("got %v, want safe rejection", err)
			}
		}
	})
	t.Run("bounds attachments before the demuxer can allocate them", func(t *testing.T) {
		inspection := matroskaInspector{ctx: t.Context(), input: bytes.NewReader([]byte{0x50, 0x35, 0x80})}
		if err := inspection.declarations(0, 3, 0); !errors.Is(err, ErrVideoProtected) {
			t.Fatalf("protection=%v", err)
		}
		inspection = matroskaInspector{ctx: t.Context(), input: bytes.NewReader([]byte{0x3c, 0xb9, 0x23, 0x80})}
		if err := inspection.declarations(0, 4, 0); !errors.Is(err, ErrVideoContainer) {
			t.Fatalf("external reference=%v", err)
		}
	})
}

func TestMatroskaMovieLayout(t *testing.T) {
	requireMatroskaTools(t)
	directory := t.TempDir()
	subtitle := filepath.Join(directory, "subtitles.srt")
	attachment := filepath.Join(directory, "font.txt")
	if err := os.WriteFile(subtitle, []byte("1\n00:00:00,000 --> 00:00:02,000\nSYNTHETIC PRIVATE SUBTITLE\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(attachment, []byte("SYNTHETIC PRIVATE ATTACHMENT"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "movie.mkv")
	args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=24:duration=3", "-f", "lavfi", "-i", "sine=duration=3", "-i", subtitle, "-map", "0:v"}
	for range 6 {
		args = append(args, "-map", "1:a")
	}
	for range 3 {
		args = append(args, "-map", "2:s")
	}
	args = append(args, "-c:v", "libx264", "-threads", "1", "-c:a", "aac", "-c:s", "srt", "-attach", attachment, "-metadata:s:t", "mimetype=application/x-truetype-font", "-metadata:s:t", "filename=../../PRIVATE.ttf", "-metadata", "title=SYNTHETIC PRIVATE TITLE", path)
	if output, err := exec.CommandContext(t.Context(), "ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, output)
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
	loop, err := source.Prepare(t.Context(), VideoTrim{EndMS: 1500}, VideoQualityEfficient)
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()
	video, err := loop.Open(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer video.Close()
	info, err := probeVideo(t.Context(), "ffprobe", video.Name())
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Streams) != 1 || info.Streams[0].CodecType != "video" {
		t.Fatalf("retained streams=%d, want one video", len(info.Streams))
	}
	output, err := os.ReadFile(video.Name())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(output, []byte("PRIVATE")) {
		t.Fatal("source private metadata retained")
	}
	files, err := os.ReadDir(source.directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name() != "source" {
		t.Fatalf("extracted source files=%v", files)
	}
}

func TestMatroskaNonzeroOrigin(t *testing.T) {
	requireMatroskaTools(t)
	path := filepath.Join(t.TempDir(), "offset.mkv")
	args := []string{"-v", "error", "-f", "lavfi", "-i", "nullsrc=size=32x24:rate=10:duration=3,geq=lum='32+50*T':cb=128:cr=128", "-c:v", "libx264", "-threads", "1", "-g", "30", "-output_ts_offset", "5", path}
	if output, err := exec.CommandContext(t.Context(), "ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, output)
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
	if got := source.Info().DurationMS; got != 3000 {
		t.Fatalf("duration=%d, want 3000", got)
	}
	frame, err := source.PreviewFrame(t.Context(), 1500)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(frame))
	if err != nil {
		t.Fatal(err)
	}
	red, _, _, _ := decoded.At(16, 12).RGBA()
	if value := red >> 8; value < 98 || value > 117 {
		t.Fatalf("brightness=%d, want source-relative 1.5-second frame", value)
	}
	loop, err := source.Prepare(t.Context(), VideoTrim{StartMS: 1500, EndMS: 2750}, VideoQualityEfficient)
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()
}

func TestMatroskaMetadataBudget(t *testing.T) {
	header := append([]byte{0x1a, 0x45, 0xdf, 0xa3, 0x8b, 0x42, 0x82, 0x88}, []byte("matroska")...)
	segment := []byte{0x18, 0x53, 0x80, 0x67, 0x01, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	attachment := []byte{0x19, 0x41, 0xa4, 0x69, 0x01, 0, 0, 0, 0x02, 0, 0, 0} // 32 MiB payload, plus headers exceeds combined metadata budget.
	start := int64(len(header) + len(segment))
	size := start + 10 + 12 + (32 << 20) + 5
	input := videoContainerHeaders{size: size, headers: map[int64][]byte{0: header, int64(len(header)): segment, start: {0x15, 0x49, 0xa9, 0x66, 0x80, 0x16, 0x54, 0xae, 0x6b, 0x80}, start + 10: attachment, size - 5: {0x1f, 0x43, 0xb6, 0x75, 0x80}}}
	if _, err := identifyVideoContainer(t.Context(), input, size); !errors.Is(err, ErrVideoContainer) {
		t.Fatalf("oversized metadata=%v, want container rejection", err)
	}
}

func FuzzMatroskaPreflight(f *testing.F) {
	header := append([]byte{0x1a, 0x45, 0xdf, 0xa3, 0x8b, 0x42, 0x82, 0x88}, []byte("matroska")...)
	f.Add(append(header, []byte{0x18, 0x53, 0x80, 0x67, 0xff, 0x15, 0x49, 0xa9, 0x66, 0x80, 0x16, 0x54, 0xae, 0x6b, 0x80, 0x1f, 0x43, 0xb6, 0x75, 0x80}...))
	f.Add([]byte{0x1a, 0x45, 0xdf, 0xa3, 0xff})
	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 1<<20 {
			return
		}
		_, err := identifyVideoContainer(t.Context(), bytes.NewReader(body), int64(len(body)))
		if err != nil && !errors.Is(err, ErrVideo) {
			t.Fatalf("unexpected public error: %v", err)
		}
	})
}

func TestMatroskaSDRColor(t *testing.T) {
	requireMatroskaTools(t)
	for _, colorRange := range []string{"tv", "pc"} {
		t.Run("Main10 "+colorRange+" range preserves neutral pixels", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "gray.mkv")
			args := []string{"-v", "error", "-f", "lavfi", "-i", "nullsrc=size=64x48:rate=24:duration=2,geq=lum=128:cb=128:cr=128", "-c:v", "libx265", "-threads", "1", "-x265-params", "pools=none:frame-threads=1:log-level=error", "-pix_fmt", "yuv420p10le", "-color_primaries", "bt709", "-color_trc", "bt709", "-colorspace", "bt709", "-color_range", colorRange, path}
			if output, err := exec.CommandContext(t.Context(), "ffmpeg", args...).CombinedOutput(); err != nil {
				t.Fatalf("fixture: %v %s", err, output)
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
			loop, err := source.Prepare(t.Context(), VideoTrim{EndMS: 1500}, VideoQualityEfficient)
			if err != nil {
				t.Fatal(err)
			}
			defer loop.Close()
			poster, err := loop.Open(t.Context(), true)
			if err != nil {
				t.Fatal(err)
			}
			defer poster.Close()
			image, err := jpeg.Decode(poster)
			if err != nil {
				t.Fatal(err)
			}
			red, green, blue, _ := image.At(32, 24).RGBA()
			want := int64(130)
			if colorRange == "pc" {
				want = 128
			}
			for _, channel := range []uint32{red, green, blue} {
				got := int64(channel >> 8)
				if got < want-4 || got > want+4 {
					t.Fatalf("channel=%d, want %d +/-4", got, want)
				}
			}
		})
	}
	t.Run("rejects untagged ten bit HDR and unsupported codecs", func(t *testing.T) {
		stream := videoStream{CodecName: "hevc", CodecType: "video", PixFmt: "yuv420p10le", Width: 160, Height: 90, Duration: "3", AvgFrameRate: "24/1"}
		if err := (videoProbe{Streams: []videoStream{stream}}).validate(false); !errors.Is(err, ErrVideoColor) {
			t.Fatalf("untagged ten-bit=%v", err)
		}
		stream.ColorSpace = "bt709"
		stream.ColorPrimaries = "bt709"
		for _, transfer := range []string{"smpte2084", "arib-std-b67"} {
			stream.ColorTransfer = transfer
			if err := (videoProbe{Streams: []videoStream{stream}}).validate(false); !errors.Is(err, ErrVideoColor) {
				t.Fatalf("HDR=%v", err)
			}
		}
	})
}

func TestMatroskaNoCues(t *testing.T) {
	path := syntheticMatroska(t, "libx264", "yuv420p")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Rename the Cues ID and its SeekHead reference to an unknown same-width ID.
	// Packet/cluster bytes are unchanged; FFmpeg must use its bounded seek path.
	body = bytes.ReplaceAll(body, []byte{0x1c, 0x53, 0xbb, 0x6b}, []byte{0x1c, 0x53, 0xbb, 0x6c})
	source, err := StageVideo(t.Context(), bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if _, err := source.PreviewFrame(t.Context(), 2750); err != nil {
		t.Fatal(err)
	}
	loop, err := source.Prepare(t.Context(), VideoTrim{StartMS: 1500, EndMS: 2750}, VideoQualityEfficient)
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()
}

func requireMatroskaTools(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("requires installed FFmpeg/ffprobe")
		}
	}
}

func TestMatroskaVariableCadence(t *testing.T) {
	requireMatroskaTools(t)
	path := filepath.Join(t.TempDir(), "variable.mkv")
	args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=24:duration=4", "-vf", "select='if(lt(t,2),1,not(mod(n,2)))'", "-fps_mode", "vfr", "-c:v", "libx264", "-threads", "1", "-g", "48", path}
	if output, err := exec.CommandContext(t.Context(), "ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, output)
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
	loop, err := source.Prepare(t.Context(), VideoTrim{StartMS: 1250, EndMS: 3750}, VideoQualityEfficient)
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()
	if loop.Info().FrameRate != (VideoFrameRate{24, 1}) {
		t.Fatalf("output rate=%v, want declared 24 FPS CFR", loop.Info().FrameRate)
	}
}

func TestMatroskaLongGOPBudget(t *testing.T) {
	requireMatroskaTools(t)
	path := filepath.Join(t.TempDir(), "long-gop.mkv")
	args := []string{"-v", "error", "-f", "lavfi", "-i", "color=size=16x16:rate=2:duration=7200", "-c:v", "libx264", "-threads", "1", "-g", "14400", "-keyint_min", "14400", "-sc_threshold", "0", "-pix_fmt", "yuv420p", path}
	if output, err := exec.CommandContext(t.Context(), "ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, output)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body = bytes.ReplaceAll(body, []byte{0x1c, 0x53, 0xbb, 0x6b}, []byte{0x1c, 0x53, 0xbb, 0x6c})
	source, err := StageVideo(t.Context(), bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	loop, err := source.Prepare(t.Context(), VideoTrim{StartMS: 7190000, EndMS: 7200000}, VideoQualityEfficient)
	if loop != nil {
		loop.Close()
		t.Fatal("late no-Cues cut escaped unchanged preroll budget")
	}
	if !errors.Is(err, ErrVideo) {
		t.Fatalf("late cut=%v, want bounded rejection", err)
	}
}
