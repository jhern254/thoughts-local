package visual

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const (
	MaxVideoFrameTime     = 10 * time.Second
	maxVideoPreviewWidth  = 640
	maxVideoPreviewHeight = 360
	maxVideoPreviewBytes  = 1 << 20
)

// inputPosition is relative to the demuxer's timeline origin. Cut controls are
// relative to the selected video, which can begin after an earlier audio track.
func (info videoProbe) inputPosition(positionMS int64) string {
	stream, _ := info.video()
	videoStart, _ := strconv.ParseFloat(stream.StartTime, 64)
	containerStart, _ := strconv.ParseFloat(info.Format.StartTime, 64)
	offset := videoStart - containerStart
	if math.IsNaN(offset) || math.IsInf(offset, 0) {
		offset = 0
	}
	return strconv.FormatFloat(float64(positionMS)/1000+offset, 'f', 6, 64)
}

// PreviewFrame is editor-owned transient work, never a retained attachment or
// a full-movie proxy. The caller serializes it with Prepare and joins before Close.
func (source *VideoSource) PreviewFrame(ctx context.Context, positionMS int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if source.directory == "" || positionMS < 0 || positionMS >= source.info.DurationMS {
		return nil, ErrVideo
	}
	ctx, cancel := context.WithTimeout(ctx, MaxVideoFrameTime)
	defer cancel()
	directory, err := os.MkdirTemp(source.directory, "frame-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(directory)
	stream, _ := source.probe.video()
	width, height := source.info.Width, source.info.Height
	scale := math.Min(1, math.Min(float64(maxVideoPreviewWidth)/float64(width), float64(maxVideoPreviewHeight)/float64(height)))
	width, height = max(1, int(float64(width)*scale)), max(1, int(float64(height)*scale))
	path := filepath.Join(directory, "frame.rgb")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		return nil, err
	}
	// Sample the frame covering the requested instant, including End-1ms.
	// Rounding toward the next output tick retains the preceding input frame
	// rather than asking for a nonexistent frame after the source endpoint.
	// A bounded cloned tail lets FPS emit that final covering frame at EOF;
	// only one preview is written, and clones do not count as decoded input.
	args := []string{
		"-nostdin", "-hide_banner", "-loglevel", "error", "-xerror",
		"-max_alloc", strconv.Itoa(maxVideoAllocation),
		"-threads", "1", "-filter_threads", "1", "-protocol_whitelist", "file",
	}
	args = append(args, source.probe.container.inputArgs()...)
	filter := fmt.Sprintf(
		"metadata=mode=add:key=thoughts.frame:value=1,"+
			"metadata=mode=print:key=thoughts.frame:file='pipe\\:1':direct=1,"+
			"tpad=stop_mode=clone:stop_duration=1,fps=%s:start_time=0:round=up,"+
			"trim=start=0,setpts=PTS-STARTPTS,"+
			"scale=%d:%d:in_color_matrix=bt709:out_range=pc,setsar=1,format=rgb24",
		stream.AvgFrameRate, width, height,
	)
	args = append(args, []string{
		"-max_pixels", strconv.Itoa(videoDecoderPixelLimit(stream.Width, stream.Height)),
		"-ss", source.probe.inputPosition(positionMS), "-noaccurate_seek",
		"-i", filepath.Join(source.directory, "source"),
		"-map", fmt.Sprintf("0:%d", stream.Index), "-an", "-sn", "-dn",
		"-map_metadata", "-1", "-map_chapters", "-1", "-frames:v", "1",
		"-vf", filter, "-pix_fmt", "rgb24", "-threads", "1", "-f", "rawvideo",
		"-fs", strconv.Itoa(width * height * 3), "-y", path,
	}...)

	budget := videoDecodeBudget{cancel: cancel, maxFrames: min(maxVideoDecodedFrames, int64(maxVideoDecodedPixels)/(int64(stream.Width)*int64(stream.Height)))}
	if err := runVideoTool(ctx, source.ffmpeg, &budget, args...); err != nil {
		if budget.err != nil {
			return nil, budget.err
		}
		return nil, err
	}
	if budget.frames == 0 || budget.wantValue || len(budget.line) != 0 {
		return nil, ErrVideo
	}
	pixels, err := readBoundedVideoAsset(path, width*height*3)
	if err != nil {
		return nil, err
	}
	body, err := encodeVideoRGB(ctx, pixels, width, height)
	if err != nil {
		return nil, err
	}
	if len(body) > maxVideoPreviewBytes {
		return nil, ErrVideo
	}
	return body, nil
}

// Raw RGB cannot carry source tags. Go's JPEG encoder supplies the JPEG color
// interpretation after FFmpeg has converted the supported Rec.709 video pixels.
func encodeVideoRGB(ctx context.Context, pixels []byte, width, height int) ([]byte, error) {
	if len(pixels) != width*height*3 {
		return nil, ErrVideo
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	decoded := image.NewRGBA(image.Rect(0, 0, width, height))
	for pixel := 0; pixel < width*height; pixel++ {
		copy(decoded.Pix[pixel*4:pixel*4+3], pixels[pixel*3:pixel*3+3])
		decoded.Pix[pixel*4+3] = 255
	}
	var body bytes.Buffer
	if err := jpeg.Encode(&body, decoded, &jpeg.Options{Quality: backgroundJPEGQuality}); err != nil {
		return nil, ErrVideo
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return body.Bytes(), nil
}
