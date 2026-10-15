package thoughts

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Deep scrolling retains three 50-record pages. Fixture creation, page reads
// and resizing are outside timing; alternating selection stays in that window.
func BenchmarkThoughtSummaryWindow(b *testing.B) {
	for _, size := range [][2]int{{80, 27}, {140, 45}} {
		for _, mode := range []string{"Render", "Navigate"} {
			b.Run(fmt.Sprintf("%dx%d/%s", size[0], size[1], mode), func(b *testing.B) {
				m := browseThoughtsModel(b, 230)
				for range 130 {
					m = browseThoughtsKey(m, tea.KeyDown)
				}
				if len(m.browseThoughts.rows) != 151 {
					b.Fatalf("got %d resident rows, want Create plus 150 records", len(m.browseThoughts.rows))
				}
				name := "Testing clarity"
				preview := string([]rune(strings.Repeat("Synthetic note 界 ", 6))[:80]) + "…"
				for index := range m.browseThoughts.rows {
					m.browseThoughts.rows[index].item.SubjectName = &name
					m.browseThoughts.rows[index].item.Preview = preview
				}
				m.Resize(size[0], size[1])
				keys := [2]rune{tea.KeyUp, tea.KeyDown}
				step := 0
				b.ReportAllocs()
				for b.Loop() {
					if mode == "Navigate" {
						var command tea.Cmd
						m, command = m.Update(key(keys[step]))
						if command != nil {
							b.Fatal("resident selection unexpectedly requested data")
						}
						step = 1 - step
					}
					_ = m.View()
				}
			})
		}
	}
}
