package tui

import (
	"context"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/tui/events"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/tui/listfilter"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
	"github.com/jhern254/go-thoughts/internal/voice"
)

const (
	defaultWidth  = 80
	defaultHeight = 24
)

type screen uint8

const (
	screenEvents screen = iota
	screenSubjectList
	screenSubjectCreate
	screenSubjectDetail
	screenSubjectEdit
	screenSubjectDelete
	screenMiscThoughts
	screenBrowseThoughts
	screenVoiceThought
)

type entityKind uint8

const (
	entitySubjects entityKind = iota
	entityThoughts
)

func (entity entityKind) title() string {
	if entity == entityThoughts {
		return "Thoughts"
	}
	return "Subjects"
}

type Model struct {
	ctx  context.Context
	user *data.User
	// screen selects the root child that owns ordinary messages and rendering.
	screen                  screen
	subjectReturn           *events.CreateSubjectRequest
	logger                  logging.Logger
	browserRecordingEnabled bool
	voiceDraftID            uint64
	voiceSubjectReturn      *thoughts.VoiceSubjectRequest
	voiceSubjectScreen      screen

	// Entity selection and panel focus are separate: selectedEntity remembers the
	// strip choice while entityFocused decides whether the strip or Events owns keys.
	selectedEntity entityKind
	entityFocused  bool
	width          int
	exitPromptOpen bool
	exitYes        bool
	events         events.Model
	subjects       subjectState
	thoughts       thoughts.Model
	metrics        MetricsService
}

func NewModel(ctx context.Context, user *data.User, subjects SubjectService, thoughtService thoughts.Service, metrics MetricsService, eventService events.Service, timelineView events.TimelineView, logger logging.Logger) Model {
	model := Model{
		ctx:      ctx,
		user:     user,
		screen:   screenEvents,
		logger:   logger,
		width:    defaultWidth,
		events:   events.New(ctx, user.UserID, eventService, timelineView, thoughtService, subjects, logger),
		subjects: newSubjectState(subjects),
		thoughts: thoughts.New(ctx, user.UserID, thoughtService, logger),
		metrics:  metrics,
	}
	model.events.Resize(defaultWidth, defaultHeight-6)
	return model
}

type homeOpened struct{}

func (Model) Init() tea.Cmd { return func() tea.Msg { return homeOpened{} } }

func (m Model) openHome() (tea.Model, tea.Cmd) {
	m.subjects.stopRead()
	m.screen = screenEvents
	m.events.SetFocused(!m.entityFocused)
	cmd := m.events.Open()
	return m, cmd
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.update(message)
	next := updated.(Model)
	if next.ctx.Err() == nil && next.browserRecordingEnabled && next.thoughts.Creating() && !next.thoughts.VoiceOpen() {
		next.voiceDraftID++
		name := ""
		if next.screen == screenSubjectDetail && next.subjects.selected != nil {
			name = next.subjects.selected.SubjectName
		}
		voiceCmd := next.thoughts.EnableVoice(next.voiceDraftID, next.subjects.service, name)
		return next, tea.Batch(cmd, voiceCmd)
	}
	return next, cmd
}

