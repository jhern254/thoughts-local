package events

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/timeline"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
)

func TestModel_TimelineTimestampLabels(t *testing.T) {
	at := time.Date(2026, 9, 11, 17, 0, 0, 0, time.UTC)
	completedEvent := func(id int64, start, end time.Time) timeline.Entry {
		return timeline.Entry{Event: &data.Event{
			EventID:   id,
			StartedAt: start,
			EndedAt:   &end,
		}}
	}
	ghost := func(observedAt time.Time) timeline.Entry {
		return timeline.Entry{ThoughtsOutsideEvents: &data.ThoughtGroupOutsideEvents{
			ObservedAt:   observedAt,
			ThoughtCount: 1,
		}}
	}
	for _, tc := range []struct {
		name    string
		entries []timeline.Entry
	}{
		{
			name: "event end then ghost later in the same minute",
			entries: []timeline.Entry{
				completedEvent(1, at.Add(-time.Hour), at.Add(10*time.Second)),
				ghost(at.Add(20 * time.Second)),
				ghost(at.Add(time.Minute)),
			},
		},
		{
			name: "event end and ghost share the exact boundary",
			entries: []timeline.Entry{
				completedEvent(1, at.Add(-time.Hour), at),
				ghost(at),
				ghost(at.Add(time.Minute)),
			},
		},
		{
			name: "ghost then event within one minute",
			entries: []timeline.Entry{
				ghost(at),
				completedEvent(1, at.Add(20*time.Second), at.Add(time.Minute)),
			},
		},
		{
			name: "separate event boundaries within one minute",
			entries: []timeline.Entry{
				completedEvent(1, at.Add(-time.Hour), at.Add(10*time.Second)),
				completedEvent(2, at.Add(20*time.Second), at.Add(time.Minute)),
			},
		},
		{
			name: "short event start and end within one minute",
			entries: []timeline.Entry{
				completedEvent(1, at, at.Add(10*time.Second)),
				ghost(at.Add(time.Minute)),
			},
		},
		{
			name: "hour marker then ghosts within its minute",
			entries: []timeline.Entry{
				ghost(at.Add(5 * time.Second)),
				ghost(at.Add(20 * time.Second)),
				ghost(at.Add(time.Minute)),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, width := range []int{80, 40} {
				m, _, _ := fixture(t)
				m.Resize(width, 30)
				m.items = tc.entries
				for _, expanded := range []bool{false, true} {
					if expanded {
						m.expanded = m.items[0].Key()
						m.opening = true
					}
					layout := m.measureTimeline()
					var labels []string
					for _, line := range layout.lines {
						if label := strings.TrimSpace(ansi.Cut(line, 0, timelineLabelWidth)); label != "" {
							labels = append(labels, label)
						}
					}
					for _, label := range []string{"10:00 AM", "10:01 AM"} {
						count := 0
						for _, got := range labels {
							if got == label {
								count++
							}
						}
						if count != 1 {
							t.Fatalf("width %d, expanded %v: got %d rail labels for %s, want 1; labels %q", width, expanded, count, label, labels)
						}
					}
					if len(layout.cards) != len(tc.entries) {
						t.Fatalf("got %d cards, want %d", len(layout.cards), len(tc.entries))
					}
				}
			}
		})
	}
	t.Run("fall-back times an hour apart retain both boundary labels", func(t *testing.T) {
		start, err := displaytime.ParseInput("2026-11-01 01:30:00 AM PDT")
		if err != nil {
			t.Fatal(err)
		}
		m, _, _ := fixture(t)
		end := start.Add(time.Hour)
		m.day, m.clock = displaytime.Day(start), end.Add(time.Hour)
		m.items = []timeline.Entry{completedEvent(1, start, end)}
		layout := m.measureTimeline()
		card := layout.cards[0]
		for _, row := range []int{card.top, card.top + len(card.content) + 1} {
			if got := strings.TrimSpace(layout.lines[row]); got != "01:30 AM" {
				t.Fatalf("got fall-back boundary %q, want 01:30 AM for each distinct time", got)
			}
		}
	})
}
