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
	"github.com/jhern254/go-thoughts/internal/render"
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
	t.Run("viewport paint matches complete output through partial cards and curve tails", func(t *testing.T) {
		m, _, _ := fixture(t)
		start := m.day.Add(8 * time.Hour)
		end := start.Add(time.Hour)
		m.items = append([]data.Event{{EventID: 2, StartedAt: start, EndedAt: &end}}, m.items...)
		m.counts = append(m.counts, data.EventThoughtCountView{EventID: 2, Count: 20})
		for _, width := range []int{80, 60, 59, 40} {
			m.Resize(width, 27)
			for _, expanded := range []bool{false, true} {
				if expanded {
					m = execute(t, m, m.openEvent(1))
				}
				for _, settings := range []distributionSettings{
					defaultDistributionSettings(),
					{boost: 0, size: 50, mode: render.Outline},
					{boost: 1, size: 125, mode: render.Filled},
					{boost: 2, size: 100, mode: render.Outline},
					{boost: 3, size: 125, mode: render.Outline},
					{boost: 4, size: 50, mode: render.Filled},
				} {
					m.distributions = settings
					whole, _, _ := m.layout()
					layout := m.measureTimeline()
					for offset := 0; offset < len(whole); offset++ {
						got := m.paintTimeline(layout, offset, 7)
						want := whole[offset:min(len(whole), offset+7)]
						if !reflect.DeepEqual(got, want) {
							t.Fatalf("width %d expanded %v settings %+v offset %d: cropped paint differs", width, expanded, settings, offset)
						}
					}
				}
				if expanded {
					m.distributions.open = false
					m, _ = m.Update(eventKey("left"))
				}
			}
		}
	})

	t.Run("live controls change only curves and restore defaults without reads", func(t *testing.T) {
		m, service, store := fixture(t)
		m.counts[0].Count = 10
		before, positions, _ := m.layout()
		position, day := m.position, m.day
		lists, counts, reads := service.lists, store.counts, store.reads
		press := func(key string) {
			t.Helper()
			var cmd tea.Cmd
			m, cmd = m.Update(eventKey(key))
			if cmd != nil {
				t.Fatalf("tuning key %q issued a command", key)
			}
		}
		press("d")
		if !strings.Contains(m.View(), "Curves:") {
			t.Fatal("distribution controls did not open")
		}
		press("right")
		boosted, got, _ := m.layout()
		if distributionLane(before) == distributionLane(boosted) {
			t.Fatal("boost did not update the curve immediately")
		}
		press("down")
		shrunk, _, _ := m.layout()
		if distributionLane(shrunk) == distributionLane(boosted) {
			t.Fatal("size did not update the curve immediately")
		}
		press("f")
		outline, _, _ := m.layout()
		if distributionLane(outline) == distributionLane(shrunk) || !strings.Contains(m.View(), "outline") {
			t.Fatal("fill toggle did not show an outline")
		}
		for i := range before {
			if timelineCardText(before[i]) != timelineCardText(outline[i]) {
				t.Fatalf("controls moved card/time row %d", i)
			}
		}
		if !reflect.DeepEqual(positions, got) || m.position != position || m.day != day {
			t.Fatal("tuning changed timeline navigation")
		}
		press("esc")
		if strings.Contains(m.View(), "Curves:") {
			t.Fatal("Escape did not close controls")
		}
		press("d")
		if !strings.Contains(m.View(), "outline") {
			t.Fatal("reopening controls lost settings")
		}
		press("0")
		reset, _, _ := m.layout()
		if distributionLane(reset) != distributionLane(before) {
			t.Fatal("reset did not restore defaults")
		}
		press("enter")
		if m.expanded != 0 {
			t.Fatal("closing controls expanded a card")
		}
		if lists != service.lists || counts != store.counts || reads != store.reads {
			t.Fatal("tuning triggered database reads")
		}
	})

	t.Run("controls stay bounded and reset every setting", func(t *testing.T) {
		m, _, _ := fixture(t)
		m, _ = m.Update(eventKey("d"))
		for range 40 {
			m, _ = m.Update(eventKey("right"))
			m, _ = m.Update(eventKey("up"))
		}
		if !strings.Contains(m.View(), "extra boost · 125% size") {
			t.Fatal("upper tuning bounds are wrong")
		}
		for range 40 {
			m, _ = m.Update(eventKey("left"))
			m, _ = m.Update(eventKey("down"))
		}
		if !strings.Contains(m.View(), "linear boost · 50% size") {
			t.Fatal("lower tuning bounds are wrong")
		}
		m, _ = m.Update(eventKey("f"))
		m, _ = m.Update(eventKey("0"))
		if !strings.Contains(m.View(), "normal boost · 100% size · filled") {
			t.Fatal("default control did not restore all intended defaults")
		}
		m.Resize(60, 27)
		if !strings.Contains(m.View(), "[0 Reset to default]") || !strings.Contains(m.View(), "Esc done") {
			t.Fatalf("minimum-width tuning panel hides essential controls: %s", m.View())
		}
	})

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
				t.Fatal("the sole event exceeded the bounded maximum size")
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
	t.Run("expanded and offscreen maximum still sets the day scale", func(t *testing.T) {
		m, _, _ := fixture(t)
		m.day = m.day.AddDate(0, 0, -1)
		m.items = nil
		m.counts = nil
		for i, count := range []int64{20, 100, 200} {
			start := m.day.Add(time.Duration(3+i*6) * time.Hour)
			end := start.Add(time.Hour)
			id := int64(i + 1)
			m.items = append(m.items, data.Event{EventID: id, StartedAt: start, EndedAt: &end})
			m.counts = append(m.counts, data.EventThoughtCountView{EventID: id, Count: count})
		}
		before, positions, _ := m.layout()
		widths := []int{}
		for _, id := range []int64{1, 2, 3} {
			lane := ansi.Cut(ansi.Strip(before[positions[id]+1]), 12, 22)
			widths = append(widths, ansi.StringWidth(strings.TrimLeft(lane, " ")))
		}
		if !(widths[0] < widths[1] && widths[1] < widths[2]) {
			t.Fatalf("counts 20, 100, 200 have peak widths %v, want increasing widths", widths)
		}
		m.expanded = 3
		expanded, got, _ := m.layout()
		for _, id := range []int64{1, 2} {
			if distributionLane(before[positions[id]:positions[id]+4]) != distributionLane(expanded[got[id]:got[id]+4]) {
				t.Fatalf("expanding the maximum rescaled distant event %d", id)
			}
		}
		m.position.followNow = false
		m.position.topLine = positions[1] - 2
		visible := strings.Split(m.timelineBody(), "\n")
		if distributionLane(visible) != distributionLane(expanded[m.position.topLine:m.position.topLine+len(visible)]) {
			t.Fatal("cropping away the maximum changed the visible scale")
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
