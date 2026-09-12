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
	"github.com/jhern254/go-thoughts/internal/timeline"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
)

const summaryLines = 3

type Metrics interface {
	CountThoughts(context.Context, string) (int64, error)
}

// ThoughtCountResult has refresh ownership independent of individual cursor requests.
type ThoughtCountResult struct {
	owner   *int
	request uint64
	total   int64
	err     error
}

type summaryRow struct {
	kind rowKind
	item data.ThoughtSummary
}

// The thought browse view uses a bounded, bidirectional window. Subject lists retain
// their existing Bubbles filtering and navigation.
type browseThoughtsState struct {
	eventScope           *timeline.ThoughtScope
	timelineView         TimelineReader
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
	owner   *int
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
	if m.browseThoughts.eventScope != nil {
		m.browseThoughts.rows = nil
	}
	m.browseThoughts.index, m.browseThoughts.offset = 0, 0
	m.browseThoughts.moreOlder, m.browseThoughts.moreNewer = false, false
	m.countRequest++
	m.browseThoughts.countPending, m.browseThoughts.countErr = true, nil
	owner, scope, reader := m.owner, m.browseThoughts.eventScope, m.browseThoughts.timelineView
	request, ctx, userID, metrics := m.countRequest, m.ctx, m.userID, m.browseThoughts.metrics
	count := func() tea.Msg {
		var total int64
		var err error
		if scope != nil {
			total, err = reader.CountThoughts(ctx, userID, *scope)
		} else {
			total, err = metrics.CountThoughts(ctx, userID)
		}
		return ThoughtCountResult{owner: owner, request: request, total: total, err: err}
	}
	return tea.Batch(m.loadThoughtsView(data.ThoughtViewRequest{}, 0), count)
}

func (m Model) receiveThoughtCount(result ThoughtCountResult) (Model, tea.Cmd) {
	if result.owner != m.owner || !m.browsingThoughtsView || result.request != m.countRequest {
		return m, nil
	}
	m.browseThoughts.total, m.browseThoughts.countErr, m.browseThoughts.countPending = result.total, result.err, false
	operation := logging.ThoughtCountAll
	if m.browseThoughts.eventScope != nil {
		operation = logging.EventThoughtCount
	}
	if category, emit := failure.Classify(operation, result.err); emit {
		m.logger.Failure(operation, category)
	}
	return m, nil
}

func (m *Model) loadThoughtsView(query data.ThoughtViewRequest, move int) tea.Cmd {
	m.request++
	m.loading, m.err = true, nil
	owner, scope, reader := m.owner, m.browseThoughts.eventScope, m.browseThoughts.timelineView
	request, ctx, userID, service := m.request, m.ctx, m.userID, m.service
	return func() tea.Msg {
		var view data.ThoughtView
		var err error
		if scope != nil {
			view, err = reader.BrowseThoughtsView(ctx, userID, *scope, query)
		} else {
			view, err = service.BrowseView(ctx, userID, query)
		}
		return BrowseThoughtsResult{owner: owner, request: request, query: query, move: move, view: view, err: err}
	}
}

