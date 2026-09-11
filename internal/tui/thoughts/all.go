package thoughts

import (
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

type summaryRow struct {
	kind rowKind
	item data.ThoughtSummary
}

// Only All Thoughts uses a bounded, bidirectional window. Subject lists retain
// their existing Bubbles filtering and navigation.
type allState struct {
	rows                 []summaryRow
	index, offset        int
	width, height        int
	moreOlder, moreNewer bool
}

// PageResult belongs to one request/session, like the existing detail results.
type PageResult struct {
	request uint64
	query   data.ThoughtPageRequest
	move    int
	page    data.ThoughtPage
	err     error
}

// SelectedSubjectName reads display metadata from the loaded browse projection.
// Subject-scoped screens already have their own subject context.
func (m Model) SelectedSubjectName() string {
	if m.selected == nil || m.selected.SubjectID == nil {
		return "Misc"
	}
	for _, row := range m.all.rows {
		if row.kind == rowRecord && row.item.ThoughtID == m.selected.ThoughtID && row.item.SubjectName != nil {
			return *row.item.SubjectName
		}
	}
	return "Subject"
}

func (m *Model) OpenAll() tea.Cmd {
	m.Reset()
	m.allThoughts = true
	return m.latestThoughts()
}

func (m *Model) latestThoughts() tea.Cmd {
	m.all.rows = []summaryRow{{kind: rowCreate}}
	m.all.index, m.all.offset = 0, 0
	m.all.moreOlder, m.all.moreNewer = false, false
	return m.browseThoughts(data.ThoughtPageRequest{}, 0)
}

func (m *Model) browseThoughts(query data.ThoughtPageRequest, move int) tea.Cmd {
	m.request++
	m.loading, m.err = true, nil
	request, ctx, userID, service := m.request, m.ctx, m.userID, m.service
	return func() tea.Msg {
		page, err := service.Browse(ctx, userID, query)
		return PageResult{request: request, query: query, move: move, page: page, err: err}
	}
}

func (m Model) receivePage(result PageResult) (Model, tea.Cmd) {
	if !m.allThoughts || result.request != m.request {
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
	selected, oldIndex := m.all.rows[m.all.index], m.all.index
	rows := make([]summaryRow, 0, len(result.page.Items))
	for _, item := range result.page.Items {
		rows = append(rows, summaryRow{kind: rowRecord, item: item})
	}
	switch {
	case result.query.Cursor == nil:
		m.all.rows = append([]summaryRow{{kind: rowCreate}}, rows...)
		m.all.moreOlder = result.page.More
		m.stale = false
	case result.query.Direction == data.ThoughtsNewer:
		m.all.rows = append(rows, m.all.rows...)
		m.all.moreNewer = result.page.More
		if !result.page.More {
			m.all.rows = append([]summaryRow{{kind: rowCreate}}, m.all.rows...)
		}
	default:
		m.all.rows = append(m.all.rows, rows...)
		m.all.moreOlder = result.page.More
	}
	// Evict the opposite edge, copying so old previews are no longer retained.
	actions := 0
	if m.all.rows[0].kind == rowCreate {
		actions = 1
	}
	if excess := len(m.all.rows) - actions - 3*data.ThoughtPageSize; excess > 0 {
		if result.query.Direction == data.ThoughtsNewer {
			m.all.rows = slices.Clone(m.all.rows[:len(m.all.rows)-excess])
			m.all.moreOlder = true
		} else {
			m.all.rows = slices.Clone(m.all.rows[excess+actions:])
			m.all.moreNewer = true
		}
	}
	for index, row := range m.all.rows {
		if row.kind == selected.kind && (row.kind == rowCreate || row.item.ThoughtID == selected.item.ThoughtID) {
			m.all.index = index
			break
		}
	}
	m.all.offset += (m.all.index - oldIndex) * summaryLines
	m.all.index = min(max(0, m.all.index+result.move), len(m.all.rows)-1)
	m.all.keepVisible()
	return m, nil
}

func (m Model) updateAll(msg tea.Msg) (Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	// Refresh may supersede a pending batch. Other commands wait for that batch.
	if key.String() == "home" || key.String() == "r" {
		cmd := m.latestThoughts()
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
		move = -max(1, m.all.height/summaryLines)
	case "pgdown":
		move = max(1, m.all.height/summaryLines)
	case "enter":
		row := m.all.rows[m.all.index]
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
	if m.all.index+move < 0 && m.all.moreNewer {
		cursor := m.all.rows[0].item.Cursor()
		cmd := m.browseThoughts(data.ThoughtPageRequest{Cursor: &cursor, Direction: data.ThoughtsNewer}, move)
		return m, cmd
	}
	if m.all.index+move >= len(m.all.rows) && m.all.moreOlder {
		cursor := m.all.rows[len(m.all.rows)-1].item.Cursor()
		cmd := m.browseThoughts(data.ThoughtPageRequest{Cursor: &cursor}, move)
		return m, cmd
	}
	m.all.index = min(max(0, m.all.index+move), len(m.all.rows)-1)
	m.all.keepVisible()
	return m, nil
}

func (s *allState) keepVisible() {
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

func (m Model) viewAll(status string) string {
	s := m.all
	lines := make([]string, 0, len(s.rows)*summaryLines)
	count := 0
	for index, row := range s.rows {
		titleStyle, descStyle := m.itemStyles.NormalTitle, m.itemStyles.NormalDesc
		if index == s.index {
			titleStyle, descStyle = m.itemStyles.SelectedTitle, m.itemStyles.SelectedDesc
		}
		title, description := "Create thought…", "Write a new thought"
		if row.kind == rowRecord {
			count++
			title = fmt.Sprintf("Thought %d • %s", row.item.ThoughtID, summaryText(row.item.Preview))
			subject := "Misc"
			if row.item.SubjectName != nil {
				subject = summaryText(*row.item.SubjectName)
			}
			description = displaytime.Format(row.item.ObservedAt, "Jan 2, 2006 3:04 PM MST") + " • " + m.list.Styles.Title.Render(subject)
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
	if count == 1 {
		label = "thought"
	}
	return strings.Join(visible, "\n") + fmt.Sprintf("\n%d %s loaded\n%s\n↑/↓: select • PgUp/PgDn: scroll\nEnter: open • Home/r: latest", count, label, strings.TrimSpace(status))
}
