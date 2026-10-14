package visual

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	MaxVideoBytes           = 128 << 20
	MaxStoredVideoBytes     = 16 << 20
	MaxVideoDimension       = 4096
	MaxVideoPixels          = 9_000_000
	MaxVideoFPS             = 60
	MaxVideoDuration        = 30
	maxVideoStreams         = 4
	maxVideoDecodedPixels   = 4_000_000_000
	MaxStoredVideoDimension = 1280
	MaxStoredVideoPixels    = 1280 * 720
	MaxStoredVideoFPS       = 24
	videoProcessingTimeout  = 2 * time.Minute
	maxVideoProbeBytes      = 64 << 10
	// This limits each FFmpeg allocation, not total process memory. Frame size,
	// stream count, decoder threads and duration bound the remaining work.
	maxVideoAllocation = 64 << 20
)

var ErrVideo = errors.New("choose a self-contained SDR H.264 MP4 or MOV within the video limits")
var ErrVideoTools = errors.New("video backgrounds require FFmpeg and ffprobe")
var ErrPlayback = errors.New("video speed must be between 25 and 100 percent")

type videoStream struct {
	Index             int    `json:"index"`
	CodecName         string `json:"codec_name"`
	CodecType         string `json:"codec_type"`
	Profile           string `json:"profile"`
	Level             int    `json:"level"`
	Width             int    `json:"width"`
	Height            int    `json:"height"`
	PixFmt            string `json:"pix_fmt"`
	AvgFrameRate      string `json:"avg_frame_rate"`
	SampleAspectRatio string `json:"sample_aspect_ratio"`
	Duration          string `json:"duration"`
	ColorSpace        string `json:"color_space"`
	ColorTransfer     string `json:"color_transfer"`
	ColorPrimaries    string `json:"color_primaries"`
	ColorRange        string `json:"color_range"`
	SideData          []struct {
		Type          string `json:"side_data_type"`
		Rotation      int    `json:"rotation"`
		DisplayMatrix string `json:"displaymatrix"`
	} `json:"side_data_list"`
}

type videoProbe struct {
	Streams []videoStream `json:"streams"`
	Format  struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func videoRatio(text, separator string) float64 {
	parts := strings.Split(text, separator)
	if len(parts) != 2 {
		return math.NaN()
	}
	numerator, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return math.NaN()
	}
	denominator, err := strconv.ParseFloat(parts[1], 64)
	if err != nil || denominator <= 0 {
		return math.NaN()
	}
	return numerator / denominator
}

func (info videoProbe) video() (videoStream, bool) {
	var selected videoStream
	count := 0
	for _, stream := range info.Streams {
		if stream.CodecType == "video" {
			selected = stream
			count++
		}
	}
	return selected, count == 1
}

func (info videoProbe) validate(prepared bool) error {
	stream, ok := info.video()
	if !ok || len(info.Streams) > maxVideoStreams {
		return ErrVideo
	}
	duration, err := strconv.ParseFloat(stream.Duration, 64)
	if info.Format.Duration != "" {
		containerDuration, parseErr := strconv.ParseFloat(info.Format.Duration, 64)
		if parseErr != nil || !finitePositive(containerDuration) || containerDuration < 1 || containerDuration > MaxVideoDuration {
			return ErrVideo
		}
	}
	fps := videoRatio(stream.AvgFrameRate, "/")
	sar := videoRatio(stream.SampleAspectRatio, ":")
	if stream.SampleAspectRatio == "N/A" || stream.SampleAspectRatio == "" {
		sar = 1
	}
	if err != nil || !finitePositive(duration) || duration < 1 || duration > MaxVideoDuration || !finitePositive(fps) || fps > MaxVideoFPS || !finitePositive(sar) || sar < .25 || sar > 4 {
		return ErrVideo
	}
	if stream.CodecName != "h264" || (stream.PixFmt != "yuv420p" && stream.PixFmt != "yuvj420p") || stream.Width < 2 || stream.Height < 2 || stream.Width > MaxVideoDimension || stream.Height > MaxVideoDimension || int64(stream.Width)*int64(stream.Height) > MaxVideoPixels {
		return ErrVideo
	}
	// The first supported path is SDR Rec.709. Unknown signaling is treated as
	// ordinary SDR Rec.709; explicitly different primaries/transfer are rejected,
	// never relabeled as Rec.709 without a reviewed conversion.
	for _, value := range []string{stream.ColorSpace, stream.ColorTransfer, stream.ColorPrimaries} {
		if value != "" && value != "unknown" && value != "bt709" {
			return ErrVideo
		}
	}
	// Bound aggregate decoded work as well as each individual frame.
	if float64(stream.Width)*float64(stream.Height)*fps*duration > maxVideoDecodedPixels {
		return ErrVideo
	}
	if _, err := stream.displayRotation(); err != nil {
		return err
	}
	if prepared {
		if len(info.Streams) != 1 || stream.PixFmt != "yuv420p" || stream.Width%2 != 0 || stream.Height%2 != 0 || stream.Width > MaxStoredVideoDimension || stream.Height > MaxStoredVideoDimension || stream.Width*stream.Height > MaxStoredVideoPixels || fps > MaxStoredVideoFPS || sar != 1 || len(stream.SideData) > 0 {
			return ErrVideo
		}
		if stream.Profile != "Constrained Baseline" || stream.Level != 32 {
			return ErrVideo
		}
		if stream.ColorSpace != "bt709" || stream.ColorTransfer != "bt709" || stream.ColorPrimaries != "bt709" || stream.ColorRange != "tv" {
			return ErrVideo
		}
	}
	return nil
}

