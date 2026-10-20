package visual

import (
	"context"
	"math"
	"strconv"
	"strings"
)

// Prepared packets are few (at most 1800) and bounded by the existing probe
// output cap. Validate their actual byte/timestamp envelope rather than treating
// an encoder option or average bitrate declaration as evidence of compliance.
func validateVideoPackets(ctx context.Context, ffprobe, path string, plan videoOutputPlan) error {
	var output videoProbeBuffer
	args := []string{"-v", "error", "-max_alloc", strconv.Itoa(maxVideoAllocation), "-protocol_whitelist", "file", "-f", "mov", "-enable_drefs", "0", "-threads", "1", "-max_pixels", strconv.Itoa(videoDecoderPixelLimit(plan.Width, plan.Height)), "-select_streams", "v:0", "-show_packets", "-show_entries", "packet=pts_time,size", "-of", "compact=p=0:nk=0", path}
	if err := runVideoTool(ctx, ffprobe, &output, args...); err != nil {
		return err
	}
	return validatePreparedPackets(output.String(), plan)
}

func validatePreparedPackets(packets string, plan videoOutputPlan) error {
	count := int64(0)
	available := float64(plan.Limits.MaxBitrate * 2)
	previous := float64(0)
	for _, line := range strings.Split(strings.TrimSpace(packets), "\n") {
		fields := strings.Split(line, "|")
		if len(fields) != 2 || count >= plan.FrameCount {
			return ErrVideo
		}
		timestamp := math.NaN()
		size := int64(0)
		for _, field := range fields {
			key, value, ok := strings.Cut(field, "=")
			if !ok {
				return ErrVideo
			}
			switch key {
			case "pts_time":
				parsed, err := strconv.ParseFloat(value, 64)
				if err != nil {
					return ErrVideo
				}
				timestamp = parsed
			case "size":
				parsed, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return ErrVideo
				}
				size = parsed
			default:
				return ErrVideo
			}
		}
		expected := float64(count*plan.FrameRate.Denominator) / float64(plan.FrameRate.Numerator)
		if math.IsNaN(timestamp) || math.IsInf(timestamp, 0) || math.Abs(timestamp-expected) > .000001 || size <= 0 || size > int64(plan.Limits.MaxBytes) {
			return ErrVideo
		}
		available = math.Min(float64(plan.Limits.MaxBitrate*2), available+float64(plan.Limits.MaxBitrate)*(timestamp-previous))
		if float64(size*8) > available+8 {
			return ErrVideo
		}
		available -= float64(size * 8)
		previous = timestamp
		count++
	}
	if count != plan.FrameCount {
		return ErrVideo
	}
	return nil
}
