package events

type timelineTarget uint8

const (
	targetEntry timelineTarget = iota
	targetBoundary
	targetMetrics
)

// timelinePosition separates timeline-entry selection from the first visible rendered
// line. Layout supplies row positions because cards have variable heights.
type timelinePosition struct {
	target     timelineTarget
	entryIndex int
	topLine    int
	followNow  bool
}

func (p *timelinePosition) clamp(lineCount int) {
	// A selected final card may stay at the top with unused rows below it.
	p.topLine = min(max(0, p.topLine), max(0, lineCount-1))
}

func (p *timelinePosition) clampSelection(entryCount int) {
	p.entryIndex = min(max(0, p.entryIndex), max(0, entryCount-1))
}

func (p *timelinePosition) moveEntry(delta, entryCount int) {
	p.followNow = false
	p.entryIndex += delta
	p.clampSelection(entryCount)
}

func (p *timelinePosition) first() {
	p.target = targetEntry
	p.followNow = false
	p.entryIndex, p.topLine = 0, 0
}

func (p *timelinePosition) scrollPage(direction, height, lineCount int) {
	p.followNow = false
	p.topLine += direction * (height - 2)
	p.clamp(lineCount)
}

func (p *timelinePosition) showSelected(row, bodyHeight int, expanded bool) {
	if row < p.topLine || row >= p.topLine+bodyHeight-3 || expanded {
		p.topLine = row
	}
}

func (p *timelinePosition) showNow(row, bodyHeight int) {
	if p.followNow && row >= 0 {
		p.topLine = max(0, row-bodyHeight+6)
	}
}
