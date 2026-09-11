// Package displaytime formats TUI dates in the temporary Pacific display timezone.
package displaytime

import (
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