func finitePositive(value float64) bool {
	return value > 0 && !math.IsInf(value, 0) && !math.IsNaN(value)
}

func (stream videoStream) displayRotation() (int, error) {
	rotation := 0
	for _, side := range stream.SideData {
		if side.Type != "Display Matrix" {
			return 0, ErrVideo
		}
		if side.Rotation%90 != 0 {
			return 0, ErrVideo
		}
		// A display matrix includes scale/mirroring as well as rotation. Accept
		// only orthogonal unit rotations; broader transforms need separate evidence.
		lines := strings.Split(strings.TrimSpace(side.DisplayMatrix), "\n")
		if len(lines) != 3 {
			return 0, ErrVideo
		}
		var matrix [9]int64
		for row, line := range lines {
			_, values, ok := strings.Cut(line, ":")
			if !ok {
				return 0, ErrVideo
			}
			fields := strings.Fields(values)
			if len(fields) != 3 {
				return 0, ErrVideo
			}
			for col, field := range fields {
				number, err := strconv.ParseInt(field, 10, 64)
				if err != nil {
					return 0, ErrVideo
				}
				matrix[row*3+col] = number
			}
		}
		// 16.16 fixed point for the 2x2 transform; the last element is 2.30.
		if matrix[2] != 0 || matrix[5] != 0 || matrix[6] != 0 || matrix[7] != 0 || matrix[8] != 1<<30 || matrix[0]*matrix[4]-matrix[1]*matrix[3] != 1<<32 {
			return 0, ErrVideo
		}
		for _, value := range matrix[:5] {
			if value != 0 && value != 1<<16 && value != -(1<<16) {
				return 0, ErrVideo
			}
		}
		rotation = ((side.Rotation % 360) + 360) % 360
	}
	return rotation, nil
}

func storedVideoDimensions(width, height int) (int, int) {
	scale := math.Min(1, math.Min(float64(MaxStoredVideoDimension)/float64(max(width, height)), math.Sqrt(float64(MaxStoredVideoPixels)/(float64(width)*float64(height)))))
	// yuv420p requires even dimensions. Round down to avoid exceeding the budget.
	return max(2, int(float64(width)*scale)/2*2), max(2, int(float64(height)*scale)/2*2)
}

// A capped writer fails the subprocess when JSON output exceeds its budget.
// Diagnostics are discarded, not retained or exposed through public errors.
type videoProbeBuffer struct{ bytes.Buffer }

func (output *videoProbeBuffer) Write(body []byte) (int, error) {
	if output.Len()+len(body) > maxVideoProbeBytes {
		return 0, ErrVideo
	}
	return output.Buffer.Write(body)
}

// Discard diagnostics while capping how much the child can emit. No authored
// metadata or decoder paths are retained, including on subprocess failure.
type videoDiagnostics int

func (remaining *videoDiagnostics) Write(body []byte) (int, error) {
	if len(body) > int(*remaining) {
		return 0, ErrVideo
	}
	*remaining -= videoDiagnostics(len(body))
	return len(body), nil
}

