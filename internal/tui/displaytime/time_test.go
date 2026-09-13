package displaytime

import (
	"testing"
	"time"
)

func TestFormat(t *testing.T) {
	for _, tt := range []struct {
		name, input, layout, want string
	}{
		{"winter uses PST and PM", "2026-01-03T00:04:05Z", "Jan 2, 2006 3:04:05 PM MST", "Jan 2, 2026 4:04:05 PM PST"},
		{"summer uses PDT and AM", "2026-07-02T16:04:05Z", "Jan 2, 2006 3:04 PM MST", "Jul 2, 2026 9:04 AM PDT"},
		{"date rolls into previous year", "2026-01-01T04:00:00Z", "Jan 2, 2006", "Dec 31, 2025"},
		{"midnight uses twelve AM", "2026-01-02T08:00:00Z", "3:04 PM MST", "12:00 AM PST"},
		{"noon uses twelve PM", "2026-01-02T20:00:00Z", "3:04 PM MST", "12:00 PM PST"},
		{"before spring transition", "2026-03-08T09:59:59Z", "3:04:05 PM MST", "1:59:59 AM PST"},
		{"after spring transition", "2026-03-08T10:00:00Z", "3:04:05 PM MST", "3:00:00 AM PDT"},
		{"before fall transition", "2026-11-01T08:59:59Z", "3:04:05 PM MST", "1:59:59 AM PDT"},
		{"after fall transition", "2026-11-01T09:00:00Z", "3:04:05 PM MST", "1:00:00 AM PST"},
		{"input offset preserves the instant", "2026-07-02T18:04:05+02:00", "3:04 PM MST", "9:04 AM PDT"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input, err := time.Parse(time.RFC3339, tt.input)
			if err != nil {
				t.Fatal(err)
			}
			original := input
			if got := Format(input, tt.layout); got != tt.want {
				t.Errorf("got timestamp %q, want %q", got, tt.want)
			}
			if input != original {
				t.Errorf("got input %v, want unchanged %v", input, original)
			}
		})
	}
}

func TestCalendarInput(t *testing.T) {
	t.Run("friendly input preserves UTC instants including both repeated hours", func(t *testing.T) {
		for _, utc := range []string{"2026-09-12T05:47:00Z", "2026-01-02T08:00:00Z", "2026-07-02T19:00:00Z", "2026-11-01T08:30:00Z", "2026-11-01T09:30:00Z"} {
			want, err := time.Parse(time.RFC3339, utc)
			if err != nil {
				t.Fatal(err)
			}
			input := FormatInput(want)
			got, err := ParseInput(input)
			if err != nil || !got.Equal(want) || got.Location() != time.UTC {
				t.Fatalf("got %v, %v from %q, want UTC %v", got, err, input, want)
			}
		}
	})
	for _, test := range []struct {
		name, input string
		valid       bool
	}{
		{"ordinary Pacific time", "2026-09-11 10:47:00 PM", true},
		{"first repeated hour", "2026-11-01 01:30:00 AM PDT", true},
		{"second repeated hour", "2026-11-01 01:30:00 AM PST", true},
		{"ambiguous hour needs a zone", "2026-11-01 01:30:00 AM", false},
		{"missing spring hour", "2026-03-08 02:30:00 AM", false},
		{"wrong summer zone", "2026-07-01 12:00:00 PM PST", false},
		{"invalid date", "2026-02-30 12:00:00 PM", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseInput(test.input)
			if (err == nil) != test.valid {
				t.Fatalf("got error %v, want valid %v", err, test.valid)
			}
		})
	}
	t.Run("calendar days use local midnights rather than fixed durations", func(t *testing.T) {
		for _, test := range []struct {
			date  string
			hours time.Duration
		}{{"2026-03-08 12:00:00 PM", 23}, {"2026-11-01 12:00:00 PM", 25}} {
			at, err := ParseInput(test.date)
			if err != nil {
				t.Fatal(err)
			}
			day := Day(at)
			if got := day.AddDate(0, 0, 1).Sub(day); got != test.hours*time.Hour {
				t.Fatalf("got day length %v, want %dh", got, test.hours)
			}
		}
	})
}
