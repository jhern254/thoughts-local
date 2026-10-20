package tui

import (
	"context"
	"fmt"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/diagnostics"
	"github.com/jhern254/go-thoughts/internal/logging"
)

type aliasMode uint8

const (
	aliasBrowse aliasMode = iota
	aliasEdit
	aliasRemove
)

// Reserve the title/primary block, section heading, feedback, and wrapped help;
// the Bubbles list pages aliases within the remaining terminal rows.
const subjectAliasReservedRows = 9

type subjectAliasState struct {
	session      *int
	readRequest  *int
	writeRequest *int
	cancelRead   context.CancelFunc
	mode         aliasMode
	list         list.Model
	input        textinput.Model
	editing      data.SubjectAlias
	selectedID   int64
	loading      bool
	err          error
	errMessage   string
}

type subjectAliasRow struct{ alias data.SubjectAlias }

func (row subjectAliasRow) Title() string {
	if row.alias.AliasID == 0 {
		return "+ Add alias…"
	}
	return row.alias.AliasName
}
func (row subjectAliasRow) Description() string { return "" }
func (row subjectAliasRow) FilterValue() string { return row.Title() }

type subjectAliasesListedMsg struct {
	session   *int
	request   *int
	subjectID int64
	aliases   []data.SubjectAlias
	err       error
}

type subjectAliasSavedMsg struct {
	session   *int
	request   *int
	subjectID int64
	alias     *data.SubjectAlias
	err       error
}

func newSubjectAliasState() subjectAliasState {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetSpacing(0)
	aliases := list.New([]list.Item{subjectAliasRow{}}, delegate, defaultWidth, defaultHeight-subjectAliasReservedRows)
	aliases.DisableQuitKeybindings()
	aliases.SetFilteringEnabled(false)
	aliases.SetShowTitle(false)
	aliases.SetShowStatusBar(false)
	aliases.SetShowHelp(false)
	input := textinput.New()
	input.Prompt = "Alias name: "
	return subjectAliasState{list: aliases, input: input}
}

func (s *subjectAliasState) stopRead() {
	if s.cancelRead != nil {
		s.cancelRead()
	}
	s.readRequest = nil
}

func (m Model) openSubjectAliases() (tea.Model, tea.Cmd) {
	if m.subjects.selected == nil {
		return m, nil
	}
	m.subjects.aliases.stopRead()
	m.subjects.aliases = newSubjectAliasState()
	m.subjects.aliases.session = new(int)
	m.screen = screenSubjectAliases
	m.resizeSubjectAliases()
	cmd := m.listSubjectAliases()
	return m, cmd
}

func (m *Model) resizeSubjectAliases() {
	m.subjects.aliases.list.SetSize(max(1, m.width), max(1, m.height-subjectAliasReservedRows))
	m.subjects.aliases.input.SetWidth(max(1, m.width-ansi.StringWidth(m.subjects.aliases.input.Prompt)-1))
}

func (m *Model) listSubjectAliases() tea.Cmd {
	state := &m.subjects.aliases
	state.stopRead()
	ctx, cancel := context.WithCancel(m.ctx)
	state.cancelRead = cancel
	state.readRequest = new(int)
	state.loading = true
	session, request := state.session, state.readRequest
	service, userID, subjectID := m.subjects.service, m.user.UserID, m.subjects.selected.SubjectID
	return func() tea.Msg {
		if ctx.Err() != nil {
			return nil
		}
		aliases, err := service.ListAliases(ctx, userID, subjectID)
		if ctx.Err() != nil {
			return nil
		}
		return subjectAliasesListedMsg{session: session, request: request, subjectID: subjectID, aliases: aliases, err: err}
	}
}

func (m Model) receiveSubjectAliases(message subjectAliasesListedMsg) (tea.Model, tea.Cmd) {
	state := &m.subjects.aliases
	if m.screen != screenSubjectAliases || message.session != state.session || message.request != state.readRequest ||
		m.subjects.selected == nil || message.subjectID != m.subjects.selected.SubjectID {
		return m, nil
	}
	state.stopRead()
	state.loading = false
	state.err = message.err
	if message.err != nil {
		logSubjectError(m.logger, logging.SubjectList, message.err)
		state.errMessage = diagnostics.SubjectMessage(message.err, "Could not load aliases. Press r to retry.")
		return m, nil
	}
	rows := make([]list.Item, 0, len(message.aliases)+1)
	rows = append(rows, subjectAliasRow{})
	selected := state.list.Index()
	for _, alias := range message.aliases {
		rows = append(rows, subjectAliasRow{alias: alias})
		if alias.AliasID == state.selectedID {
			selected = len(rows) - 1
		}
	}
	cmd := state.list.SetItems(rows)
	state.list.Select(min(selected, len(rows)-1))
	state.selectedID = 0
	return m, cmd
}

