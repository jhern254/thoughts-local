package thoughts

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/diagnostics"
	"github.com/jhern254/go-thoughts/internal/failure"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
)

const summaryLines = 3

type Metrics interface {
	CountThoughts(context.Context, string) (int64, error)
}

// ThoughtCountResult has refresh ownership independent of individual cursor requests.
type ThoughtCountResult struct {
	request uint64
	total   int64
	err     error
}

type summaryRow struct {
	kind rowKind
	item data.ThoughtSummary
}

// Only All Thoughts uses a bounded, bidirectional window. Subject lists retain
// their existing Bubbles filtering and navigation.
type browseThoughtsState struct {
	rows                 []summaryRow
	index, offset        int
	width, height        int
	moreOlder, moreNewer bool
	metrics              Metrics
	total                int64
	countPending         bool
	countErr             error
}

// BrowseThoughtsResult belongs to one request/session, like the existing detail results.
type BrowseThoughtsResult struct {
	request uint64
	query   data.ThoughtViewRequest
	move    int
	view    data.ThoughtView
	err     error
}

// SelectedSubjectName reads display metadata from the loaded browse projection.
// Subject-scoped screens already have their own subject context.
func (m Model) SelectedSubjectName() string {
	if m.selected == nil || m.selected.SubjectID == nil {
		return "Misc"
	}
	for _, row := range m.browseThoughts.rows {
		if row.kind == rowRecord && row.item.ThoughtID == m.selected.ThoughtID && row.item.SubjectName != nil {
			return *row.item.SubjectName
		}
	}
	return "Subject"
}

func (m *Model) OpenBrowseThoughtsView(metrics Metrics) tea.Cmd {
	m.Reset()
	m.browsingThoughtsView = true
	m.browseThoughts.metrics = metrics
	return m.reloadBrowseThoughtsView()
}

func (m *Model) reloadBrowseThoughtsView() tea.Cmd {
	m.browseThoughts.rows = []summaryRow{{kind: rowCreate}}
	m.browseThoughts.index, m.browseThoughts.offset = 0, 0
	m.browseThoughts.moreOlder, m.browseThoughts.moreNewer = false, false
	m.countRequest++
	m.browseThoughts.countPending, m.browseThoughts.countErr = true, nil
	request, ctx, userID, metrics := m.countRequest, m.ctx, m.userID, m.browseThoughts.metrics
	count := func() tea.Msg {
		total, err := metrics.CountThoughts(ctx, userID)
		return ThoughtCountResult{request: request, total: total, err: err}
	}
	return tea.Batch(m.loadThoughtsView(data.ThoughtViewRequest{}, 0), count)
}

func (m Model) receiveThoughtCount(result ThoughtCountResult) (Model, tea.Cmd) {
	if !m.browsingThoughtsView || result.request != m.countRequest {
		return m, nil
	}
	m.browseThoughts.total, m.browseThoughts.countErr, m.browseThoughts.countPending = result.total, result.err, false
	if category, emit := failure.Classify(logging.ThoughtCountAll, result.err); emit {
		m.logger.Failure(logging.ThoughtCountAll, category)
	}
	return m, nil
}

func (m *Model) loadThoughtsView(query data.ThoughtViewRequest, move int) tea.Cmd {
	m.request++
	m.loading, m.err = true, nil
	request, ctx, userID, service := m.request, m.ctx, m.userID, m.service
	return func() tea.Msg {
		view, err := service.BrowseView(ctx, userID, query)
		return BrowseThoughtsResult{request: request, query: query, move: move, view: view, err: err}
	}
}

func (m Model) receiveBrowseThoughts(result BrowseThoughtsResult) (Model, tea.Cmd) {
	if !m.browsingThoughtsView || result.request != m.request {
		return m, nil
	}
	m.loading, m.err = false, result.err
	if result.err != nil {
		if category, emit := failure.Classify(logging.ThoughtList, result.err); emit {
			m.logger.Failure(logging.ThoughtList, category)
		}
		m.errMessage = diagnostics.ThoughtMessage(result.err, "Could not load thoughts.")
		return m, nil
	}
	selected, oldIndex := m.browseThoughts.rows[m.browseThoughts.index], m.browseThoughts.index
	rows := make([]summaryRow, 0, len(result.view.Items))
	for _, item := range result.view.Items {
		rows = append(rows, summaryRow{kind: rowRecord, item: item})
	}
	switch {
	case result.query.Cursor == nil:
		m.browseThoughts.rows = append([]summaryRow{{kind: rowCreate}}, rows...)
		m.browseThoughts.moreOlder = result.view.More
		m.stale = false
	case result.query.Direction == data.ThoughtsNewer:
		m.browseThoughts.rows = append(rows, m.browseThoughts.rows...)
		m.browseThoughts.moreNewer = result.view.More
		if !result.view.More {
			m.browseThoughts.rows = append([]summaryRow{{kind: rowCreate}}, m.browseThoughts.rows...)
		}
	default:
		m.browseThoughts.rows = append(m.browseThoughts.rows, rows...)
		m.browseThoughts.moreOlder = result.view.More
	}
	// Evict the opposite edge, copying so old previews are no longer retained.
	actions := 0
	if m.browseThoughts.rows[0].kind == rowCreate {
		actions = 1
	}
	if excess := len(m.browseThoughts.rows) - actions - 3*data.ThoughtViewBatchSize; excess > 0 {
		if result.query.Direction == data.ThoughtsNewer {
			m.browseThoughts.rows = slices.Clone(m.browseThoughts.rows[:len(m.browseThoughts.rows)-excess])
			m.browseThoughts.moreOlder = true
		} else {
			m.browseThoughts.rows = slices.Clone(m.browseThoughts.rows[excess+actions:])
			m.browseThoughts.moreNewer = true
		}
	}
	for index, row := range m.browseThoughts.rows {
		if row.kind == selected.kind && (row.kind == rowCreate || row.item.ThoughtID == selected.item.ThoughtID) {
			m.browseThoughts.index = index
			break
		}
	}
	m.browseThoughts.offset += (m.browseThoughts.index - oldIndex) * summaryLines
	m.browseThoughts.index = min(max(0, m.browseThoughts.index+result.move), len(m.browseThoughts.rows)-1)
	m.browseThoughts.keepVisible()
	return m, nil
}

