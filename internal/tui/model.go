package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/subjectdiscovery"
	"github.com/jhern254/go-thoughts/internal/tui/events"
	"github.com/jhern254/go-thoughts/internal/tui/listfilter"
	"github.com/jhern254/go-thoughts/internal/tui/mindset"
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
	screenMindsetThought
	screenSubjectList
	screenSubjectCreate
	screenSubjectFromAlias
	screenSubjectDetail
	screenSubjectEdit
	screenSubjectDelete
	screenSubjectAliases
	screenMiscThoughts
	screenBrowseThoughts
	screenVoiceThought
	screenOptions
)

type entityKind uint8

const (
	entitySubjects entityKind = iota
	entityThoughts
	entityOptions
)

func (entity entityKind) title() string {
	if entity == entityOptions {
		return "Options"
	}
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
	browserOptionsEnabled   bool
	voiceDraftID            uint64
	voiceSubjectReturn      *thoughts.VoiceSubjectRequest
	voiceSubjectScreen      screen

	// Entity selection and panel focus are separate: selectedEntity remembers the
	// strip choice while entityFocused decides whether the strip or Events owns keys.
	selectedEntity                    entityKind
	entityFocused                     bool
	width                             int
	height                            int
	options                           optionsState
	speechSetup                       speechSetupState
	exitPromptOpen                    bool
	exitYes                           bool
	mindsets                          mindset.Model
	mindsetsNeedRefresh               bool
	homeEventsHeight, homeEventsWidth int
	events                            events.Model
	subjects                          subjectState
	thoughts                          thoughts.Model
	metrics                           MetricsService
}

func NewModel(ctx context.Context, user *data.User, subjects SubjectService, thoughtService ThoughtService, metrics MetricsService, eventService events.Service, timelineView events.TimelineView, logger logging.Logger) Model {
	model := Model{
		ctx:      ctx,
		user:     user,
		screen:   screenEvents,
		logger:   logger,
		width:    defaultWidth,
		height:   defaultHeight,
		events:   events.New(ctx, user.UserID, eventService, timelineView, thoughtService, subjects, logger),
		subjects: newSubjectState(subjects),
		thoughts: thoughts.New(ctx, user.UserID, thoughtService, logger),
		metrics:  metrics,
		mindsets: mindset.New(ctx, user.UserID, thoughtService, logger),
	}
	model.resizeHome()
	return model
}

type homeOpened struct{}

func (m Model) Init() tea.Cmd {
	home := func() tea.Msg { return homeOpened{} }
	if m.speechSetup.service != nil {
		return tea.Batch(home, m.checkInstalledSpeechModel())
	}
	return home
}

func (m Model) openHome() (tea.Model, tea.Cmd) {
	m.subjects.stopRead()
	m.screen = screenEvents
	m.events.SetFocused(!m.entityFocused && !m.mindsets.Focused())
	m.mindsetsNeedRefresh = true
	cmd := m.events.Open()
	return m, cmd
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.update(message)
	next := updated.(Model)
	next.offerSpeechSetup()
	if next.ctx.Err() == nil {
		cmd = tea.Batch(cmd, next.syncMindsets())
	}
	if next.ctx.Err() == nil && next.browserRecordingEnabled && next.thoughts.Creating() && !next.thoughts.VoiceOpen() {
		next.voiceDraftID++
		name := ""
		if next.screen == screenSubjectDetail && next.subjects.selected != nil {
			name = next.subjects.selected.SubjectName
		}
		next.thoughts.ConfigureVoiceSubjectDiscovery(subjectdiscovery.NewService(next.subjects.service, next.metrics, subjectdiscovery.NameRecommender{}))
		voiceCmd := next.thoughts.EnableVoice(next.voiceDraftID, next.subjects.service, name)
		return next, tea.Batch(cmd, voiceCmd)
	}
	return next, cmd
}

