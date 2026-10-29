package tui

import "testing"

func TestModel_DistributionColorCycle(t *testing.T) {
	for _, panel := range []homePanel{homePanelEntities, homePanelMindset} {
		m := newMindsetHome(t, 1)
		m.focusHome(panel)
		for _, want := range []homeDistributionColorMode{colorsAllNeutral, colorsMetricsOnly, colorsMountainsMetrics, colorsEventsMetrics, colorsAll} {
			next, command := rootUpdate(m, runeKey('d'))
			m = next
			if command != nil {
				t.Fatalf("panel %d: color toggle returned work %T", panel, command)
			}
			if m.distributionColors != want || m.homeFocus() != panel || m.events.TuningDistributions() {
				t.Fatalf("panel %d: got mode %d focus %d, want %d with retained focus", panel, m.distributionColors, m.homeFocus(), want)
			}
			m.focusHome(homePanelEvents)
			m.focusHome(panel)
			if m.distributionColors != want {
				t.Fatal("panel switch reset color mode")
			}
		}
	}
}
