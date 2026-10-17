package events

import (
	"fmt"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

// Setup and command completion are outside the rendering/API timed loop.
// Alternating movement stays within the resident thought window.
func BenchmarkTimeline(b *testing.B) {
	for _, size := range [][2]int{{80, 27}, {40, 20}} {
		for _, mode := range []string{"Render", "Navigate", "Expanded"} {
			b.Run(fmt.Sprintf("%dx%d/%s", size[0], size[1], mode), func(b *testing.B) {
				m, _, _ := fixture(b)
				current := m.entries[0]
				m.entries = nil
				for i := 0; i < 23; i++ {
					start := m.day.Add(time.Duration(i) * 20 * time.Minute)
					end := start.Add(20 * time.Minute)
					m.entries = append(m.entries, eventEntries([]data.Event{{EventID: int64(i + 2), StartedAt: start, EndedAt: &end}})...)
					m.eventCounts = append(m.eventCounts, data.EventThoughtCountView{EventID: int64(i + 2), Count: int64(i)})
				}
				m.entries = append(m.entries, current)
				m.position.entryIndex = len(m.entries) - 1
				m.Resize(size[0], size[1])
				keys := [2]string{"up", "down"}
				if mode == "Expanded" {
					m = execute(b, m, m.openEvent(current.Event.EventID))
					keys = [2]string{"down", "up"}
				}
				key := 0
				b.ReportAllocs()
				for b.Loop() {
					if mode != "Render" {
						m, _ = m.Update(eventKey(keys[key]))
						key = 1 - key
					}
					_ = m.View()
				}
			})
		}
	}
}