func (m Model) update(message tea.Msg) (tea.Model, tea.Cmd) {
	if m.ctx.Err() != nil {
		return m, nil
	}
	switch message.(type) {
	case speechSetupChecked, speechSetupProgress, speechSetupFinished:
		return m.updateSpeechSetup(message)
	}
	if m.exitPromptOpen {
		switch message := message.(type) {
		case tea.KeyPressMsg:
			return m.updateExitPrompt(message)
		case tea.PasteMsg, thoughts.VoiceAction:
			return m, nil
		}
	}
	if m.speechSetup.open && !m.exitPromptOpen {
		switch msg := message.(type) {
		case tea.KeyPressMsg, tea.PasteMsg, tea.MouseClickMsg, tea.MouseReleaseMsg, tea.MouseMotionMsg, tea.MouseWheelMsg:
			return m.updateSpeechSetupInput(message)
		case thoughts.VoiceAction:
			if msg.Action == "start" || msg.Action == "open" {
				return m, nil
			}
		}
	}
	if msg, ok := message.(BrowserOptionsMsg); ok {
		return m.updateBrowserOptions(msg)
	}
	if m.options.open {
		switch message.(type) {
		case tea.KeyPressMsg, tea.PasteMsg, tea.MouseClickMsg, tea.MouseReleaseMsg, tea.MouseMotionMsg, tea.MouseWheelMsg:
			return m.updateOptionsInput(message)
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
	if mindset.Owns(message) {
		var cmd tea.Cmd
		m.mindsets, cmd = m.mindsets.Update(message)
		return m, cmd
	}
	if events.Owns(message) {
		var cmd tea.Cmd
		m.events, cmd = m.events.Update(message)
		return m, cmd
	}
	switch message := message.(type) {
	case mindset.OpenThoughtMsg:
		if m.screen != screenEvents || !m.mindsets.AcceptsOpen(message) {
			return m, nil
		}
		m.screen = screenMindsetThought
		return m, m.thoughts.OpenDetail(data.ThoughtSummaryView{ThoughtID: message.Thought.ThoughtID, SubjectName: message.Thought.SubjectName})
	case thoughts.MindsetChanged:
		var thoughtCmd, eventCmd, panelCmd tea.Cmd
		m.thoughts, thoughtCmd = m.thoughts.Update(message)
		m.events, eventCmd = m.events.Update(message)
		if !m.mindsets.Day().IsZero() {
			panelCmd = m.mindsets.Load(m.events.Day())
		}
		return m, tea.Batch(thoughtCmd, eventCmd, panelCmd)
	case BrowserOptionsEnabledMsg:
		m.browserOptionsEnabled = true
		return m, nil
	case thoughts.BrowserVoiceSessionMsg:
		m.browserRecordingEnabled = true
		if m.speechSetup.service != nil {
			message.SpeechRecognitionAvailable = m.speechSetup.ready
		}
		m.thoughts, _ = m.thoughts.Update(message)
		return m, nil
	case voice.TranscriptUpdate, voice.RecordingEnded, thoughts.VoiceSubjectsLoaded:
		var cmd tea.Cmd
		m.thoughts, cmd = m.thoughts.Update(message)
		return m, cmd
	case thoughts.VoiceAction:
		if message.Action == "open" {
			if !m.VoiceState().CanOpenThoughtDraft {
				return m, nil
			}
			command := m.openThoughtDraft()
			return m, command
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
		m.height = message.Height
		m.resizeHome()
		m.resizeSubjects(message.Width, message.Height)
		m.thoughts.Resize(message.Width, max(1, message.Height-8))
		return m, nil
	case thoughts.Result, thoughts.BrowseThoughtsResult, thoughts.ThoughtBrowseStatsResult, thoughts.MindsetResult:
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
	case subjectAliasesListedMsg:
		return m.receiveSubjectAliases(message)
	case subjectAliasSavedMsg:
		return m.receiveSubjectAliasSaved(message)
	case subjectFoundMsg:
		if message.request != m.subjects.readRequest {
			return m, nil
		}
		return m.handleSubjectFound(message)
	case tea.KeyPressMsg:
		if (message.String() == "t" || message.Code == tea.KeyF8) && m.VoiceState().CanOpenThoughtDraft {
			openDraft := m.openThoughtDraft()
			if message.Code == tea.KeyF8 {
				var startRecording tea.Cmd
				m.thoughts, startRecording = m.thoughts.Update(message)
				return m, tea.Batch(openDraft, startRecording)
			}
			return m, openDraft
		}
	}

	switch m.screen {
	case screenOptions:
		if key, ok := message.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "esc", "q":
				m.screen = screenEvents
				return m, nil
			}
		}
		return m, nil
	case screenMindsetThought:
		if key, ok := message.(tea.KeyPressMsg); ok && !m.thoughts.SavingMindset() && (key.String() == "esc" || key.String() == "q" || key.String() == "left") {
			m.thoughts.Reset()
			m.screen = screenEvents
			return m, nil
		}
		var cmd tea.Cmd
		m.thoughts, cmd = m.thoughts.Update(message)
		return m, cmd
	case screenEvents:
		return m.updateHome(message)
	case screenSubjectList:
		return m.updateSubjectList(message)
	case screenSubjectCreate:
		return m.updateSubjectCreate(message)
	case screenSubjectFromAlias:
		return m.updateSubjectFromAlias(message)
	case screenSubjectEdit:
		return m.updateSubjectEdit(message)
	case screenSubjectDelete:
		return m.updateSubjectDelete(message)
	case screenSubjectDetail:
		return m.updateSubjectDetail(message)
	case screenSubjectAliases:
		return m.updateSubjectAliases(message)
	case screenMiscThoughts:
		return m.updateMiscThoughts(message)
	case screenVoiceThought:
		if key, ok := message.(tea.KeyPressMsg); ok && (key.String() == "esc" || key.String() == "q" || key.String() == "left") && m.thoughts.ShowingDetail() && !m.thoughts.SavingMindset() {
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
			case "left", "q", "esc":
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
		if key.String() == "tab" || key.String() == "shift+tab" {
			panel := (m.homeFocus() + 1) % homePanelCount
			if key.String() == "shift+tab" {
				panel = (m.homeFocus() + homePanelCount - 1) % homePanelCount
			}
			return m, m.focusHome(panel)
		}
		if m.mindsets.Focused() {
			var cmd tea.Cmd
			m.mindsets, cmd = m.mindsets.Update(message)
			return m, cmd
		}
		if key.String() == "r" {
			m.mindsetsNeedRefresh = true
		}
		if m.entityFocused {
			switch key.String() {
			case "q":
				m.entityFocused = false
				m.events.SetFocused(true)
				return m, nil
			case "left", "right", "h", "l":
				order := []entityKind{entityThoughts, entitySubjects, entityOptions}
				for index, entity := range order {
					if entity != m.selectedEntity {
						continue
					}
					step := 1
					if key.String() == "left" || key.String() == "h" {
						step = -1
					}
					m.selectedEntity = order[(index+step+len(order))%len(order)]
					break
				}
				return m, nil
			case "enter":
				if m.selectedEntity == entityOptions {
					if m.browserOptionsEnabled {
						return m.updateBrowserOptions(BrowserOptionsMsg{Action: "open"})
					}
					m.screen = screenOptions
					return m, nil
				}
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
	for _, entity := range []entityKind{entityThoughts, entitySubjects, entityOptions} {
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
	help := "Tab/Shift+Tab: panels"
	if m.mindsets.Focused() {
		help = "Enter: open • ↑/↓: read/select • PgUp/PgDn: page • r: refresh"
	}
	if m.entityFocused {
		help = "Tab: mindset • Q: events • ←/→: entity • Enter: open"
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

func (m Model) homeDivider(focused bool) string {
	line := strings.Repeat("─", max(1, m.width))
	if focused {
		sectionHighlightColor := m.subjects.list.Styles.Title.GetBackground()
		return lipgloss.NewStyle().Foreground(sectionHighlightColor).Render(line)
	}
	return line
}

func (m Model) View() tea.View {
	view := m.viewUnderlying()
	if m.exitPromptOpen {
		return view
	}
	if m.speechSetup.open {
		return m.overlaySpeechSetupPopup(view)
	}
	if m.speechSetup.notice != "" && strings.Count(view.Content, "\n")+1 < m.height {
		view.Content += "\n" + ansi.Truncate(m.speechSetup.notice, max(1, m.width), "…")
	}
	return view
}
func (m Model) viewUnderlying() tea.View {
	if m.options.open && !m.exitPromptOpen {
		return m.viewOptions()
	}
	view := tea.NewView("")
	view.AltScreen = true
	if m.exitPromptOpen {
		view.Content = m.viewExitPrompt()
		return view
	}
	switch m.screen {
	case screenSubjectDetail, screenMiscThoughts, screenBrowseThoughts, screenVoiceThought, screenMindsetThought:
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
	case screenOptions:
		content = "Options\n\nVisual backgrounds are available in browser mode.\n\nq/Esc: events"
	case screenMindsetThought:
		content = m.thoughts.View()
	case screenEvents:
		content = m.events.View()
		if m.events.TimelineVisible() {
			content = m.mindsets.View() + "\n\n" + m.homeDivider(m.homeFocus() == homePanelEvents) + "\n" + content
			content = ansi.Truncate("Local user: "+localUserLabel(m.user), max(1, m.width), "…") + "\n\n" + content
			content += "\n" + m.homeDivider(m.entityFocused) + "\n" + m.entityStrip()
		}
	case screenSubjectList:
		content = m.viewSubjectList()
	case screenSubjectCreate:
		content = m.viewSubjectCreate()
	case screenSubjectFromAlias:
		content = m.viewSubjectFromAlias()
	case screenSubjectEdit:
		content = m.viewSubjectEdit()
	case screenSubjectDelete:
		content = m.viewSubjectDelete()
	case screenSubjectDetail:
		content = m.viewSubjectDetail()
	case screenSubjectAliases:
		content = m.viewSubjectAliases()
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
			content += "\n←/Q/Esc: events"
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

func (m *Model) openThoughtDraft() tea.Cmd {
	m.events.Close()
	m.subjects.stopRead()
	m.screen = screenVoiceThought
	m.voiceDraftID++
	m.thoughts.ConfigureVoiceSubjectDiscovery(subjectdiscovery.NewService(m.subjects.service, m.metrics, subjectdiscovery.NameRecommender{}))
	return m.thoughts.OpenVoice(m.voiceDraftID, m.subjects.service)
}

// VoiceState exposes only bounded control metadata to the browser bridge.
func (m Model) VoiceState() thoughts.VoiceState {
	state := thoughts.VoiceState{BrowserRecordingEnabled: m.browserRecordingEnabled, SpeechRecognitionAvailable: m.thoughts.VoiceState().SpeechRecognitionAvailable}
	if !m.browserRecordingEnabled || m.exitPromptOpen {
		return state
	}
	switch m.screen {
	case screenEvents:
		state.CanOpenThoughtDraft = m.events.CanLeave() && !m.speechSetup.open
	case screenSubjectList:
		state.CanOpenThoughtDraft = !m.subjects.loading && !m.subjects.list.SettingFilter() && !m.subjects.list.IsFiltered()
	case screenBrowseThoughts, screenMiscThoughts, screenSubjectDetail, screenMindsetThought:
		state.CanOpenThoughtDraft = m.thoughts.Browsing() || m.thoughts.ShowingDetail()
	case screenVoiceThought:
		state.CanOpenThoughtDraft = m.thoughts.ShowingDetail()
	}
	if m.thoughts.VoiceOpen() && m.voiceSubjectReturn == nil {
		state = m.thoughts.VoiceState()
		state.BrowserRecordingEnabled = true
	}
	if m.speechSetup.open || m.options.open || m.thoughts.SavingMindset() {
		state.CanOpenThoughtDraft = false
	}
	return state
}