func runVideoTool(ctx context.Context, tool string, output io.Writer, args ...string) error {
	command := exec.CommandContext(ctx, tool, args...)
	command.Stdout = output
	diagnostics := videoDiagnostics(maxVideoProbeBytes)
	command.Stderr = &diagnostics
	command.WaitDelay = time.Second
	// Run waits for the process and output copier before the caller removes
	// staged files. There is no detached processing after request cancellation.
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrVideo
	}
	return ctx.Err()
}

func probeVideo(ctx context.Context, tool, path string) (videoProbe, error) {
	var output videoProbeBuffer
	args := []string{"-v", "error", "-max_alloc", strconv.Itoa(maxVideoAllocation), "-protocol_whitelist", "file", "-f", "mov", "-enable_drefs", "0", "-export_all", "0", "-threads", "1", "-max_pixels", strconv.Itoa(MaxVideoPixels), "-show_entries", "stream=index,codec_name,codec_type,profile,level,width,height,pix_fmt,avg_frame_rate,sample_aspect_ratio,duration,color_space,color_transfer,color_primaries,color_range:stream_side_data=side_data_type,rotation,displaymatrix:format=duration", "-of", "json", path}
	if err := runVideoTool(ctx, tool, &output, args...); err != nil {
		return videoProbe{}, err
	}
	var info videoProbe
	if json.Unmarshal(output.Bytes(), &info) != nil {
		return videoProbe{}, ErrVideo
	}
	return info, nil
}

// prepareVideo owns its staging directory until every subprocess has joined.
// Outputs are bounded display derivatives, not archives of the uploaded source.
func prepareVideo(ctx context.Context, input io.Reader) ([]byte, []byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, nil, ErrVideoTools
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		return nil, nil, ErrVideoTools
	}
	ctx, cancel := context.WithTimeout(ctx, videoProcessingTimeout)
	defer cancel()
	directory, err := os.MkdirTemp("", "thoughts-video-")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(directory)
	source := filepath.Join(directory, "source.mp4")
	file, err := os.OpenFile(source, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return nil, nil, err
	}
	size, copyErr := io.Copy(file, io.LimitReader(input, MaxVideoBytes+1))
	if copyErr == nil {
		copyErr = ctx.Err()
	}
	if copyErr == nil {
		copyErr = validateVideoContainer(file, size)
	}
	closeErr := file.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return nil, nil, err
	}
	info, err := probeVideo(ctx, ffprobe, source)
	if err != nil {
		return nil, nil, err
	}
	if err = info.validate(false); err != nil {
		return nil, nil, err
	}
	stream, _ := info.video()
	rotation, _ := stream.displayRotation()
	sar := videoRatio(stream.SampleAspectRatio, ":")
	if !finitePositive(sar) {
		sar = 1
	}
	width, height := int(math.Round(float64(stream.Width)*sar)), stream.Height
	if rotation == 90 || rotation == 270 {
		width, height = height, width
	}
	width, height = storedVideoDimensions(width, height)
	fps := math.Min(MaxStoredVideoFPS, videoRatio(stream.AvgFrameRate, "/"))
	output := filepath.Join(directory, "display.mp4")
	if err := os.WriteFile(output, nil, 0600); err != nil {
		return nil, nil, err
	}
	// All arguments and paths are application-controlled. Explicit MOV input,
	// disabled external references and the preflight dref check complement the
	// protocol allowlist; none of them constitute OS-level filesystem isolation.
	args := []string{
		"-nostdin", "-hide_banner", "-loglevel", "error",
		"-max_alloc", strconv.Itoa(maxVideoAllocation), "-threads", "1", "-filter_threads", "1",
		"-protocol_whitelist", "file", "-f", "mov", "-enable_drefs", "0", "-export_all", "0",
		"-max_pixels", strconv.Itoa(MaxVideoPixels), "-i", source,
		"-map", fmt.Sprintf("0:%d", stream.Index), "-an", "-sn", "-dn",
		"-map_metadata", "-1", "-map_metadata:s:v", "-1", "-map_chapters", "-1", "-t", "30",
		"-vf", fmt.Sprintf("scale=%d:%d:in_color_matrix=bt709:out_color_matrix=bt709:out_range=tv,setsar=1,fps=%.8f", width, height, fps),
		// Level 3.2 accommodates macroblock padding for non-16-aligned
		// shapes within the same 720p pixel budget; output limits remain stricter.
		"-c:v", "libx264", "-threads", "1", "-pix_fmt", "yuv420p", "-profile:v", "baseline", "-level:v", "3.2",
		"-preset", "veryfast", "-crf", "23", "-maxrate", "3M", "-bufsize", "6M", "-g", "48",
		"-color_primaries", "bt709", "-color_trc", "bt709", "-colorspace", "bt709", "-color_range", "tv",
		"-bsf:v", "filter_units=remove_types=6", "-movflags", "+faststart",
		"-fs", strconv.Itoa(MaxStoredVideoBytes), "-y", output,
	}

	if err = runVideoTool(ctx, ffmpeg, io.Discard, args...); err != nil {
		return nil, nil, err
	}
	prepared, err := probeVideo(ctx, ffprobe, output)
	if err != nil {
		return nil, nil, err
	}
	if err = prepared.validate(true); err != nil {
		return nil, nil, err
	}
	body, err := readBoundedVideoAsset(output, MaxStoredVideoBytes)
	if err != nil {
		return nil, nil, err
	}
	if err = inspectVideoContainer(bytes.NewReader(body), int64(len(body)), true); err != nil {
		return nil, nil, err
	}
	if err = runVideoTool(ctx, ffmpeg, io.Discard, "-nostdin", "-v", "error", "-xerror", "-max_alloc", strconv.Itoa(maxVideoAllocation), "-threads", "1", "-protocol_whitelist", "file", "-f", "mov", "-enable_drefs", "0", "-max_pixels", strconv.Itoa(MaxStoredVideoPixels), "-i", output, "-map", "0:v:0", "-an", "-sn", "-dn", "-f", "null", "-"); err != nil {
		return nil, nil, err
	}
	poster, err := prepareVideoPoster(ctx, ffmpeg, output, directory, prepared)
	if err != nil {
		return nil, nil, err
	}
	return body, poster, ctx.Err()
}

