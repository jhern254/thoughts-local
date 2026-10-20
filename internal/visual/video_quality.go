package visual

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// VideoQuality selects preparation ceilings, never the properties of an existing asset.
type VideoQuality string

const (
	VideoQualityEfficient VideoQuality = "efficient"
	VideoQualityHigh      VideoQuality = "high"
	VideoQualityMaximum   VideoQuality = "maximum"
)

var ErrVideoQuality = errors.New("choose Efficient, High quality, or Maximum video quality")

type videoQualityLimits struct {
	MaxDimension int
	MaxPixels    int
	MaxFPS       int64
	MaxBytes     int
	MaxBitrate   int64
}

func (quality VideoQuality) limits() (videoQualityLimits, error) {
	switch quality {
	case VideoQualityEfficient:
		return videoQualityLimits{1280, 1280 * 720, 30, 16 << 20, 3_000_000}, nil
	case VideoQualityHigh:
		return videoQualityLimits{1920, 1920 * 1080, 30, 32 << 20, 6_000_000}, nil
	case VideoQualityMaximum:
		return videoQualityLimits{1920, 1920 * 1080, 60, 64 << 20, 10_000_000}, nil
	default:
		return videoQualityLimits{}, ErrVideoQuality
	}
}
func (quality VideoQuality) Valid() bool { _, err := quality.limits(); return err == nil }
func (quality VideoQuality) Label() string {
	switch quality {
	case VideoQualityEfficient:
		return "Efficient"
	case VideoQualityHigh:
		return "High quality"
	case VideoQualityMaximum:
		return "Maximum"
	default:
		return "Unknown"
	}
}
func (quality VideoQuality) Step(direction int) VideoQuality {
	tiers := [...]VideoQuality{VideoQualityEfficient, VideoQualityHigh, VideoQualityMaximum}
	for index, tier := range tiers {
		if tier == quality {
			return tiers[max(0, min(len(tiers)-1, index+direction))]
		}
	}
	return VideoQualityEfficient
}

// VideoFrameRate keeps NTSC and other fractional rates exact through encoding.
// Values originate in FFmpeg AVRational fields, whose components are signed 32-bit integers.
type VideoFrameRate struct {
	Numerator   int64 `json:"numerator"`
	Denominator int64 `json:"denominator"`
}

func parseVideoFrameRate(text string) (VideoFrameRate, error) {
	numerator, denominator, ok := strings.Cut(text, "/")
	if !ok {
		return VideoFrameRate{}, ErrVideo
	}
	num, err := strconv.ParseInt(numerator, 10, 32)
	if err != nil || num <= 0 {
		return VideoFrameRate{}, ErrVideo
	}
	den, err := strconv.ParseInt(denominator, 10, 32)
	if err != nil || den <= 0 {
		return VideoFrameRate{}, ErrVideo
	}
	a, b := num, den
	for b != 0 {
		a, b = b, a%b
	}
	rate := VideoFrameRate{num / a, den / a}
	if !rate.Valid() {
		return VideoFrameRate{}, ErrVideo
	}
	return rate, nil
}
func (rate VideoFrameRate) Valid() bool {
	return rate.Numerator > 0 && rate.Numerator <= math.MaxInt32 && rate.Denominator > 0 && rate.Denominator <= math.MaxInt32 && rate.Numerator <= MaxVideoFPS*rate.Denominator
}
func (rate VideoFrameRate) String() string {
	return fmt.Sprintf("%d/%d", rate.Numerator, rate.Denominator)
}
func (rate VideoFrameRate) Display() string {
	return strconv.FormatFloat(float64(rate.Numerator)/float64(rate.Denominator), 'f', 2, 64)
}

type videoOutputPlan struct {
	Width      int
	Height     int
	FrameRate  VideoFrameRate
	FrameCount int64
	Level      int
	Quality    VideoQuality
	Limits     videoQualityLimits
}

