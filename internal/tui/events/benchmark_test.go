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
				current := m.items[0]
				m.items = nil
				for i := 0; i < 23; i++ {
					start := m.day.Add(time.Duration(i) * 20 * time.Minute)
					end := start.Add(20 * time.Minute)
					m.items = append(m.items, data.Event{EventID: int64(i + 2), StartedAt: start, EndedAt: &end})
				}
				m.items = append(m.items, current)
				m.index = len(m.items) - 1
				m.Resize(size[0], size[1])
				keys := [2]string{"up", "down"}
				if mode == "Expanded" {
					m = execute(b, m, m.openEvent(current.EventID))
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
