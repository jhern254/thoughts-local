package thoughts

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/timeline"
)

// Fixed presentation label until selectable event-thought ordering is implemented.
const eventThoughtOrderLabel = "Newest first"

// TimelineReader supplies a resolved event interval; the thought picker owns the
// same summary window, selection, and detail behavior as the Browse Thoughts view.
type TimelineReader interface {
	BrowseThoughtsView(context.Context, string, timeline.ThoughtScope, data.ThoughtSummaryViewRequest) (data.ThoughtSummaryViewResult, error)
	ThoughtStats(context.Context, string, timeline.ThoughtScope) (data.ThoughtIntervalStats, error)
}

func (m *Model) OpenEventView(scope timeline.ThoughtScope, reader TimelineReader) tea.Cmd {
	previous := m.browseThoughts
	retain := previous.eventScope != nil && previous.eventScope.Event().EventID == scope.Event().EventID
	m.Reset()
	m.browsingThoughtsView = true
	m.browseThoughts.eventScope, m.browseThoughts.timelineView = &scope, reader
	cmd := m.reloadBrowseThoughtsView()
	// Retain the usable window during refresh. The replacement latest batch
	// preserves selection by ID when present, otherwise selects its newest row.
	if retain {
		m.browseThoughts.rows = previous.rows
		m.browseThoughts.index = previous.index
		m.browseThoughts.offset = previous.offset
	}
	return cmd
}

// ResizeEventView budgets complete rows, leaving detail rendering unchanged.
func (m *Model) ResizeEventView(width, rows int) {
	m.browseThoughts.width, m.browseThoughts.height = max(1, width), max(1, rows)*summaryLines
	m.browseThoughts.keepVisible()
}

// ThoughtPlot identifies a visible preview row relative to the picker's View.
// Events owns lane placement and rendering; the picker owns scrolling/selection.
type ThoughtPlot struct {
	Row            int
	CharacterCount int64
	Selected       bool
}

func (m Model) EventThoughtPlots() ([]ThoughtPlot, int64) {
	s := m.browseThoughts
	if s.eventScope == nil || m.ShowingDetail() || s.statsPending || s.statsErr != nil || !s.summariesCurrent {
		return nil, 0
	}
	var plots []ThoughtPlot
	for index := s.offset / summaryLines; index < len(s.rows); index++ {
		row := index*summaryLines - s.offset
		if row >= s.height {
			break
		}
		if row < 0 || s.rows[index].kind != rowRecord {
			continue
		}
		plots = append(plots, ThoughtPlot{
			Row:            row + 1,
			CharacterCount: s.rows[index].item.CharacterCount,
			Selected:       index == s.index && !m.blurred,
		})
	}
	return plots, s.maxCharacters
}