func (m Model) receiveBrowseThoughts(result BrowseThoughtsResult) (Model, tea.Cmd) {
	if result.owner != m.owner || !m.browsingThoughtsView || result.request != m.request {
		return m, nil
	}
	m.loading, m.err = false, result.err
	if result.err != nil {
		operation := logging.ThoughtList
		if m.browseThoughts.eventScope != nil {
			operation = logging.EventThoughtList
		}
		if category, emit := failure.Classify(operation, result.err); emit {
			m.logger.Failure(operation, category)
		}
		m.errMessage = diagnostics.ThoughtMessage(result.err, "Could not load thoughts.")
		return m, nil
	}
	var selected summaryRow
	oldIndex, oldLength := m.browseThoughts.index, len(m.browseThoughts.rows)
	if oldLength > 0 {
		selected = m.browseThoughts.rows[oldIndex]
	}
	rows := make([]summaryRow, 0, len(result.view.Items))
	for _, item := range result.view.Items {
		rows = append(rows, summaryRow{kind: rowRecord, item: item})
	}
	switch {
	case result.query.Cursor == nil:
		m.browseThoughts.index = 0
		m.browseThoughts.rows = rows
		if m.browseThoughts.eventScope == nil {
			m.browseThoughts.rows = append([]summaryRow{{kind: rowCreate}}, rows...)
		}
		m.browseThoughts.moreOlder = result.view.More
		m.stale = false
	case result.query.Direction == data.ThoughtsNewer:
		m.browseThoughts.rows = append(rows, m.browseThoughts.rows...)
		m.browseThoughts.moreNewer = result.view.More
		if !result.view.More && m.browseThoughts.eventScope == nil {
			m.browseThoughts.rows = append([]summaryRow{{kind: rowCreate}}, m.browseThoughts.rows...)
		}
	default:
		m.browseThoughts.rows = append(m.browseThoughts.rows, rows...)
		m.browseThoughts.moreOlder = result.view.More
	}
	// Evict the opposite edge, copying so old previews are no longer retained.
	actions := 0
	if len(m.browseThoughts.rows) > 0 && m.browseThoughts.rows[0].kind == rowCreate {
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
	if m.browseThoughts.eventScope != nil {
		m.browseThoughts.offset += (len(m.browseThoughts.rows) - oldLength - m.browseThoughts.index + oldIndex) * summaryLines
	} else {
		m.browseThoughts.offset += (m.browseThoughts.index - oldIndex) * summaryLines
	}
	m.browseThoughts.index = min(max(0, m.browseThoughts.index+result.move), max(0, len(m.browseThoughts.rows)-1))
	m.browseThoughts.keepVisible()
	return m, nil
}

func (m Model) updateBrowseThoughtsView(msg tea.Msg) (Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	// Refresh may supersede a pending batch. Other commands wait for that batch.
	if m.browseThoughts.eventScope == nil && (key.String() == "home" || key.String() == "r") {
		cmd := m.reloadBrowseThoughtsView()
		return m, cmd
	}
	if m.loading || len(m.browseThoughts.rows) == 0 {
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
	if m.browseThoughts.eventScope != nil {
		move = -move
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
	index := s.index
	if s.eventScope != nil {
		index = max(0, len(s.rows)-1-index)
	}
	top := index * summaryLines
	if s.eventScope != nil {
		s.offset = max(0, s.offset/summaryLines*summaryLines)
		if top < s.offset {
			s.offset = top
		}
		if top+summaryLines > s.offset+s.height {
			s.offset = max(0, top+summaryLines-s.height)
		}
		s.offset = min(s.offset, max(0, len(s.rows)*summaryLines-s.height))
		return
	}
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
	for visualIndex := range s.rows {
		index := visualIndex
		if s.eventScope != nil {
			index = len(s.rows) - 1 - visualIndex
		}
		row := s.rows[index]
		titleStyle, descStyle := m.itemStyles.NormalTitle, m.itemStyles.NormalDesc
		if index == s.index {
			titleStyle, descStyle = m.itemStyles.SelectedTitle, m.itemStyles.SelectedDesc
		}
		title, description := "Create thought…", "Write a new thought"
		if row.kind == rowRecord {
			lines = append(lines, strings.Split(m.SummaryPreview(row.item, s.width, index == s.index), "\n")...)
			lines = append(lines, "")
			continue
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
	if s.eventScope != nil {
		if len(s.rows) == 0 && !m.loading && m.err == nil {
			visible[0] = "No thoughts yet"
		}
		return count + "\n" + strings.Join(visible, "\n") + "\n" + strings.TrimSpace(status)
	}
	return strings.Join(visible, "\n") + fmt.Sprintf("\n%s\n%s\n↑/↓: select • PgUp/PgDn: scroll\nEnter: open • Home/r: latest", count, strings.TrimSpace(status))
}

// SummaryPreview is shared by the collection picker and event cards.
func (m Model) SummaryPreview(item data.ThoughtSummary, width int, selected bool) string {
	return m.summaryPreview(item, width, selected, "Jan 2, 2006 3:04 PM MST")
}

// TimelinePreview keeps collapsed calendar cards free of timezone detail.
func (m Model) TimelinePreview(item data.ThoughtSummary, width int) string {
	return m.summaryPreview(item, width, false, "3:04 PM")
}

func (m Model) summaryPreview(item data.ThoughtSummary, width int, selected bool, layout string) string {
	titleStyle, descStyle := m.itemStyles.NormalTitle, m.itemStyles.NormalDesc
	if selected {
		titleStyle, descStyle = m.itemStyles.SelectedTitle, m.itemStyles.SelectedDesc
	}
	subject := "Misc"
	if item.SubjectName != nil {
		subject = summaryText(*item.SubjectName)
	}
	title := m.list.Styles.Title.Render(subject) + " " + summaryText(item.Preview)
	description := fmt.Sprintf("Thought %d • %s", item.ThoughtID, displaytime.Format(item.ObservedAt, layout))
	title = titleStyle.Render(ansi.Truncate(title, max(0, width-titleStyle.GetHorizontalFrameSize()), "…"))
	description = descStyle.Render(ansi.Truncate(description, max(0, width-descStyle.GetHorizontalFrameSize()), "…"))
	return ansi.Truncate(title, width, "") + "\n" + ansi.Truncate(description, width, "")
}