func (m Model) updateBrowseThoughtsView(msg tea.Msg) (Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	// Refresh may supersede a pending batch. Other commands wait for that batch.
	if key.String() == "home" || key.String() == "r" {
		cmd := m.reloadBrowseThoughtsView()
		return m, cmd
	}
	if m.loading {
		return m, nil
	}
	move := 0
	switch key.String() {
	case "up", "k":
		move = -1
	case "down", "j":
		move = 1
	case "pgup":
		move = -max(1, m.browseThoughts.height/summaryLines)
	case "pgdown":
		move = max(1, m.browseThoughts.height/summaryLines)
	case "enter":
		row := m.browseThoughts.rows[m.browseThoughts.index]
		m.err = nil
		if row.kind == rowCreate {
			m.request++
			m.screen, m.inputWarning = create, ""
			return m, m.input.Focus()
		}
		cmd := m.getThought(row.item.ThoughtID)
		return m, cmd
	}
	if move == 0 {
		return m, nil
	}
	if m.browseThoughts.index+move < 0 && m.browseThoughts.moreNewer {
		cursor := m.browseThoughts.rows[0].item.Cursor()
		cmd := m.loadThoughtsView(data.ThoughtViewRequest{Cursor: &cursor, Direction: data.ThoughtsNewer}, move)
		return m, cmd
	}
	if m.browseThoughts.index+move >= len(m.browseThoughts.rows) && m.browseThoughts.moreOlder {
		cursor := m.browseThoughts.rows[len(m.browseThoughts.rows)-1].item.Cursor()
		cmd := m.loadThoughtsView(data.ThoughtViewRequest{Cursor: &cursor}, move)
		return m, cmd
	}
	m.browseThoughts.index = min(max(0, m.browseThoughts.index+move), len(m.browseThoughts.rows)-1)
	m.browseThoughts.keepVisible()
	return m, nil
}

func (s *browseThoughtsState) keepVisible() {
	top := s.index * summaryLines
	if top < s.offset {
		s.offset = top
	}
	if bottom := top + min(2, s.height); bottom > s.offset+s.height {
		s.offset = bottom - s.height
	}
	s.offset = min(max(0, s.offset), max(0, len(s.rows)*summaryLines-s.height))
}

// Single-line display sanitization does not alter the stored body or editor.
func summaryText(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
}

func (m Model) renderBrowseThoughtsView(status string) string {
	s := m.browseThoughts
	lines := make([]string, 0, len(s.rows)*summaryLines)
	for index, row := range s.rows {
		titleStyle, descStyle := m.itemStyles.NormalTitle, m.itemStyles.NormalDesc
		if index == s.index {
			titleStyle, descStyle = m.itemStyles.SelectedTitle, m.itemStyles.SelectedDesc
		}
		title, description := "Create thought…", "Write a new thought"
		if row.kind == rowRecord {
			subject := "Misc"
			if row.item.SubjectName != nil {
				subject = summaryText(*row.item.SubjectName)
			}
			title = m.list.Styles.Title.Render(subject) + " " + summaryText(row.item.Preview)
			description = fmt.Sprintf("Thought %d • %s", row.item.ThoughtID, displaytime.Format(row.item.ObservedAt, "Jan 2, 2006 3:04 PM MST"))
		}
		title = titleStyle.Render(ansi.Truncate(title, max(0, s.width-titleStyle.GetHorizontalFrameSize()), "…"))
		description = descStyle.Render(ansi.Truncate(description, max(0, s.width-descStyle.GetHorizontalFrameSize()), "…"))
		// Clip the selection rail/padding too when the terminal is only one cell wide.
		lines = append(lines, ansi.Truncate(title, s.width, ""), ansi.Truncate(description, s.width, ""), "")
	}
	visible := append([]string{}, lines[min(s.offset, len(lines)):min(s.offset+s.height, len(lines))]...)
	for len(visible) < s.height {
		visible = append(visible, "")
	}
	label := "thoughts"
	if s.total == 1 {
		label = "thought"
	}
	count := fmt.Sprintf("%d %s", s.total, label)
	if s.countPending {
		count = "Counting thoughts…"
	} else if s.countErr != nil {
		count = "Thought count unavailable"
	}
	return strings.Join(visible, "\n") + fmt.Sprintf("\n%s\n%s\n↑/↓: select • PgUp/PgDn: scroll\nEnter: open • Home/r: latest", count, strings.TrimSpace(status))
}