func displayVideoDimensions(stream videoStream) (int, int, error) {
	if stream.Width < 2 || stream.Height < 2 {
		return 0, 0, ErrVideo
	}
	rotation, err := stream.displayRotation()
	if err != nil {
		return 0, 0, err
	}
	sar := videoRatio(stream.SampleAspectRatio, ":")
	if stream.SampleAspectRatio == "" || stream.SampleAspectRatio == "N/A" {
		sar = 1
	}
	if !finitePositive(sar) || sar < .25 || sar > 4 {
		return 0, 0, ErrVideo
	}
	// Square pixels must preserve display aspect without inventing source samples.
	width, height := float64(stream.Width), float64(stream.Height)
	if sar < 1 {
		width *= sar
	} else {
		height /= sar
	}
	w, h := int(width), int(height)
	if rotation == 90 || rotation == 270 {
		w, h = h, w
	}
	if w < 2 || h < 2 {
		return 0, 0, ErrVideo
	}
	return w, h, nil
}

func planVideoOutput(stream videoStream, trim VideoTrim, quality VideoQuality) (videoOutputPlan, error) {
	limits, err := quality.limits()
	if err != nil {
		return videoOutputPlan{}, err
	}
	rate, err := parseVideoFrameRate(stream.AvgFrameRate)
	if err != nil {
		return videoOutputPlan{}, err
	}
	if rate.Numerator > limits.MaxFPS*rate.Denominator {
		rate = VideoFrameRate{limits.MaxFPS, 1}
	}
	width, height, err := displayVideoDimensions(stream)
	if err != nil {
		return videoOutputPlan{}, err
	}
	width, height = fitVideoDimensions(width, height, limits)
	if width < 2 || height < 2 {
		return videoOutputPlan{}, ErrVideo
	}
	span := trim.EndMS - trim.StartMS
	if span < 1000 || span > MaxVideoDuration*1000 {
		return videoOutputPlan{}, ErrVideo
	}
	// Round down except when needed to keep a one-second loop at fractional FPS.
	frames := max((rate.Numerator+rate.Denominator-1)/rate.Denominator, span*rate.Numerator/(1000*rate.Denominator))
	if frames*rate.Denominator > MaxVideoDuration*rate.Numerator {
		return videoOutputPlan{}, ErrVideo
	}
	level, err := videoH264Level(width, height, rate, limits.MaxBitrate, limits.MaxBitrate*2)
	if err != nil {
		return videoOutputPlan{}, err
	}
	return videoOutputPlan{width, height, rate, frames, level, quality, limits}, nil
}

func fitVideoDimensions(width, height int, limits videoQualityLimits) (int, int) {
	scale := math.Min(1, math.Min(float64(limits.MaxDimension)/float64(max(width, height)), math.Sqrt(float64(limits.MaxPixels)/(float64(width)*float64(height)))))
	return int(float64(width)*scale) / 2 * 2, int(float64(height)*scale) / 2 * 2
}

// H.264 Annex A bounds padded 16x16 macroblocks, including reference pictures
// and the configured VBV rate/buffer. A nominal "720p" name is insufficient:
// 1228x750 at 60 FPS exceeds level 3.2's macroblock processing ceiling.
func videoH264Level(width, height int, rate VideoFrameRate, bitrate, buffer int64) (int, error) {
	levels := [...]struct {
		number                                 int
		frame, perSecond, dpb, bitrate, buffer int64
	}{
		{31, 3600, 108000, 18000, 14_000_000, 14_000_000},
		{32, 5120, 216000, 20480, 20_000_000, 20_000_000},
		{40, 8192, 245760, 32768, 20_000_000, 25_000_000},
		{42, 8704, 522240, 34816, 50_000_000, 62_500_000},
	}
	horizontal, vertical := int64((width+15)/16), int64((height+15)/16)
	frame := horizontal * vertical
	for _, level := range levels {
		if frame <= level.frame && frame*rate.Numerator <= level.perSecond*rate.Denominator && frame <= level.dpb && horizontal*horizontal <= 8*level.frame && vertical*vertical <= 8*level.frame && bitrate <= level.bitrate && buffer <= level.buffer {
			return level.number, nil
		}
	}
	return 0, ErrVideo
}

// FFmpeg's H.264 frame buffers may pad rows/columns for SIMD access. This
// allocation bound permits that padding; validated visible pixels still obey
// the tier's stricter geometry limit, and raw poster bytes use visible pixels.
func videoDecoderPixelLimit(width, height int) int {
	return ((width + 63) / 64 * 64) * ((height + 63) / 64 * 64)
}
