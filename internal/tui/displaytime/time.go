// Package displaytime formats TUI dates in the temporary Pacific display timezone.
package displaytime

import (
	"errors"
	"strings"
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

const InputLayout = "2006-01-02 03:04:05 PM"
const InputHelp = "Enter a Pacific timestamp: YYYY-MM-DD HH:MM:SS AM/PM. For a repeated fall-back hour, append PDT or PST."

// FormatInput includes a zone only when the Pacific fall-back hour repeats.
func FormatInput(value time.Time) string {
	layout := InputLayout
	if repeatedHour(value) {
		layout += " MST"
	}
	return Format(value, layout)
}

func repeatedHour(value time.Time) bool {
	text := Format(value, InputLayout)
	return Format(value.Add(time.Hour), InputLayout) == text || Format(value.Add(-time.Hour), InputLayout) == text
}

// ParseInput rejects nonexistent local times and requires a zone to resolve
// the repeated Pacific hour instead of silently choosing an instant.
func ParseInput(value string) (time.Time, error) {
	layout := InputLayout
	if strings.HasSuffix(value, " PST") || strings.HasSuffix(value, " PDT") {
		layout += " MST"
	}
	parsed, err := time.ParseInLocation(layout, value, pacific)
	if err != nil || Format(parsed, layout) != value || (layout == InputLayout && repeatedHour(parsed)) {
		return time.Time{}, errors.New(InputHelp)
	}
	return parsed.UTC(), nil
}
