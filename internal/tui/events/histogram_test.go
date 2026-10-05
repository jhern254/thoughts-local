package events

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/render"
)

func TestModel_ThoughtHistogram(t *testing.T) {
	t.Run("bars align only with thought titles and survive paging and detail return", func(t *testing.T) {
		m, _, store := fixture(t)
		// The longest body is beyond the initial page. All visible titles stay small.
		for i := range store.items {
			store.items[i].CharacterCount = 20
		}
		store.items[0].CharacterCount = 320
		m = execute(t, m, m.openEvent(1))
		counts := store.counts
		check := func() string {
			t.Helper()
			rows, _, _ := m.layout()
			found := 0
			for _, row := range rows {
				plain := ansi.Strip(row)
				isTitle := strings.Contains(ansi.Cut(plain, 24, 80), "preview ")
				lane := ansi.Cut(plain, 12, 22)
				if (hasDistribution(lane) && strings.TrimSpace(lane) != "⢸") != isTitle {
					t.Fatalf("bar/title mismatch: %q", plain)
				}
				if isTitle {
					found++
					if strings.Count(lane, " ") < 7 {
						t.Fatalf("visible page incorrectly set scale: %q", lane)
					}
				}
			}
			if found == 0 {
				t.Fatal("missing thought previews")
			}
			return distributionLane(rows)
		}
		before := check()
		for i := 0; i < 55; i++ {
			next, action := m.Update(eventKey("down"))
			m = execute(t, next, action)
			check()
		}
		if store.counts != counts {
			t.Fatal("paging recounted interval")
		}
		next, cmd := m.Update(eventKey("right"))
		m = execute(t, next, cmd)
		if !m.picker.ShowingDetail() {
			t.Fatal("thought detail did not open")
		}
		m, cmd = m.Update(eventKey("esc"))
		m = execute(t, m, cmd)
		if after := check(); after != before {
			t.Fatal("same-length visible thoughts changed histogram on return")
		}
		m.Resize(40, 20)
		if hasDistribution(m.View()) {
			t.Fatal("narrow layout retained histogram")
		}
		m.Resize(80, 27)
		check()
	})
	t.Run("shared fill and size controls preserve card geometry", func(t *testing.T) {
		m, _, store := fixture(t)
		m = execute(t, m, m.openEvent(1))
		before, positions, _ := m.layout()
		reads, counts := store.reads, store.counts
		m.distributions.mode = render.Outline
		after, got, _ := m.layout()
		if got[1] != positions[1] || len(after) != len(before) {
			t.Fatal("mode moved card")
		}
		if distributionLane(after) == distributionLane(before) {
			t.Fatal("outline did not change stroke")
		}
		for i := range before {
			if timelineCardText(before[i]) != timelineCardText(after[i]) {
				t.Fatal("mode changed card content")
			}
		}
		m.tuneDistributions("down")
		smaller, _, _ := m.layout()
		if distributionLane(smaller) == distributionLane(after) {
			t.Fatal("size did not affect bars")
		}
		m.tuneDistributions("0")
		reset, _, _ := m.layout()
		if distributionLane(reset) != distributionLane(before) {
			t.Fatal("reset did not restore bars")
		}
		if store.reads != reads || store.counts != counts {
			t.Fatal("controls requested data")
		}
	})
}

func TestModel_ThoughtHistogramOwnership(t *testing.T) {
	t.Run("selection and panel focus change only the corresponding bar color", func(t *testing.T) {
		m, _, _ := fixture(t)
		m = execute(t, m, m.openEvent(1))
		before, positions, _ := m.layout()
		first := positions[1] + 4
		if stem := ansi.Cut(before[first+1], 12, 22); !strings.Contains(stem, "38;5;240") || strings.TrimSpace(ansi.Strip(stem)) != "⢸" {
			t.Fatal("stem is missing or not muted")
		}

		if !strings.Contains(ansi.Cut(before[first], 12, 22), "38;5;62") {
			t.Fatal("selected thought bar is not purple")
		}
		m, _ = m.Update(eventKey("down"))
		after, _, _ := m.layout()
		if distributionLane(before) != distributionLane(after) {
			t.Fatal("selection moved bars")
		}
		if !strings.Contains(ansi.Cut(after[first], 12, 22), "38;5;245") || !strings.Contains(ansi.Cut(after[first+3], 12, 22), "38;5;62") {
			t.Fatal("highlight did not follow thought")
		}
		m.SetFocused(false)
		blurred, _, _ := m.layout()
		if distributionLane(after) != distributionLane(blurred) || strings.Contains(ansi.Cut(blurred[first+3], 12, 22), "38;5;62") {
			t.Fatal("blur changed geometry or retained highlight")
		}
	})
	t.Run("refresh does not pair new stats with retained old previews", func(t *testing.T) {
		m, _, store := fixture(t)
		m = execute(t, m, m.openEvent(1))
		refresh := m.openEvent(1)
		next, cmd := m.Update(refresh())
		m = next
		batch := cmd().(tea.BatchMsg)
		store.items[0].CharacterCount = 1000
		m = execute(t, m, batch[1])
		if hasDistribution(m.View()) || !strings.Contains(m.View(), "preview 229") {
			t.Fatal("new stats were paired with old previews or previews were hidden")
		}
		m = execute(t, m, batch[0])
		if !hasDistribution(m.View()) {
			t.Fatal("matching refresh replies did not restore bars")
		}
	})

	t.Run("previews remain usable before stats and rejected replies cannot restore bars", func(t *testing.T) {
		m, _, store := fixture(t)
		open := m.openEvent(1)
		next, cmd := m.Update(open())
		m = next
		batch := cmd().(tea.BatchMsg)
		m = execute(t, m, batch[0])
		if hasDistribution(m.View()) || !strings.Contains(m.View(), "preview 229") {
			t.Fatal("pending stats hid previews or invented bars")
		}
		reply := batch[1]()
		m, _ = m.Update(reply)
		if !hasDistribution(m.View()) {
			t.Fatal("accepted stats did not show bars")
		}
		m, _ = m.Update(eventKey("left"))
		collapsed := m.View()
		m, _ = m.Update(reply)
		if m.View() != collapsed {
			t.Fatal("closed picker accepted stale stats")
		}
		open = m.openEvent(1)
		m, cmd = m.Update(open())
		batch = cmd().(tea.BatchMsg)
		m = execute(t, m, batch[0])
		store.err = errors.New("stats unavailable")
		m = execute(t, m, batch[1])
		if hasDistribution(m.View()) || !strings.Contains(m.View(), "preview 229") || !strings.Contains(m.View(), "Thought count unavailable") {
			t.Fatal("failed stats changed usable previews or invented bars")
		}
		before := m.View()
		m, _ = m.Update(reply)
		if m.View() != before {
			t.Fatal("previous generation restored stale stats")
		}
	})
}
