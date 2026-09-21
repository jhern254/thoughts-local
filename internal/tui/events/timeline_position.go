package events

// timelinePosition separates Event selection from the first visible rendered
// line. Layout supplies row positions because cards have variable heights.
type timelinePosition struct {
	eventIndex int
	topLine    int
	followNow  bool
}

func (p *timelinePosition) clamp(lineCount, bodyHeight int, expanded bool) {
	if !expanded && lineCount <= bodyHeight {
		p.topLine = 0
		return
	}
	// A selected final card may stay at the top with unused rows below it.
	p.topLine = min(max(0, p.topLine), max(0, lineCount-1))
}

func (p *timelinePosition) clampSelection(eventCount int) {
	p.eventIndex = min(max(0, p.eventIndex), max(0, eventCount-1))
}

func (p *timelinePosition) moveEvent(delta, eventCount int) {
	p.followNow = false
	p.eventIndex += delta
	p.clampSelection(eventCount)
}

func (p *timelinePosition) first() {
	p.followNow = false
	p.eventIndex, p.topLine = 0, 0
}

func (p *timelinePosition) scrollPage(direction, height int) {
	p.followNow = false
	p.topLine += direction * (height - 2)
}

func (p *timelinePosition) showSelected(row, bodyHeight int, expanded bool) {
	if row < p.topLine || row >= p.topLine+bodyHeight-3 || expanded {
		p.topLine = row
	}
}

func (p *timelinePosition) showNow(row, bodyHeight int) {
	if p.followNow && row >= 0 {
		p.topLine = max(0, row-bodyHeight+2)
	}
}
