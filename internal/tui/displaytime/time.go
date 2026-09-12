// Package displaytime formats TUI dates in the temporary Pacific display timezone.
package displaytime

import (
	"errors"
	"time"
	_ "time/tzdata" // Available before loading the location, even without system zone files.
)

// Load once for all views; this location is never changed after initialization.
var pacific = func() *time.Location {
	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic("Could not load the application display timezone.")
	}
	return location
}()

// Format preserves the instant and formats it using Pacific daylight-saving rules.
func Format(value time.Time, layout string) string {
	return value.In(pacific).Format(layout)
}

func Day(value time.Time) time.Time {
	local := value.In(pacific)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, pacific)
}

const InputLayout = "2006-01-02 15:04:05 -07:00"

// ParseInput requires an explicit offset, including during the repeated DST
// hour, and round-trips through Pacific to reject nonexistent local times.
func ParseInput(value string) (time.Time, error) {
	parsed, err := time.Parse(InputLayout, value)
	if err != nil || Format(parsed, InputLayout) != value {
		return time.Time{}, errors.New("enter a Pacific date/time and matching offset: YYYY-MM-DD HH:MM:SS ±HH:MM")
	}
	return parsed.UTC(), nil
}