func (m *Model) saveSubjectAlias() tea.Cmd {
	state := &m.subjects.aliases
	state.loading = true
	state.err = nil
	state.writeRequest = new(int)
	session, request := state.session, state.writeRequest
	mode, aliasID, name := state.mode, state.editing.AliasID, state.input.Value()
	ctx, service, userID, subjectID := m.ctx, m.subjects.service, m.user.UserID, m.subjects.selected.SubjectID
	// Confirmed writes belong to the application session. Navigation is blocked
	// until the result arrives; canceling a later read cannot undo a committed write.
	return func() tea.Msg {
		if ctx.Err() != nil {
			return nil
		}
		var alias *data.SubjectAlias
		var err error
		switch {
		case mode == aliasRemove:
			err = service.RemoveAlias(ctx, userID, subjectID, aliasID)
		case aliasID == 0:
			alias, err = service.CreateAlias(ctx, userID, subjectID, name)
		default:
			alias, err = service.UpdateAlias(ctx, userID, subjectID, aliasID, name)
		}
		return subjectAliasSavedMsg{session: session, request: request, subjectID: subjectID, alias: alias, err: err}
	}
}

func (m Model) receiveSubjectAliasSaved(message subjectAliasSavedMsg) (tea.Model, tea.Cmd) {
	state := &m.subjects.aliases
	if m.screen != screenSubjectAliases || message.session != state.session || message.request != state.writeRequest ||
		m.subjects.selected == nil || message.subjectID != m.subjects.selected.SubjectID {
		return m, nil
	}
	state.writeRequest = nil
	state.loading = false
	state.err = message.err
	if message.err != nil {
		logSubjectError(m.logger, logging.SubjectUpdate, message.err)
		state.errMessage = diagnostics.SubjectMessage(message.err, "Could not save the alias.")
		if state.mode == aliasEdit {
			return m, state.input.Focus()
		}
		return m, nil
	}
	state.input.Blur()
	state.input.Reset()
	state.mode = aliasBrowse
	if message.alias != nil {
		state.selectedID = message.alias.AliasID
	}
	m.logger.Mutation(logging.SubjectUpdated, message.subjectID)
	cmd := m.listSubjectAliases()
	return m, cmd
}

func (m Model) updateSubjectAliases(message tea.Msg) (tea.Model, tea.Cmd) {
	state := &m.subjects.aliases
	if state.writeRequest != nil {
		return m, nil
	}
	if key, ok := message.(tea.KeyPressMsg); ok {
		switch state.mode {
		case aliasEdit:
			switch key.String() {
			case "esc":
				state.input.Blur()
				state.input.Reset()
				state.mode, state.err = aliasBrowse, nil
				return m, nil
			case "enter":
				cmd := m.saveSubjectAlias()
				return m, cmd
			}
		case aliasRemove:
			switch key.String() {
			case "esc", "q", "n":
				state.mode, state.err = aliasBrowse, nil
				return m, nil
			case "y":
				cmd := m.saveSubjectAlias()
				return m, cmd
			}
			return m, nil
		default:
			switch key.String() {
			case "esc", "q":
				state.stopRead()
				state.session = nil
				m.screen = screenSubjectDetail
				// Keep the existing Thought browser, including selection and paging.
				return m, nil
			case "r":
				if !state.loading {
					cmd := m.listSubjectAliases()
					return m, cmd
				}
			case "enter", "d":
				if state.loading || state.err != nil {
					return m, nil
				}
				row, ok := state.list.SelectedItem().(subjectAliasRow)
				if !ok {
					return m, nil
				}
				state.editing, state.err = row.alias, nil
				if key.String() == "d" {
					if row.alias.AliasID != 0 {
						state.mode = aliasRemove
					}
					return m, nil
				}
				state.mode = aliasEdit
				state.input.SetValue(row.alias.AliasName)
				state.input.CursorEnd()
				return m, state.input.Focus()
			}
		}
	}
	var cmd tea.Cmd
	if state.mode == aliasEdit {
		state.input, cmd = state.input.Update(message)
	} else {
		state.list, cmd = state.list.Update(message)
	}
	return m, cmd
}

func (m Model) viewSubjectAliases() string {
	state := m.subjects.aliases
	width := max(1, m.width)
	titleStyle := m.subjects.list.Styles.Title
	heading := titleStyle.Render(ansi.Truncate("Names & Aliases — "+m.subjects.selected.SubjectName, max(1, width-titleStyle.GetHorizontalFrameSize()), "…"))
	primaryLabel := "  Primary"
	primary := ansi.Truncate(m.subjects.selected.SubjectName, max(1, width-ansi.StringWidth(primaryLabel)), "…") + primaryLabel
	status := ""
	if state.loading {
		status = "Loading…"
	}
	if state.err != nil {
		status = state.errMessage
	}
	content, help := "Aliases\n"+state.list.View(), "Enter: add/edit • d: remove • Q/Esc: back"
	switch state.mode {
	case aliasEdit:
		label := "Add alias"
		if state.editing.AliasID != 0 {
			label = "Edit alias"
		}
		content, help = label+"\n"+state.input.View(), "Enter: save • Esc: cancel"
	case aliasRemove:
		name := ansi.Truncate(fmt.Sprintf("Remove %q?", state.editing.AliasName), width, "…")
		content = name + "\nOnly this alias will be removed.\nSubject, Thoughts, Events, and Goals stay unchanged."
		help = "y: remove • n/Q/Esc: cancel"
	}
	// Wrap feedback/help independently of the paged list so narrow terminals
	// retain the same vertical controls without overflowing long names.
	style := lipgloss.NewStyle().Width(width)
	return heading + "\n\n" + primary + "\n\n" + style.Render(content) + "\n" + style.Render(status) + "\n" + style.Render(help)
}
