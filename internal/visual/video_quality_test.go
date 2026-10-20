package visual

import (
	"errors"
	"testing"
)

func TestVideoOutputPlan(t *testing.T) {
	t.Run("preserves intermediate geometry and rational rates within tier ceilings", func(t *testing.T) {
		cases := []struct {
			name                  string
			width, height         int
			rate                  string
			quality               VideoQuality
			wantWidth, wantHeight int
			wantRate              string
			wantLevel             int
		}{
			{"480p", 854, 480, "24/1", VideoQualityEfficient, 854, 480, "24/1", 31},
			{"720p30", 1280, 720, "30/1", VideoQualityEfficient, 1280, 720, "30/1", 31},
			{"720p60", 1280, 720, "60/1", VideoQualityMaximum, 1280, 720, "60/1", 32},
			{"900p25", 1600, 900, "25/1", VideoQualityHigh, 1600, 900, "25/1", 40},
			{"1080p fractional", 1920, 1080, "30000/1001", VideoQualityHigh, 1920, 1080, "30000/1001", 40},
			{"1080p fractional sixty", 1920, 1080, "60000/1001", VideoQualityMaximum, 1920, 1080, "60000/1001", 42},
			{"1080p60", 1920, 1080, "60/1", VideoQualityMaximum, 1920, 1080, "60/1", 42},
			{"portrait", 1080, 1920, "30/1", VideoQualityHigh, 1080, 1920, "30/1", 40},
			{"efficient downscale", 1920, 1080, "60/1", VideoQualityEfficient, 1280, 720, "30/1", 31},
			{"no enlargement", 1280, 720, "24000/1001", VideoQualityMaximum, 1280, 720, "24000/1001", 32},
			{"even dimensions", 853, 479, "25/1", VideoQualityEfficient, 852, 478, "25/1", 31},
			{"padded macroblocks", 1228, 750, "60/1", VideoQualityMaximum, 1228, 750, "60/1", 40},
			{"padded 1080 class", 1846, 1122, "30/1", VideoQualityHigh, 1846, 1122, "30/1", 42},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				stream := videoStream{Width: tc.width, Height: tc.height, AvgFrameRate: tc.rate, SampleAspectRatio: "1:1"}
				plan, err := planVideoOutput(stream, VideoTrim{EndMS: 2000}, tc.quality)
				if err != nil {
					t.Fatal(err)
				}
				if plan.Width != tc.wantWidth || plan.Height != tc.wantHeight || plan.FrameRate.String() != tc.wantRate || plan.Level != tc.wantLevel {
					t.Fatalf("plan=%+v rate=%s, want %dx%d %s level %d", plan, plan.FrameRate, tc.wantWidth, tc.wantHeight, tc.wantRate, tc.wantLevel)
				}
			})
		}
	})
	t.Run("rejects unknown tiers and invalid frame rates", func(t *testing.T) {
		for _, rate := range []string{"0/0", "0/1", "60/0", "-1/1", "NaN/1", "1e6/1", "2147483648/1", "61/1"} {
			_, err := planVideoOutput(videoStream{Width: 1280, Height: 720, AvgFrameRate: rate, SampleAspectRatio: "1:1"}, VideoTrim{EndMS: 1000}, VideoQualityMaximum)
			if !errors.Is(err, ErrVideo) {
				t.Fatalf("rate %q: got %v, want ErrVideo", rate, err)
			}
		}
		_, err := planVideoOutput(videoStream{}, VideoTrim{EndMS: 1000}, VideoQuality("unknown"))
		if !errors.Is(err, ErrVideoQuality) {
			t.Fatalf("got %v, want ErrVideoQuality", err)
		}
	})
	t.Run("uses exact frame counts at fractional interval boundaries", func(t *testing.T) {
		for _, tc := range []struct {
			rate   string
			end    int64
			frames int64
		}{{"30000/1001", 1000, 30}, {"30000/1001", 30000, 899}, {"60000/1001", 30000, 1798}, {"24000/1001", 1250, 29}} {
			plan, err := planVideoOutput(videoStream{Width: 1280, Height: 720, AvgFrameRate: tc.rate, SampleAspectRatio: "1:1"}, VideoTrim{EndMS: tc.end}, VideoQualityMaximum)
			if err != nil {
				t.Fatal(err)
			}
			if plan.FrameCount != tc.frames {
				t.Fatalf("%s %dms: got %d frames, want %d", tc.rate, tc.end, plan.FrameCount, tc.frames)
			}
		}
	})
}

func TestPreparedVideoPackets(t *testing.T) {
	plan, err := planVideoOutput(videoStream{Width: 640, Height: 480, AvgFrameRate: "1/1", SampleAspectRatio: "1:1"}, VideoTrim{EndMS: 2000}, VideoQualityEfficient)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("accepts bounded bytes at the encoded cadence", func(t *testing.T) {
		if err := validatePreparedPackets("pts_time=0.000000|size=10000\npts_time=1.000000|size=10000\n", plan); err != nil {
			t.Fatal(err)
		}
	})
	for name, packets := range map[string]string{
		"oversized initial burst":    "pts_time=0.000000|size=800000\npts_time=1.000000|size=10000",
		"overdrawn sustained budget": "pts_time=0.000000|size=700000\npts_time=1.000000|size=700000",
		"wrong cadence":              "pts_time=0.000000|size=10000\npts_time=0.500000|size=10000",
		"missing frame":              "pts_time=0.000000|size=10000",
		"invalid timestamp":          "pts_time=NaN|size=10000\npts_time=1.000000|size=10000",
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			if err := validatePreparedPackets(packets, plan); !errors.Is(err, ErrVideo) {
				t.Fatalf("got %v, want ErrVideo", err)
			}
		})
	}
}