func prepareVideoPoster(ctx context.Context, ffmpeg, videoPath, directory string, info videoProbe) ([]byte, error) {
	stream, _ := info.video()
	path := filepath.Join(directory, "poster.rgb")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		return nil, err
	}
	// JPEG decoders assume the JPEG YCbCr matrix, not the video's Rec.709
	// matrix. Extract one bounded RGB frame explicitly, then let Go's encoder
	// perform its standard JPEG conversion. No source metadata can enter this
	// raw representation, and no frames are extracted during recurring playback.
	args := []string{
		"-nostdin", "-v", "error", "-max_alloc", strconv.Itoa(maxVideoAllocation), "-threads", "1", "-filter_threads", "1",
		"-protocol_whitelist", "file", "-f", "mov", "-enable_drefs", "0", "-max_pixels", strconv.Itoa(MaxStoredVideoPixels), "-i", videoPath,
		"-map", "0:v:0", "-frames:v", "1", "-an", "-sn", "-dn", "-map_metadata", "-1",
		"-vf", "scale=in_color_matrix=bt709:in_range=tv:out_range=pc,format=rgb24",
		"-pix_fmt", "rgb24", "-threads", "1", "-f", "rawvideo", "-fs", strconv.Itoa(MaxStoredVideoPixels * 3), "-y", path,
	}
	if err := runVideoTool(ctx, ffmpeg, io.Discard, args...); err != nil {
		return nil, err
	}
	pixels, err := readBoundedVideoAsset(path, MaxStoredVideoPixels*3)
	if err != nil {
		return nil, err
	}
	if len(pixels) != stream.Width*stream.Height*3 {
		return nil, ErrVideo
	}
	// Video pixels are opaque. RGBA also uses the JPEG encoder's direct
	// pixel path rather than allocating an interface color for every pixel.
	decoded := image.NewRGBA(image.Rect(0, 0, stream.Width, stream.Height))
	for pixel := 0; pixel < stream.Width*stream.Height; pixel++ {
		copy(decoded.Pix[pixel*4:pixel*4+3], pixels[pixel*3:pixel*3+3])
		decoded.Pix[pixel*4+3] = 255
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var poster bytes.Buffer
	if err := jpeg.Encode(&poster, decoded, &jpeg.Options{Quality: backgroundJPEGQuality}); err != nil {
		return nil, ErrVideo
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return poster.Bytes(), nil
}

func readBoundedVideoAsset(path string, limit int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > limit {
		return nil, ErrVideo
	}
	return body, nil
}