func (m Model) update(message tea.Msg) (tea.Model, tea.Cmd) {
	if m.ctx.Err() != nil {
		return m, nil
	}
	if m.exitPromptOpen {
		switch message := message.(type) {
		case tea.KeyPressMsg:
			return m.updateExitPrompt(message)
		case tea.PasteMsg, thoughts.VoiceAction:
			return m, nil
		}
	}
	if key, ok := message.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "ctrl+c":
			if m.canExit() {
				m.exitPromptOpen, m.exitYes = true, false
			}
			return m, nil
		case "esc":
			if m.canExit() {
				m.exitPromptOpen, m.exitYes = true, false
				return m, nil
			}
		}
	}
	if events.Owns(message) {
		var cmd tea.Cmd
		m.events, cmd = m.events.Update(message)
		return m, cmd
	}
	switch message := message.(type) {
	case thoughts.BrowserRecordingEnabledMsg:
		m.browserRecordingEnabled = true
		m.thoughts, _ = m.thoughts.Update(message)
		return m, nil
	case voice.TranscriptUpdate, voice.RecordingEnded:
		var cmd tea.Cmd
		m.thoughts, cmd = m.thoughts.Update(message)
		return m, cmd
	case thoughts.VoiceAction:
		if message.Action == "open" {
			if !m.VoiceState().CanOpenThoughtDraft {
				return m, nil
			}
			m.events.Close()
			m.subjects.stopRead()
			m.screen = screenVoiceThought
			m.voiceDraftID++
			return m, m.thoughts.OpenVoice(m.voiceDraftID, m.subjects.service)
		}
		if m.thoughts.VoiceOpen() {
			var cmd tea.Cmd
			m.thoughts, cmd = m.thoughts.Update(message)
			return m, cmd
		}
		return m, nil
	case thoughts.VoiceSubjectRequest:
		if !m.thoughts.AwaitingVoiceSubject(message) {
			return m, nil
		}
		m.voiceSubjectReturn = &message
		m.voiceSubjectScreen = m.screen
		return m.openSubjectCreate(message.Query)
	case events.CreateSubjectRequest:
		if m.screen != screenEvents || !m.events.AwaitingSubject(message) {
			return m, nil
		}
		m.subjectReturn = &message
		return m.openSubjectCreate(message.Query)
	case homeOpened:
		if m.screen != screenEvents {
			return m, nil
		}
		return m.openHome()
	case list.FilterMatchesMsg:
		return m, nil
	case listfilter.Reply:
		if m.subjects.filter.Owns(message) {
			cmd := m.subjects.filter.Update(&m.subjects.list, message)
			return m, cmd
		}
		var cmd tea.Cmd
		m.thoughts, cmd = m.thoughts.Update(message)
		return m, cmd
	case tea.WindowSizeMsg:
		m.width = message.Width
		m.events.Resize(message.Width, max(1, message.Height-6))
		m.resizeSubjects(message.Width, message.Height)
		m.thoughts.Resize(message.Width, max(1, message.Height-8))
		return m, nil
	case thoughts.Result, thoughts.BrowseThoughtsResult, thoughts.ThoughtBrowseStatsResult:
		var cmd, eventCmd tea.Cmd
		m.thoughts, cmd = m.thoughts.Update(message)
		m.events, eventCmd = m.events.Update(message)
		return m, tea.Batch(cmd, eventCmd)
	case thoughts.ChangedMsg:
		m.subjects.listStale = true
		if m.screen == screenSubjectList && !m.subjects.loading {
			return m.openSubjects()
		}
		return m, nil
	case subjectsListedMsg:
		if m.screen != screenSubjectList || message.request != m.subjects.readRequest {
			return m, nil
		}
		return m.handleSubjectsListed(message)
	case subjectCreatedMsg:
		return m.handleSubjectCreated(message)
	case subjectUpdatedMsg:
		return m.handleSubjectUpdated(message)
	case subjectDeletedMsg:
		return m.handleSubjectDeleted(message)
	case subjectFoundMsg:
		if message.request != m.subjects.readRequest {
			return m, nil
		}
		return m.handleSubjectFound(message)
	case tea.KeyPressMsg:
		if message.String() == "t" && m.VoiceState().CanOpenThoughtDraft {
			return m.update(thoughts.VoiceAction{Action: "open"})
		}
	}

	switch m.screen {
	case screenEvents:
		return m.updateHome(message)
	case screenSubjectList:
		return m.updateSubjectList(message)
	case screenSubjectCreate:
		return m.updateSubjectCreate(message)
	case screenSubjectEdit:
		return m.updateSubjectEdit(message)
	case screenSubjectDelete:
		return m.updateSubjectDelete(message)
	case screenSubjectDetail:
		return m.updateSubjectDetail(message)
	case screenMiscThoughts:
		return m.updateMiscThoughts(message)
	case screenVoiceThought:
		if key, ok := message.(tea.KeyPressMsg); ok && (key.String() == "esc" || key.String() == "q") && m.thoughts.ShowingDetail() {
			m.thoughts.Reset()
			return m.openHome()
		}
		var cmd tea.Cmd
		m.thoughts, cmd = m.thoughts.Update(message)
		if m.thoughts.Browsing() {
			return m.openHome()
		}
		return m, cmd
	case screenBrowseThoughts:
		if key, ok := message.(tea.KeyPressMsg); ok && m.thoughts.Browsing() {
			switch key.String() {
			case "q", "esc":
				m.thoughts.Reset()
				return m.openHome()
			}
		}
		var cmd tea.Cmd
		m.thoughts, cmd = m.thoughts.Update(message)
		return m, cmd
	default:
		return m, nil
	}
}

