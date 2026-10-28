package visual

import (
	"context"
	"encoding/binary"
	"errors"
	"image"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

type countedVideoReader struct {
	remaining int64
	largest   int
}

func (reader *countedVideoReader) Read(body []byte) (int, error) {
	if reader.remaining == 0 {
		return 0, io.EOF
	}
	reader.largest = max(reader.largest, len(body))
	size := int(min(int64(len(body)), reader.remaining))
	reader.remaining -= int64(size)
	return size, nil
}

type failingVideoWriter struct{}

func (failingVideoWriter) Write([]byte) (int, error) { return 0, syscall.ENOSPC }

func TestVideoSource_Streaming(t *testing.T) {
	for _, extra := range []int64{0, 1} {
		t.Run(map[int64]string{0: "accepts eight GiB without a whole source buffer", 1: "rejects actual bytes beyond eight GiB"}[extra], func(t *testing.T) {
			input := &countedVideoReader{remaining: MaxVideoSourceBytes + extra}
			size, err := copyVideoSource(t.Context(), io.Discard, input)
			if size != MaxVideoSourceBytes+extra || (extra == 0 && err != nil) || (extra == 1 && !errors.Is(err, ErrVideoSourceSize)) {
				t.Fatalf("copy = %d, %v", size, err)
			}
			if input.largest > 32<<10 {
				t.Fatalf("read buffer = %d, want <=32768", input.largest)
			}
		})
	}
	t.Run("preserves disk failures", func(t *testing.T) {
		_, err := copyVideoSource(t.Context(), failingVideoWriter{}, &countedVideoReader{remaining: 100})
		if !errors.Is(err, syscall.ENOSPC) {
			t.Fatalf("got %v, want ENOSPC", err)
		}
	})
	t.Run("cancels before reading", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		input := &countedVideoReader{remaining: MaxVideoSourceBytes}
		_, err := copyVideoSource(ctx, io.Discard, input)
		if !errors.Is(err, context.Canceled) || input.remaining != MaxVideoSourceBytes {
			t.Fatalf("copy = %v, remaining %d", err, input.remaining)
		}
	})
}
func TestVideoSource_LongTrim(t *testing.T) {
	tool, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("requires ffmpeg")
	}
	path := filepath.Join(t.TempDir(), "four-hours.mp4")
	command := exec.CommandContext(t.Context(), tool, "-v", "error", "-f", "lavfi", "-i", "nullsrc=size=32x24:rate=1:duration=14400,geq=lum='32+3*mod(T,60)':cb=128:cr=128", "-c:v", "libx264", "-threads", "1", "-g", "60", "-pix_fmt", "yuv420p", "-movflags", "+faststart", path)
	if output, err := command.CombinedOutput(); err != nil {
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
	for _, name := range []string{source.directory, filepath.Join(source.directory, "source")} {
		stat, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		want := os.FileMode(0600)
		if stat.IsDir() {
			want = 0700
		}
		if got := stat.Mode().Perm(); got != want {
			t.Fatalf("staging mode=%v, want %v", got, want)
		}
	}
	if got := source.Info().DurationMS; got != 14400000 {
		t.Fatalf("duration=%d, want 14400000", got)
	}
	loop, err := source.Prepare(t.Context(), VideoTrim{StartMS: 14396250, EndMS: 14399250}, VideoQualityEfficient)
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()
	if got := loop.Info().DurationMS; got != 3000 {
		t.Fatalf("loop duration=%d, want 3000", got)
	}
	poster, err := loop.Open(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	decoded, _, err := image.Decode(poster)
	poster.Close()
	if err != nil {
		t.Fatal(err)
	}
	red, _, _, _ := decoded.At(16, 12).RGBA()
	// The late frame has Y around 203 (roughly RGB 218). The preceding
	// keyframe and timestamp-zero frame have Y=32, so neither can pass.
	if got := int(red >> 8); got < 210 || got > 224 {
		t.Fatalf("late first-frame brightness=%d, want 210–224", got)
	}

}

// Only a handful of headers exist; media payload and large index padding are
// virtual. This exercises 64-bit offsets without a multi-gigabyte fixture.
type videoContainerHeaders struct {
	size    int64
	headers map[int64][]byte
}

func (input videoContainerHeaders) ReadAt(body []byte, offset int64) (int, error) {
	if offset < 0 || offset+int64(len(body)) > input.size {
		return 0, io.EOF
	}
	clear(body)
	for start, header := range input.headers {
		for index, value := range header {
			position := start + int64(index) - offset
			if position >= 0 && position < int64(len(body)) {
				body[position] = value
			}
		}
	}
	return len(body), nil
}
func TestVideoSource_LargeContainer(t *testing.T) {
	indexSize := int64(12 << 20)
	total := int64(6 << 30)
	box := func(size int64, kind string) []byte {
		header := make([]byte, 8)
		binary.BigEndian.PutUint32(header, uint32(size))
		copy(header[4:], kind)
		return header
	}
	media := make([]byte, 16)
	binary.BigEndian.PutUint32(media, 1)
	copy(media[4:], "mdat")
	binary.BigEndian.PutUint64(media[8:], uint64(total-20-indexSize))
	input := videoContainerHeaders{size: total, headers: map[int64][]byte{0: mp4Box("ftyp", []byte("isom0000isom")), 20: box(indexSize, "moov"), 28: box(indexSize-8, "free"), 20 + indexSize: media}}
	if err := validateVideoContainer(input, total); err != nil {
		t.Fatalf("large source = %v, want supported container", err)
	}
	if err := inspectVideoContainer(input, total, true); !errors.Is(err, ErrVideo) {
		t.Fatalf("prepared metadata = %v, want ErrVideo", err)
	}
	movieOffset := int64(5 << 30)
	tailMedia := append([]byte(nil), media...)
	binary.BigEndian.PutUint64(tailMedia[8:], uint64(movieOffset-20))
	tail := videoContainerHeaders{size: total, headers: map[int64][]byte{
		0: mp4Box("ftyp", []byte("isom0000isom")), 20: tailMedia,
		movieOffset: box(indexSize, "moov"), movieOffset + 8: box(indexSize-8, "free"),
		movieOffset + indexSize: box(total-movieOffset-indexSize, "free"),
	}}
	if err := validateVideoContainer(tail, total); err != nil {
		t.Fatalf("movie index past four GiB = %v, want supported source", err)
	}
	if err := validateVideoContainer(input, MaxVideoSourceBytes+1); !errors.Is(err, ErrVideo) {
		t.Fatalf("oversize = %v, want ErrVideo", err)
	}
}
func TestVideoSource_FailedStagingCleanup(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("requires video tools")
	}
	directory := t.TempDir()
	t.Setenv("TMPDIR", directory)
	_, err := StageVideo(t.Context(), brokenVideoTransfer{})
	if !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("stage error = %v, want ENOSPC", err)
	}
	files, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("failed staging retained %d files", len(files))
	}
}

type brokenVideoTransfer struct{}

func (brokenVideoTransfer) Read([]byte) (int, error) { return 0, syscall.ENOSPC }

func TestVideoSource_LongGOPBudget(t *testing.T) {
	tool, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("requires ffmpeg")
	}
	path := filepath.Join(t.TempDir(), "two-hour-gop.mp4")
	command := exec.CommandContext(t.Context(), tool, "-v", "error", "-f", "lavfi", "-i", "color=size=16x16:rate=2:duration=7200", "-c:v", "libx264", "-threads", "1", "-g", "14400", "-keyint_min", "14400", "-sc_threshold", "0", "-pix_fmt", "yuv420p", "-movflags", "+faststart", path)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, output)
	}
	input, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	source, err := StageVideo(t.Context(), input)
	input.Close()
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	loop, err := source.Prepare(t.Context(), VideoTrim{StartMS: 7190000, EndMS: 7200000}, VideoQualityEfficient)
	if loop != nil {
		loop.Close()
		t.Fatal("late cut exceeded the unchanged 7200-frame preroll budget")
	}
	if !errors.Is(err, ErrVideo) {
		t.Fatalf("prepare=%v, want bounded-work rejection", err)
	}
}
