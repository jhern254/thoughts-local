package visual

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// VideoTrim uses milliseconds relative to the displayed source timeline. The
// interval is [StartMS, EndMS); playback speed never changes these source cuts.
type VideoTrim struct {
	StartMS int64 `json:"startMS"`
	EndMS   int64 `json:"endMS"`
}

func (trim VideoTrim) Valid(durationMS int64) bool {
	// Check bounds before subtracting attacker-controlled integer timestamps.
	return trim.StartMS >= 0 && trim.EndMS <= durationMS && trim.EndMS >= 0 && trim.StartMS < trim.EndMS &&
		trim.EndMS-trim.StartMS >= 1000 && trim.EndMS-trim.StartMS <= MaxVideoDuration*1000
}

func videoSeconds(milliseconds int64) string {
	return fmt.Sprintf("%d.%03d", milliseconds/1000, milliseconds%1000)
}

// VideoSourceInfo reports source-relative time and square-pixel display geometry.
// PreparedVideoInfo supplies actual encoded geometry after quality selection.
type VideoSourceInfo struct {
	DurationMS int64 `json:"durationMS"`
	Width      int   `json:"width"`
	Height     int   `json:"height"`
}

// VideoSource owns one private staged upload, not an archived original. Its
// editor must join Prepare callers before Close removes its files.
type VideoSource struct {
	directory string
	probe     videoProbe
	ffmpeg    string
	ffprobe   string
	info      VideoSourceInfo
}

func (source *VideoSource) Info() VideoSourceInfo { return source.info }
func (source *VideoSource) Close() error {
	if source.directory == "" {
		return nil
	}
	err := os.RemoveAll(source.directory)
	if err == nil {
		source.directory = ""
	}
	return err
}

// StageVideo validates once; later explicit previews reuse the immutable input.
// The request owns this work until success transfers file ownership to its editor.
func StageVideo(ctx context.Context, input io.Reader) (*VideoSource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, ErrVideoTools
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		return nil, ErrVideoTools
	}
	ctx, cancel := context.WithTimeout(ctx, MaxVideoProcessingTime)
	defer cancel()
	directory, err := os.MkdirTemp("", "thoughts-video-source-")
	if err != nil {
		return nil, err
	}
	transferred := false
	defer func() {
		if !transferred {
			_ = os.RemoveAll(directory)
		}
	}()
	path := filepath.Join(directory, "source.mp4")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	size, copyErr := io.Copy(file, videoSourceReader{ctx: ctx, input: io.LimitReader(input, MaxVideoBytes+1)})
	if copyErr == nil {
		copyErr = ctx.Err()
	}
	if copyErr == nil {
		copyErr = validateVideoContainer(file, size)
	}
	if err := errors.Join(copyErr, file.Close()); err != nil {
		return nil, err
	}
	probe, err := probeVideo(ctx, ffprobe, path)
	if err != nil {
		return nil, err
	}
	if err := probe.validate(false); err != nil {
		return nil, err
	}
	stream, _ := probe.video()
	duration, _ := strconv.ParseFloat(stream.Duration, 64)
	width, height, err := displayVideoDimensions(stream)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	transferred = true
	return &VideoSource{
		directory: directory,
		probe:     probe,
		ffmpeg:    ffmpeg,
		ffprobe:   ffprobe,
		info: VideoSourceInfo{
			DurationMS: int64(math.Floor(duration * 1000)),
			Width:      width,
			Height:     height,
		},
	}, nil
}

// Trim records the requested source interval; DurationMS records the validated
// whole-frame output duration, which can differ by at most one frame.
type PreparedVideoInfo struct {
	VideoQuality VideoQuality   `json:"videoQuality"`
	FrameRate    VideoFrameRate `json:"frameRate"`
	Trim         VideoTrim      `json:"trim"`
	DurationMS   int64          `json:"durationMS"`
	Width        int            `json:"width"`
	Height       int            `json:"height"`
}

// PreparedVideo can only be populated by validated native preparation. Browsers
// receive an opaque editor identifier, never this private path or a trust flag.
type PreparedVideo struct {
	directory string
	info      PreparedVideoInfo
}

func (video *PreparedVideo) Info() PreparedVideoInfo { return video.info }
func (video *PreparedVideo) Close() error {
	if video.directory == "" {
		return nil
	}
	err := os.RemoveAll(video.directory)
	if err == nil {
		video.directory = ""
	}
	return err
}
func (video *PreparedVideo) Open(ctx context.Context, poster bool) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if video.directory == "" {
		return nil, os.ErrNotExist
	}
	name := "display.mp4"
	if poster {
		name = "poster.jpg"
	}
	return os.Open(filepath.Join(video.directory, name))
}
func (source *VideoSource) Prepare(ctx context.Context, trim VideoTrim, quality VideoQuality) (*PreparedVideo, error) {
	if !quality.Valid() {
		return nil, ErrVideoQuality
	}
	if !trim.Valid(source.info.DurationMS) || source.directory == "" {
		return nil, ErrVideo
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, MaxVideoProcessingTime)
	defer cancel()
	// Each candidate has its own bounded files. The previous accepted loop remains
	// usable until the browser acknowledges its replacement, even on encode failure.
	directory, err := os.MkdirTemp("", "thoughts-video-loop-")
	if err != nil {
		return nil, err
	}
	prepared, err := encodeVideoLoop(ctx, source.ffmpeg, source.ffprobe, filepath.Join(source.directory, "source.mp4"), directory, source.probe, trim, quality)
	if err != nil {
		_ = os.RemoveAll(directory)
	}
	return prepared, err
}

// validateLoop checks the requested span as well as the independent codec and
// resource limits. A successful processor exit or a valid short file alone must
// not turn a truncated source/output into a silently shorter accepted loop.
func (info videoProbe) validateLoop(trim VideoTrim, plan videoOutputPlan) error {
	if err := info.validate(true); err != nil {
		return err
	}
	stream, _ := info.video()
	duration, _ := strconv.ParseFloat(stream.Duration, 64)
	rate, err := parseVideoFrameRate(stream.AvgFrameRate)
	if err != nil {
		return err
	}
	count, err := strconv.ParseInt(stream.FrameCount, 10, 64)
	if err != nil || count != plan.FrameCount || stream.Width != plan.Width || stream.Height != plan.Height || rate != plan.FrameRate || stream.Level != plan.Level || stream.Refs != 1 {
		return ErrVideo
	}
	fps := float64(rate.Numerator) / float64(rate.Denominator)
	requested := float64(trim.EndMS-trim.StartMS) / 1000
	// Frame quantization may round either edge. One microsecond accommodates
	// ffprobe's decimal timestamp precision, not another displayed frame.
	if math.Abs(duration-requested) > 1/fps+0.000001 {
		return ErrVideo
	}
	return nil
}
