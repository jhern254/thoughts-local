package visual

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	MaxVideoSourceBytes int64 = 8 << 30
	MaxVideoDimension         = 4096
	MaxVideoPixels            = 9_000_000
	MaxVideoFPS               = 60
	// Duration bounds are source footage seconds, independent of playbackRate.
	MaxVideoDuration       = 30
	MaxVideoSourceDuration = 14400
	// Movie language/subtitle/attachment tracks are bounded at probe time;
	// preparation still maps only one verified moving-video stream.
	maxVideoStreams       = 64
	maxVideoDecodedPixels = 4_000_000_000
	// Per-preparation work includes seek preroll, independent of source length.
	maxVideoDecodedFrames    int64 = 7200
	MaxVideoTransferTime           = 20 * time.Minute
	MaxVideoTransferIdleTime       = 30 * time.Second
	MaxVideoProcessingTime         = 2 * time.Minute
	maxVideoProbeBytes             = 64 << 10
	maxVideoProbeInputBytes        = 5_000_000
	maxVideoProbeAnalyzeTime       = 5 * time.Second
	maxVideoProbePackets           = 2500
	// This limits each FFmpeg allocation, not total process memory. Frame size,
	// stream count, decoder threads and duration bound the remaining work.
	maxVideoAllocation = 64 << 20
)

var ErrVideo = errors.New("choose a supported self-contained SDR MP4, MOV or MKV within the video limits")
var ErrVideoContainer = fmt.Errorf("%w: invalid or externally dependent container", ErrVideo)
var ErrVideoProtected = fmt.Errorf("%w: protected movie", ErrVideo)
var ErrVideoTracks = fmt.Errorf("%w: ambiguous video tracks", ErrVideo)
var ErrVideoCodec = fmt.Errorf("%w: unsupported codec", ErrVideo)
var ErrVideoColor = fmt.Errorf("%w: unsupported color format", ErrVideo)
var ErrVideoDimensions = fmt.Errorf("%w: unsupported resolution", ErrVideo)
var ErrVideoFrameRate = fmt.Errorf("%w: unsupported frame rate", ErrVideo)
var ErrVideoDuration = fmt.Errorf("%w: unsupported duration", ErrVideo)
var ErrVideoSourceSize = errors.New("video source exceeds 8 GiB")
var ErrVideoTools = errors.New("video backgrounds require FFmpeg and ffprobe")
var ErrPlayback = errors.New("video speed must be between 25 and 100 percent")

