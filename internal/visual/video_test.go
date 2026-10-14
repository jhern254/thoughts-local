package visual

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"testing"
	"time"
)

func mp4Box(kind string, payload []byte) []byte {
	body := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(body, uint32(len(body)))
	copy(body[4:8], kind)
	copy(body[8:], payload)
	return body
}

func TestVideoContainer(t *testing.T) {
	t.Run("accepts only self contained data references", func(t *testing.T) {
		entry := mp4Box("url ", []byte{0, 0, 0, 1})
		dref := append([]byte{0, 0, 0, 0, 0, 0, 0, 1}, entry...)
		body := mp4Box("ftyp", []byte("isom\x00\x00\x00\x00isom"))
		body = append(body, mp4Box("moov", mp4Box("trak", mp4Box("mdia", mp4Box("minf", mp4Box("dinf", mp4Box("dref", dref))))))...)
		body = append(body, mp4Box("mdat", []byte{1})...)
		if err := validateVideoContainer(bytes.NewReader(body), int64(len(body))); err != nil {
			t.Fatal(err)
		}
		entry[11] = 0 // A location string is forbidden even when the file protocol is allowed.
		body = bytes.Replace(body, []byte{0, 0, 0, 1}, []byte{0, 0, 0, 0}, 1)
		if err := validateVideoContainer(bytes.NewReader(body), int64(len(body))); !errors.Is(err, ErrVideo) {
			t.Fatalf("external reference: got %v, want ErrVideo", err)
		}
	})
	t.Run("rejects playlists truncated boxes and compressed containers", func(t *testing.T) {
		for _, body := range [][]byte{[]byte("#EXTM3U private"), {0, 0, 0, 100, 'm', 'o', 'o', 'v'}, mp4Box("cmov", nil), mp4Box("moof", nil)} {
			if err := validateVideoContainer(bytes.NewReader(body), int64(len(body))); !errors.Is(err, ErrVideo) {
				t.Fatalf("got %v, want ErrVideo", err)
			}
		}
	})
}

func TestVideoLimits(t *testing.T) {
	t.Run("validates supported source and independent prepared output", func(t *testing.T) {
		stream := videoStream{CodecName: "h264", CodecType: "video", Profile: "Constrained Baseline", Level: 32, Width: 1280, Height: 720, PixFmt: "yuv420p", AvgFrameRate: "24/1", SampleAspectRatio: "1:1", Duration: "8.0", ColorSpace: "bt709", ColorTransfer: "bt709", ColorPrimaries: "bt709", ColorRange: "tv"}
		info := videoProbe{Streams: []videoStream{stream}}
		if err := info.validate(false); err != nil {
			t.Fatal(err)
		}
		if err := info.validate(true); err != nil {
			t.Fatal(err)
		}
		for _, change := range []func(*videoStream){func(s *videoStream) { s.PixFmt = "yuv420p10le" }, func(s *videoStream) { s.ColorTransfer = "smpte2084" }, func(s *videoStream) { s.Duration = "31" }, func(s *videoStream) { s.AvgFrameRate = "61/1" }, func(s *videoStream) { s.CodecName = "hevc" }} {
			bad := stream
			change(&bad)
			if err := (videoProbe{Streams: []videoStream{bad}}).validate(false); !errors.Is(err, ErrVideo) {
				t.Fatalf("got %v, want ErrVideo", err)
			}
		}
		info.Streams = append(info.Streams, videoStream{CodecType: "audio"})
		if err := info.validate(true); !errors.Is(err, ErrVideo) {
			t.Fatalf("audio retained: %v", err)
		}
	})
	t.Run("keeps even dimensions within pixel and axis budgets without upscaling", func(t *testing.T) {
		for _, size := range [][2]int{{4096, 2160}, {1080, 1920}, {4096, 100}, {101, 99}, {320, 240}} {
			width, height := storedVideoDimensions(size[0], size[1])
			if width%2 != 0 || height%2 != 0 || width > 1280 || height > 1280 || width*height > 921600 || width > size[0] || height > size[1] {
				t.Fatalf("%v -> %dx%d violates budget", size, width, height)
			}
		}
	})
}

func TestVideoProcessHelper(t *testing.T) {
	if len(os.Args) < 2 || os.Args[len(os.Args)-1] != "video-process-helper" {
		return
	}
	time.Sleep(10 * time.Second)
	os.Exit(0)
}

func TestVideoProcessingOwnership(t *testing.T) {
	t.Run("cancellation joins the child before returning", func(t *testing.T) {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer cancel()
		start := time.Now()
		err = runVideoTool(ctx, executable, io.Discard, "-test.run=^TestVideoProcessHelper$", "video-process-helper")
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("got %v, want deadline", err)
		}
		if time.Since(start) > 2*time.Second {
			t.Fatal("cancellation did not promptly join child")
		}
	})
	t.Run("probe and diagnostic budgets reject excess without retaining private content", func(t *testing.T) {
		var output videoProbeBuffer
		if _, err := output.Write(make([]byte, maxVideoProbeBytes+1)); !errors.Is(err, ErrVideo) {
			t.Fatalf("got %v, want ErrVideo", err)
		}
		remaining := videoDiagnostics(4)
		if _, err := remaining.Write([]byte("PRIVATE path metadata")); !errors.Is(err, ErrVideo) {
			t.Fatalf("got %v, want ErrVideo", err)
		}
	})
}

func TestVideoDisplayDeclarations(t *testing.T) {
	t.Run("rejects ICC HDR and clean aperture declarations instead of relabeling", func(t *testing.T) {
		for _, declaration := range [][]byte{mp4Box("colr", []byte("profPRIVATE")), mp4Box("colr", []byte{'n', 'c', 'l', 'x', 0, 9, 0, 16, 0, 9, 0}), mp4Box("clap", make([]byte, 32))} {
			sample := mp4Box("avc1", append(make([]byte, 78), declaration...))
			entries := mp4Box("stsd", append([]byte{0, 0, 0, 0, 0, 0, 0, 1}, sample...))
			body := mp4Box("ftyp", []byte("isom\x00\x00\x00\x00isom"))
			body = append(body, mp4Box("moov", mp4Box("trak", mp4Box("mdia", mp4Box("minf", mp4Box("stbl", entries)))))...)
			body = append(body, mp4Box("mdat", []byte{1})...)
			if err := validateVideoContainer(bytes.NewReader(body), int64(len(body))); !errors.Is(err, ErrVideo) {
				t.Fatalf("got %v, want ErrVideo", err)
			}
		}
	})
}

func FuzzVideoContainer(f *testing.F) {
	body, err := os.ReadFile("../browserterm/client/fixtures/visual-video.mp4")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(body)
	f.Add(mp4Box("moov", mp4Box("dref", []byte{0, 0, 0, 0, 255, 255, 255, 255})))
	f.Add([]byte("#EXTM3U"))
	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 1<<20 {
			return
		}
		err := validateVideoContainer(bytes.NewReader(body), int64(len(body)))
		if err != nil && !errors.Is(err, ErrVideo) {
			t.Fatalf("unexpected diagnostic: %v", err)
		}
	})
}
