package events

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
)

// Calendar assertions inspect the unchanged time field and the card at column
// 24 independently of the decorative lane. All callers use a wide panel.
func timelineCardText(line string) string {
	line = ansi.Strip(line)
	return ansi.Cut(line, 0, 10) + ansi.Cut(line, 24, ansi.StringWidth(line))
}

func hasDistribution(text string) bool {
	return strings.ContainsFunc(text, func(r rune) bool { return r >= 0x2800 && r <= 0x28ff })
}

func distributionLane(lines []string) string {
	var lane strings.Builder
	for _, line := range lines {
		lane.WriteString(ansi.Cut(ansi.Strip(line), 12, 22))
		lane.WriteByte('\n')
	}
	return lane.String()
}

func TestModel_Distributions(t *testing.T) {
	t.Run("separate lane appears at sixty columns without changing chronological rows", func(t *testing.T) {
		m, _, _ := fixture(t)
		var positions map[int64]int
		for _, width := range []int{100, 60, 59, 40, 80} {
			m.Resize(width, 27)
			lines, got, now := m.layout()
			if positions == nil {
				positions = got
			}
			if !reflect.DeepEqual(positions, got) {
				t.Fatal("resizing changed chronological card rows")
			}
			column := 10
			if width >= 60 {
				column = 24
			}
			top := ansi.Strip(lines[got[1]])
			if ansi.Cut(top, column, column+1) != "╭" {
				t.Fatalf("width %d: card is not at column %d: %q", width, column, top)
			}
			if hasDistribution(strings.Join(lines, "\n")) != (width >= 60) {
				t.Fatalf("width %d: wrong distribution visibility", width)
			}
			if now != got[1]+6 || hasDistribution(lines[now]) {
				t.Fatal("curve changed or covered Now")
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > width {
					t.Fatalf("width %d: overflowing row %q", width, line)
				}
			}
		}
	})
	t.Run("unknown counts reserve the lane and known zero has a mound", func(t *testing.T) {
		m, _, _ := fixture(t)
		m.counts = nil
		missing, positions, now := m.layout()
		if hasDistribution(strings.Join(missing, "\n")) {
			t.Fatal("unknown counts invented activity")
		}
		if ansi.Cut(ansi.Strip(missing[positions[1]]), 24, 25) != "╭" {
			t.Fatal("unknown count removed reserved lane")
		}
		m.counts = []data.EventThoughtCountView{{EventID: 1, Count: 0}}
		zero, got, end := m.layout()
		if !hasDistribution(strings.Join(zero, "\n")) || !reflect.DeepEqual(positions, got) || now != end {
			t.Fatal("zero mound absent or moved cards")
		}
		m.load.countErr = errors.New("count failed")
		failed, got, end := m.layout()
		if hasDistribution(strings.Join(failed, "\n")) || !reflect.DeepEqual(positions, got) || now != end {
			t.Fatal("failed count drew activity or moved cards")
		}
	})
	t.Run("expansion removes its curve and collapse restores it without reads", func(t *testing.T) {
		m, service, store := fixture(t)
		before, _, _ := m.layout()
		m = execute(t, m, m.openEvent(1))
		expanded, positions, _ := m.layout()
		if hasDistribution(strings.Join(expanded, "\n")) {
			t.Fatal("expanded event still has a distribution")
		}
		if ansi.Cut(ansi.Strip(expanded[positions[1]]), 24, 25) != "╭" {
			t.Fatal("expansion moved card out of its column")
		}
		lists, counts, reads := service.lists, store.counts, store.reads
		m, cmd := m.Update(eventKey("left"))
		if cmd != nil {
			t.Fatal("collapse requested data")
		}
		collapsed, _, _ := m.layout()
		if distributionLane(before) != distributionLane(collapsed) {
			t.Fatal("collapse did not restore the curve")
		}
		m.SetFocused(false)
		m.Resize(59, 27)
		m.Resize(80, 27)
		_ = m.View()
		if lists != service.lists || counts != store.counts || reads != store.reads {
			t.Fatal("presentation issued reads")
		}
	})
	t.Run("accepted counts drive the scale while card labels retain their totals", func(t *testing.T) {
		m, _, _ := fixture(t)
		previous, maximum := "", ""
		for _, count := range []int64{0, 1, 10, 20, 230} {
			m.counts = []data.EventThoughtCountView{{EventID: 1, Count: count}}
			lines, _, _ := m.layout()
			lane := distributionLane(lines)
			if count <= 20 && lane == previous {
				t.Fatalf("count %d did not change the shape", count)
			}
			if count == 20 {
				maximum = lane
			}
			if count > 20 && lane != maximum {
				t.Fatal("counts above twenty exceeded the display scale")
			}
			label := fmt.Sprintf("%d thoughts", count)
			if count == 1 {
				label = "1 thought"
			}
			if !strings.Contains(ansi.Strip(strings.Join(lines, "\n")), label) {
				t.Fatalf("missing actual count %q", label)
			}
			previous = lane
		}
	})
	t.Run("highlight follows event identity when an earlier event has no count", func(t *testing.T) {
		m, _, _ := fixture(t)
		end := m.items[0].StartedAt
		m.items = append([]data.Event{{EventID: 2, StartedAt: end.Add(-time.Hour), EndedAt: &end}}, m.items...)
		m.position.eventIndex = 1
		lines, positions, _ := m.layout()
		row := positions[1] + 2
		selected := ansi.Cut(lines[row], 12, 22)
		if !strings.Contains(selected, "38;5;62") {
			t.Fatal("selected contribution did not match event ID")
		}
		m.SetFocused(false)
		blurred, _, _ := m.layout()
		if distributionLane(lines) != distributionLane(blurred) || strings.Contains(ansi.Cut(blurred[row], 12, 22), "38;5;62") {
			t.Fatal("blur changed geometry or retained highlight")
		}
		m.SetFocused(true)
		m.position.eventIndex = 0
		other, _, _ := m.layout()
		if distributionLane(lines) != distributionLane(other) || !strings.Contains(ansi.Cut(other[row], 12, 22), "38;5;245") {
			t.Fatal("selection changed geometry or failed to dull the curve")
		}
	})
	t.Run("neighboring cards blend through their separator and leave the ending clear", func(t *testing.T) {
		m, _, _ := fixture(t)
		m.day = m.day.AddDate(0, 0, -1)
		start := m.day.Add(9 * time.Hour)
		middle, end := start.Add(time.Hour), start.Add(2*time.Hour)
		m.items = []data.Event{{EventID: 1, StartedAt: start, EndedAt: &middle}, {EventID: 2, StartedAt: middle, EndedAt: &end}}
		m.counts = []data.EventThoughtCountView{{EventID: 1, Count: 20}, {EventID: 2, Count: 20}}
		combined, positions, _ := m.layout()
		row := positions[2] - 1
		if positions[2]-positions[1] != 5 || strings.TrimSpace(timelineCardText(combined[row])) != "" {
			t.Fatal("curve moved or filled the card separator")
		}
		both := ansi.Cut(ansi.Strip(combined[row]), 12, 22)
		m.counts = m.counts[:1]
		single, _, _ := m.layout()
		if !hasDistribution(both) || both == ansi.Cut(ansi.Strip(single[row]), 12, 22) {
			t.Fatal("neighboring tails did not combine in the separator")
		}
		last := combined[len(combined)-1]
		if hasDistribution(last) || strings.TrimSpace(last) != "..." {
			t.Fatal("historical ending covered or changed")
		}
		m.expanded = 1
		expanded, _, _ := m.layout()
		if hasDistribution(strings.Join(expanded, "\n")) {
			t.Fatal("expanded event or unknown count contributed a curve")
		}
		m.counts = []data.EventThoughtCountView{{EventID: 2, Count: 20}}
		expanded, _, _ = m.layout()
		if !hasDistribution(strings.Join(expanded, "\n")) {
			t.Fatal("expansion removed another collapsed event's curve")
		}
	})
	t.Run("count replies cannot move cards or leak curves into a different day", func(t *testing.T) {
		m, service, store := fixture(t)
		before, positions, end := m.layout()
		batch := m.loadDay(m.day)().(tea.BatchMsg)
		if m.View() == "" {
			t.Fatal("refresh lost retained body")
		}
		m, latest := m.Update(batch[0]())
		m = execute(t, m, latest)
		refreshing, _, _ := m.layout()
		if distributionLane(refreshing) != distributionLane(before) {
			t.Fatal("refresh discarded valid cached curves")
		}
		store.items = nil
		reply := batch[1]()
		m, _ = m.Update(reply)
		zero, got, marker := m.layout()
		if !reflect.DeepEqual(positions, got) || end != marker || !hasDistribution(distributionLane(zero)) {
			t.Fatal("count reply changed geometry or lost zero mound")
		}
		// A real reply from an older request must not restore its curve after navigation.
		service.items = nil
		m = execute(t, m, m.loadDay(m.day.AddDate(0, 0, -1)))
		empty := m.View()
		m, _ = m.Update(reply)
		if m.View() != empty || hasDistribution(empty) {
			t.Fatal("stale count reply drew on a different day")
		}
		m.Close()
	})

	t.Run("scrolling crops the same complete day mixture", func(t *testing.T) {
		m, _, _ := fixture(t)
		start := m.day.Add(9 * time.Hour)
		end := start.Add(time.Hour)
		m.items = []data.Event{{EventID: 2, StartedAt: start, EndedAt: &end}, m.items[0]}
		m.counts = []data.EventThoughtCountView{{EventID: 1, Count: 10}, {EventID: 2, Count: 20}}
		m.position.followNow = false
		lines, positions, _ := m.layout()
		for _, offset := range []int{positions[2] - 2, positions[2] + 2, positions[1]} {
			m.position.topLine = offset
			body := strings.Split(m.timelineBody(), "\n")
			for i := 0; i < len(body) && offset+i < len(lines); i++ {
				if body[i] != lines[offset+i] {
					t.Fatalf("scrolling changed curve or content at row %d", offset+i)
				}
			}
		}
		before := m.position.eventIndex
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp}))
		if m.position.eventIndex != before {
			t.Fatal("paging changed selection")
		}
	})
}