type videoStream struct {
	Index       int `json:"index"`
	Disposition struct {
		Default     int `json:"default"`
		AttachedPic int `json:"attached_pic"`
	} `json:"disposition"`
	CodecName         string `json:"codec_name"`
	CodecType         string `json:"codec_type"`
	Profile           string `json:"profile"`
	Level             int    `json:"level"`
	Refs              int    `json:"refs"`
	FrameCount        string `json:"nb_frames"`
	Width             int    `json:"width"`
	Height            int    `json:"height"`
	PixFmt            string `json:"pix_fmt"`
	AvgFrameRate      string `json:"avg_frame_rate"`
	SampleAspectRatio string `json:"sample_aspect_ratio"`
	Duration          string `json:"duration"`
	StartTime         string `json:"start_time"`
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
	container videoSourceContainer
	Streams   []videoStream `json:"streams"`
	Format    struct {
		Duration  string `json:"duration"`
		StartTime string `json:"start_time"`
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

// Attached covers are not moving-video candidates. A unique default track is
// authoritative; otherwise multiple moving tracks need an explicit future UI.
func (info videoProbe) video() (videoStream, bool) {
	var selected, preferred videoStream
	count, defaults := 0, 0
	for _, stream := range info.Streams {
		if stream.CodecType != "video" || stream.Disposition.AttachedPic != 0 {
			continue
		}
		selected = stream
		count++
		if stream.Disposition.Default != 0 {
			preferred = stream
			defaults++
		}
	}
	if count == 1 {
		return selected, true
	}
	return preferred, defaults == 1
}

// Matroska commonly omits stream duration. Its segment duration denotes the
// timestamp endpoint; subtract the selected video's origin, not an audio origin.
func (info videoProbe) sourceDuration() (float64, error) {
	stream, ok := info.video()
	if !ok {
		return 0, ErrVideoTracks
	}
	if stream.Duration != "" && stream.Duration != "N/A" {
		duration, err := strconv.ParseFloat(stream.Duration, 64)
		if err == nil && finitePositive(duration) {
			return duration, nil
		}
		return 0, ErrVideoDuration
	}
	if info.container != sourceMatroska {
		return 0, ErrVideoDuration
	}
	end, err := strconv.ParseFloat(info.Format.Duration, 64)
	if err != nil || !finitePositive(end) {
		return 0, ErrVideoDuration
	}
	start := float64(0)
	if stream.StartTime != "" && stream.StartTime != "N/A" {
		start, err = strconv.ParseFloat(stream.StartTime, 64)
		if err != nil || math.IsNaN(start) || math.IsInf(start, 0) {
			return 0, ErrVideoDuration
		}
	}
	duration := end - start
	if !finitePositive(duration) {
		return 0, ErrVideoDuration
	}
	return duration, nil
}

func (info videoProbe) validate(prepared bool) error {
	stream, ok := info.video()
	if !ok {
		return ErrVideoTracks
	}
	if len(info.Streams) > maxVideoStreams {
		return ErrVideoContainer
	}
	duration, err := info.sourceDuration()
	maxDuration := MaxVideoSourceDuration
	if prepared {
		maxDuration = MaxVideoDuration
	}
	if info.container == sourceMOV && info.Format.Duration != "" {
		containerDuration, parseErr := strconv.ParseFloat(info.Format.Duration, 64)
		if parseErr != nil || !finitePositive(containerDuration) || containerDuration < 1 || containerDuration > float64(maxDuration) {
			return ErrVideoDuration
		}
	}
	rate, rateErr := parseVideoFrameRate(stream.AvgFrameRate)
	fps := float64(rate.Numerator) / float64(rate.Denominator)
	sar := videoRatio(stream.SampleAspectRatio, ":")
	if stream.SampleAspectRatio == "N/A" || stream.SampleAspectRatio == "" {
		sar = 1
	}
	if err != nil || !finitePositive(duration) || duration < 1 || duration > float64(maxDuration) {
		return ErrVideoDuration
	}
	if rateErr != nil || !finitePositive(fps) || fps > MaxVideoFPS {
		return ErrVideoFrameRate
	}
	if !finitePositive(sar) || sar < .25 || sar > 4 {
		return ErrVideo
	}
	switch stream.CodecName {
	case "h264", "hevc", "vp9", "av1":
	default:
		return ErrVideoCodec
	}
	if prepared && stream.CodecName != "h264" {
		return ErrVideoCodec
	}
	tenBitSDR := stream.CodecName == "hevc" && stream.PixFmt == "yuv420p10le"
	if stream.PixFmt != "yuv420p" && stream.PixFmt != "yuvj420p" && !tenBitSDR {
		return ErrVideoColor
	}
	// Ten-bit is not synonymous with HDR. Require explicit SDR declarations for
	// depth conversion rather than guessing the intent of an untagged 10-bit file.
	if tenBitSDR && (stream.ColorSpace != "bt709" || stream.ColorTransfer != "bt709" || stream.ColorPrimaries != "bt709") {
		return ErrVideoColor
	}
	if stream.Width < 2 || stream.Height < 2 || stream.Width > MaxVideoDimension || stream.Height > MaxVideoDimension || int64(stream.Width)*int64(stream.Height) > MaxVideoPixels {
		return ErrVideoDimensions
	}
	// The first supported path is SDR Rec.709. Unknown signaling is treated as
	// ordinary SDR Rec.709; explicitly different primaries/transfer are rejected,
	// never relabeled as Rec.709 without a reviewed conversion.
	for _, value := range []string{stream.ColorSpace, stream.ColorTransfer, stream.ColorPrimaries} {
		if value != "" && value != "unknown" && value != "bt709" {
			return ErrVideoColor
		}
	}
	if _, err := stream.displayRotation(); err != nil {
		return err
	}
	if prepared {
		limits, _ := VideoQualityMaximum.limits()
		if len(info.Streams) != 1 || stream.PixFmt != "yuv420p" || stream.Width%2 != 0 || stream.Height%2 != 0 || stream.Width > limits.MaxDimension || stream.Height > limits.MaxDimension || stream.Width*stream.Height > limits.MaxPixels || fps > float64(limits.MaxFPS) || sar != 1 || len(stream.SideData) > 0 {
			return ErrVideo
		}
		if stream.Profile != "Constrained Baseline" || (stream.Level != 31 && stream.Level != 32 && stream.Level != 40 && stream.Level != 42) {
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
		if side.Type == "Mastering display metadata" || side.Type == "Content light level metadata" || strings.Contains(side.Type, "DOVI") {
			return 0, ErrVideoColor
		}
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
	return probeVideoSource(ctx, tool, path, sourceMOV)
}

func probeVideoSource(ctx context.Context, tool, path string, container videoSourceContainer) (videoProbe, error) {
	var output videoProbeBuffer
	args := []string{
		"-v", "error", "-max_alloc", strconv.Itoa(maxVideoAllocation),
		"-protocol_whitelist", "file",
	}
	args = append(args, container.inputArgs()...)
	// Return technical facts only, never titles, attachment names, arbitrary tags
	// or codec-private blobs. Probe input and JSON output have separate budgets.
	const entries = "stream=index,codec_name,codec_type,profile,level,refs,nb_frames," +
		"width,height,pix_fmt,avg_frame_rate,sample_aspect_ratio,duration,start_time," +
		"color_space,color_transfer,color_primaries,color_range:" +
		"stream_disposition=default,attached_pic:" +
		"stream_side_data=side_data_type,rotation,displaymatrix:format=duration,start_time"
	args = append(args, []string{
		"-threads", "1", "-max_pixels", strconv.Itoa(MaxVideoPixels),
		"-max_streams", strconv.Itoa(maxVideoStreams),
		"-probesize", strconv.Itoa(maxVideoProbeInputBytes),
		"-analyzeduration", strconv.FormatInt(maxVideoProbeAnalyzeTime.Microseconds(), 10),
		"-max_probe_packets", strconv.Itoa(maxVideoProbePackets),
		"-show_entries", entries, "-of", "json", path,
	}...)

	if err := runVideoTool(ctx, tool, &output, args...); err != nil {
		return videoProbe{}, err
	}
	info := videoProbe{container: container}
	if json.Unmarshal(output.Bytes(), &info) != nil {
		return videoProbe{}, ErrVideo
	}
	return info, nil
}

// The caller owns interrupting a blocked Read (HTTP uses a read deadline).
// Checking each returned chunk also stops continuously arriving input promptly
// when the processing context expires, without a detached copying goroutine.
type videoSourceReader struct {
	ctx   context.Context
	input io.Reader
}

func (source videoSourceReader) Read(body []byte) (int, error) {
	if err := source.ctx.Err(); err != nil {
		return 0, err
	}
	return source.input.Read(body)
}

func encodeVideoLoop(ctx context.Context, ffmpeg, ffprobe, source, directory string, info videoProbe, trim VideoTrim, quality VideoQuality) (*PreparedVideo, error) {
	stream, _ := info.video()
	plan, err := planVideoOutput(stream, trim, quality)
	if err != nil {
		return nil, err
	}
	limits := plan.Limits
	width, height := plan.Width, plan.Height
	// Both pixel work and frame count remain finite even for tiny frames or
	// malformed timestamps. The first filter counts preroll before trimming.
	sourceFrameLimit := min(int64(maxVideoDecodedPixels)/(int64(stream.Width)*int64(stream.Height)), maxVideoDecodedFrames)
	output := filepath.Join(directory, "display.mp4")
	if err := os.WriteFile(output, nil, 0600); err != nil {
		return nil, err
	}
	// All arguments and paths are application-controlled. Explicit MOV input,
	// disabled external references and the preflight dref check complement the
	// protocol allowlist; none of them constitute OS-level filesystem isolation.
	args := []string{
		"-nostdin", "-hide_banner", "-loglevel", "error", "-xerror",
		"-max_alloc", strconv.Itoa(maxVideoAllocation), "-threads", "1", "-filter_threads", "1",
		"-protocol_whitelist", "file",
	}
	args = append(args, info.container.inputArgs()...)
	args = append(args, []string{
		"-max_pixels", strconv.Itoa(videoDecoderPixelLimit(stream.Width, stream.Height)), "-ss", info.inputPosition(trim.StartMS), "-noaccurate_seek", "-i", source,
		"-map", fmt.Sprintf("0:%d", stream.Index), "-an", "-sn", "-dn",
		"-map_metadata", "-1", "-map_metadata:s:v", "-1", "-map_chapters", "-1", "-t", videoSeconds(trim.EndMS - trim.StartMS), "-frames:v", strconv.FormatInt(plan.FrameCount, 10),
		"-vf", fmt.Sprintf("metadata=mode=add:key=thoughts.frame:value=1,metadata=mode=print:key=thoughts.frame:file='pipe\\:1':direct=1,trim=start=0:end=%s,setpts=PTS-STARTPTS,scale=%d:%d:in_color_matrix=bt709:out_color_matrix=bt709:out_range=tv,setsar=1,fps=%s", videoSeconds(trim.EndMS-trim.StartMS), width, height, plan.FrameRate.String()),
		"-c:v", "libx264", "-threads", "1", "-pix_fmt", "yuv420p", "-profile:v", "baseline", "-level:v", fmt.Sprintf("%d.%d", plan.Level/10, plan.Level%10), "-refs", "1", "-bf", "0",
		"-preset", "veryfast", "-crf", "23", "-maxrate", strconv.FormatInt(limits.MaxBitrate, 10), "-bufsize", strconv.FormatInt(limits.MaxBitrate*2, 10), "-g", strconv.FormatInt((plan.FrameRate.Numerator*2+plan.FrameRate.Denominator-1)/plan.FrameRate.Denominator, 10),
		"-color_primaries", "bt709", "-color_trc", "bt709", "-colorspace", "bt709", "-color_range", "tv",
		"-bsf:v", "filter_units=remove_types=6", "-movflags", "+faststart",
		"-fs", strconv.Itoa(limits.MaxBytes), "-y", output,
	}...)

	encodeCtx, cancelEncode := context.WithCancel(ctx)
	defer cancelEncode()
	budget := videoDecodeBudget{cancel: cancelEncode, maxFrames: sourceFrameLimit}
	if err := runVideoTool(encodeCtx, ffmpeg, &budget, args...); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if budget.err != nil {
			return nil, budget.err
		}
		return nil, err
	}
	if budget.frames == 0 || len(budget.line) != 0 || budget.wantValue {
		return nil, ErrVideo
	}
	prepared, err := probeVideo(ctx, ffprobe, output)
	if err != nil {
		return nil, err
	}
	if err = prepared.validateLoop(trim, plan); err != nil {
		return nil, err
	}
	file, err := os.Open(output)
	if err != nil {
		return nil, err
	}
	stat, err := file.Stat()
	if err == nil && stat.Size() > int64(limits.MaxBytes) {
		err = ErrVideo
	}
	if err == nil {
		err = inspectVideoContainer(file, stat.Size(), true)
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		return nil, err
	}
	if err := validateVideoPackets(ctx, ffprobe, output, plan); err != nil {
		return nil, err
	}
	if err := validateVideoDecode(ctx, ffmpeg, output, plan); err != nil {
		return nil, err
	}
	poster, err := prepareVideoPoster(ctx, ffmpeg, output, directory, prepared)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(directory, "poster.jpg"), poster, 0600); err != nil {
		return nil, err
	}
	if err := os.Remove(filepath.Join(directory, "poster.rgb")); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	duration, _ := strconv.ParseFloat(prepared.Streams[0].Duration, 64)
	return &PreparedVideo{
		directory: directory,
		info: PreparedVideoInfo{
			Trim:         trim,
			FrameCount:   plan.FrameCount,
			DurationMS:   int64(math.Round(duration * 1000)),
			VideoQuality: quality,
			FrameRate:    plan.FrameRate,
			Width:        width,
			Height:       height,
		},
	}, nil
}

func prepareVideoPoster(ctx context.Context, ffmpeg, videoPath, directory string, info videoProbe) ([]byte, error) {
	stream, _ := info.video()
	pixelsLimit := stream.Width * stream.Height
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
		"-protocol_whitelist", "file", "-f", "mov", "-enable_drefs", "0", "-max_pixels", strconv.Itoa(videoDecoderPixelLimit(stream.Width, stream.Height)), "-i", videoPath,
		"-map", "0:v:0", "-frames:v", "1", "-an", "-sn", "-dn", "-map_metadata", "-1",
		"-vf", "scale=in_color_matrix=bt709:in_range=tv:out_range=pc,format=rgb24",
		"-pix_fmt", "rgb24", "-threads", "1", "-f", "rawvideo", "-fs", strconv.Itoa(pixelsLimit * 3), "-y", path,
	}
	if err := runVideoTool(ctx, ffmpeg, io.Discard, args...); err != nil {
		return nil, err
	}
	pixels, err := readBoundedVideoAsset(path, pixelsLimit*3)
	if err != nil {
		return nil, err
	}
	return encodeVideoRGB(ctx, pixels, stream.Width, stream.Height)
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

func (info videoProbe) movingVideoCount() int {
	count := 0
	for _, stream := range info.Streams {
		if stream.CodecType == "video" && stream.Disposition.AttachedPic == 0 {
			count++
		}
	}
	return count
}
