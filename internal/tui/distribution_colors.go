package tui

// These session-local modes follow the approved d-key cycle. Metrics remain
// colored except in the all-neutral mode; focus never partially colors its bars.
type homeDistributionColorMode uint8

const (
	colorsAll homeDistributionColorMode = iota
	colorsAllNeutral
	colorsMetricsOnly
	colorsMountainsMetrics
	colorsEventsMetrics
	distributionColorModeCount
)

func (m *Model) applyDistributionColors() {
	outsideEvents := m.homeFocus() != homePanelEvents
	mountains := m.distributionColors == colorsAll || m.distributionColors == colorsMountainsMetrics
	events := m.distributionColors == colorsAll || m.distributionColors == colorsEventsMetrics
	m.days.SetHighlighted(outsideEvents && mountains)
	m.events.SetDistributionHighlight(outsideEvents && events)
	m.events.SetSubjectTimeColors(m.distributionColors != colorsAllNeutral)
}
