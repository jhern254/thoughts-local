package thoughts

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/timeline"
)

// TimelineReader supplies a resolved event interval; the thought picker owns
// the same summary window, selection and detail as the collection screen.
type TimelineReader interface {
	BrowseThoughtsView(context.Context, string, timeline.ThoughtScope, data.ThoughtViewRequest) (data.ThoughtView, error)
	CountThoughts(context.Context, string, timeline.ThoughtScope) (int64, error)
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
