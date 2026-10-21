package thoughts

import (
	"bytes"
	"charm.land/bubbles/v2/list"
	"github.com/charmbracelet/x/ansi"
	"image/color"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/diagnostics"
	"github.com/jhern254/go-thoughts/internal/failure"
	"github.com/jhern254/go-thoughts/internal/logging"
)

// MindsetChanged updates resident previews without disturbing selection.
type MindsetChanged struct {
	ThoughtID int64
	Active    bool
}
type MindsetResult struct {
	owner     *int
	request   uint64
	thoughtID int64
	active    bool
	err       error
}

func MindsetBadge() string {
	return lipgloss.NewStyle().Background(color.White).Foreground(color.Black).Padding(0, 1).Render("M")
}

func (m Model) SavingMindset() bool { return m.mindsetSaving }

func (m *Model) OpenDetail(item data.ThoughtSummaryView) tea.Cmd {
	m.Reset()
	m.directThoughtID = &item.ThoughtID
	if item.SubjectName != nil {
		m.selectedSubjectName = *item.SubjectName
	}
	return m.getThought(item.ThoughtID)
}

func (m *Model) setMindset() tea.Cmd {
	if m.loading || m.mindsetSaving || m.screen == create || m.list.SettingFilter() {
		return nil
	}
	var id int64
	var active bool
	if m.screen == detail && m.selected != nil {
		id, active = m.selected.ThoughtID, m.selected.IsMindset
	} else if m.browsingThoughtsView {
		if len(m.browseThoughts.rows) == 0 {
			return nil
		}
		row := m.browseThoughts.rows[m.browseThoughts.index]
		if row.kind != rowRecord {
			return nil
		}
		id, active = row.item.ThoughtID, row.item.IsMindset
	} else {
		selected, ok := m.list.SelectedItem().(row)
		if !ok || selected.kind != rowRecord {
			return nil
		}
		id, active = selected.item.ThoughtID, selected.item.IsMindset
		// Filtered rows may still await their refreshed matches after a write.
		// The source row already contains the last accepted designation.
		for _, source := range m.list.Items() {
			current, ok := source.(row)
			if ok && current.kind == rowRecord && current.item.ThoughtID == id {
				active = current.item.IsMindset
				break
			}
		}
	}
	m.mindsetSaving = true
	m.err = nil
	ctx, service, user, owner, request := m.ctx, m.service, m.userID, m.owner, m.request
	at := m.now()
	return func() tea.Msg {
		active, err := service.SetMindset(ctx, user, id, !active, at)
		return MindsetResult{owner: owner, request: request, thoughtID: id, active: active, err: err}
	}
}

func (m Model) receiveMindset(result MindsetResult) (Model, tea.Cmd) {
	if result.owner != m.owner {
		return m, nil
	}
	m.mindsetSaving = false
	if result.err != nil {
		if category, emit := failure.Classify(logging.ThoughtMindsetSet, result.err); emit {
			m.logger.Failure(logging.ThoughtMindsetSet, category)
		}
		if result.request == m.request {
			m.err = result.err
			m.errMessage = diagnostics.ThoughtMessage(result.err, "Could not change mindset. Press m to retry.")
		}
		return m, nil
	}
	m.applyMindset(MindsetChanged{ThoughtID: result.thoughtID, Active: result.active})
	m.logger.Mutation(logging.ThoughtMindsetChanged, result.thoughtID)
	return m, func() tea.Msg { return MindsetChanged{ThoughtID: result.thoughtID, Active: result.active} }
}

func (m *Model) applyMindset(change MindsetChanged) tea.Cmd {
	if m.selected != nil && m.selected.ThoughtID == change.ThoughtID {
		m.selected.IsMindset = change.Active
	}
	for i := range m.browseThoughts.rows {
		item := &m.browseThoughts.rows[i].item
		if item.ThoughtID == change.ThoughtID {
			item.IsMindset = change.Active
		}
	}
	for i, item := range m.list.Items() {
		row, ok := item.(row)
		if ok && row.kind == rowRecord && row.item.ThoughtID == change.ThoughtID {
			row.item.IsMindset = change.Active
			return m.filter.SetItem(&m.list, i, row)
		}
	}
	return nil
}

// Decorate after the default delegate highlights filter matches. ANSI badges
// inside Title would shift match offsets and split their escape sequences.
type mindsetDelegate struct{ list.DefaultDelegate }

func (d mindsetDelegate) Render(writer io.Writer, model list.Model, index int, item list.Item) {
	thought, ok := item.(row)
	if !ok || thought.kind != rowRecord || !thought.item.IsMindset {
		d.DefaultDelegate.Render(writer, model, index, item)
		return
	}
	var rendered bytes.Buffer
	d.DefaultDelegate.Render(&rendered, model, index, item)
	title, description, hasDescription := strings.Cut(rendered.String(), "\n")
	// Both normal and selected default titles reserve the same left frame.
	frame := d.Styles.NormalTitle.GetHorizontalFrameSize()
	title = ansi.Cut(title, 0, frame) + MindsetBadge() + " " + ansi.Cut(title, frame, ansi.StringWidth(title))
	io.WriteString(writer, ansi.Truncate(title, max(1, model.Width()), "…"))
	if hasDescription {
		io.WriteString(writer, "\n"+description)
	}
}
