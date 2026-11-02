package goals

import "testing"

func TestTargetSeconds(t *testing.T) {
	for _, tc := range []struct {
		hours, minutes string
		want           int64
		valid          bool
	}{
		{"1", "30", 5400, true}, {"", "90", 5400, true}, {"", "", 0, false}, {"-1", "30", 0, false}, {"x", "0", 0, false}, {"9223372036854775807", "0", 0, false},
	} {
		t.Run(tc.hours+"h"+tc.minutes+"m", func(t *testing.T) {
			got, err := targetSeconds(tc.hours, tc.minutes)
			if (err == nil) != tc.valid || (tc.valid && got != tc.want) {
				t.Fatalf("got %d, %v; want %d, valid=%v", got, err, tc.want, tc.valid)
			}
		})
	}
}