func (m Model) updateHome(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyPressMsg); ok && m.events.CanLeave() {
		if key.String() == "tab" {
			m.entityFocused = !m.entityFocused
			m.events.SetFocused(!m.entityFocused)
			return m, nil
		}
		if m.entityFocused {
			switch key.String() {
			case "q":
				m.entityFocused = false
				m.events.SetFocused(true)
				return m, nil
			case "left", "right", "h", "l":
				if m.selectedEntity == entitySubjects {
					m.selectedEntity = entityThoughts
				} else {
					m.selectedEntity = entitySubjects
				}
				return m, nil
			case "enter":
				m.events.Close()
				if m.selectedEntity == entitySubjects {
					return m.openSubjects()
				}
				m.screen = screenBrowseThoughts
				cmd := m.thoughts.OpenBrowseThoughtsView(m.metrics)
				return m, cmd
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.events, cmd = m.events.Update(message)
	return m, cmd
}

func (m Model) entityStrip() string {
	parts := []string{}
	for _, entity := range []entityKind{entityThoughts, entitySubjects} {
		label := entity.title()
		if m.selectedEntity == entity && m.entityFocused {
			label = m.subjects.list.Styles.Title.Render(label)
		}
		parts = append(parts, label)
	}
	line := strings.Join(parts, "   ")
	if ansi.StringWidth(line) > m.width {
		// Keep the selected entry visible instead of clipping it off-screen.
		line = m.selectedEntity.title()
		if m.entityFocused {
			line = m.subjects.list.Styles.Title.Render(line)
		}
	}
	help := "Tab: entities"
	if m.entityFocused {
		help = "Tab/Q: events • ←/→: entity • Enter: open"
	}
	if m.canExit() {
		help += " • Esc/Ctrl+C: exit"
		if m.entityFocused && m.width < 60 {
			help = "Esc/Ctrl+C: exit • Q: back • Enter: open"
		}
	}
	if m.browserRecordingEnabled && ansi.StringWidth(help+" • t: create thought") <= m.width {
		help += " • t: create thought"
	}
	return ansi.Truncate(line, max(1, m.width), "…") + "\n" + ansi.Truncate(help, max(1, m.width), "…")
}

func (m Model) View() tea.View {
	view := tea.NewView("")
	view.AltScreen = true
	if m.exitPromptOpen {
		view.Content = m.viewExitPrompt()
		return view
	}
	switch m.screen {
	case screenSubjectDetail, screenMiscThoughts, screenBrowseThoughts, screenVoiceThought:
		if m.thoughts.Creating() {
			heading := m.subjects.list.Styles.TitleBar.Render(m.subjects.list.Styles.Title.Render("Thoughts"))
			view.Content = heading + "\n" + m.thoughts.View()
			return view
		}
		if m.thoughts.ShowingDetail() {
			name := m.thoughts.SelectedSubjectName()
			if m.screen == screenSubjectDetail && m.subjects.selected != nil && name == "Subject" {
				name = m.subjects.selected.SubjectName
			}
			heading := m.subjects.list.Styles.Title.Render(name)
			view.Content = heading + "\n\n" + m.thoughts.View()
			return view
		}
	}
	var content string
	switch m.screen {
	case screenEvents:
		content = m.events.View()
		if m.events.CanLeave() {
			content = ansi.Truncate("Local user: "+localUserLabel(m.user), max(1, m.width), "…") + "\n\n" + content
			content += "\n" + strings.Repeat("─", max(1, m.width)) + "\n" + m.entityStrip()
		}
	case screenSubjectList:
		content = m.viewSubjectList()
	case screenSubjectCreate:
		content = m.viewSubjectCreate()
	case screenSubjectEdit:
		content = m.viewSubjectEdit()
	case screenSubjectDelete:
		content = m.viewSubjectDelete()
	case screenSubjectDetail:
		content = m.viewSubjectDetail()
	case screenVoiceThought:
		content = m.thoughts.View()
	case screenMiscThoughts:
		content = "Misc thoughts\n\n" + m.thoughts.View()
		if m.thoughts.Browsing() {
			content += "\nQ/Esc: subjects"
		}
	case screenBrowseThoughts:
		heading := m.subjects.list.Styles.TitleBar.Render(m.subjects.list.Styles.Title.Render("Thoughts"))
		content = heading + "\n" + m.thoughts.View()
		if m.thoughts.Browsing() {
			content += "\nQ/Esc: entities"
		}
	}
	view.Content = content
	return view
}

func localUserLabel(user *data.User) string {
	if user.Handle != nil {
		return fmt.Sprintf("%s (%s)", *user.Handle, user.UserID)
	}
	return user.UserID
}

// VoiceState exposes only bounded control metadata to the browser bridge.
func (m Model) VoiceState() thoughts.VoiceState {
	state := thoughts.VoiceState{BrowserRecordingEnabled: m.browserRecordingEnabled}
	if !m.browserRecordingEnabled || m.exitPromptOpen {
		return state
	}
	switch m.screen {
	case screenEvents:
		state.CanOpenThoughtDraft = m.events.CanLeave()
	case screenSubjectList:
		state.CanOpenThoughtDraft = !m.subjects.loading && !m.subjects.list.SettingFilter() && !m.subjects.list.IsFiltered()
	case screenBrowseThoughts, screenMiscThoughts, screenSubjectDetail:
		state.CanOpenThoughtDraft = m.thoughts.Browsing() || m.thoughts.ShowingDetail()
	case screenVoiceThought:
		state.CanOpenThoughtDraft = m.thoughts.ShowingDetail()
	}
	if m.thoughts.VoiceOpen() && m.voiceSubjectReturn == nil {
		state = m.thoughts.VoiceState()
		state.BrowserRecordingEnabled = true
	}
	return state
}
