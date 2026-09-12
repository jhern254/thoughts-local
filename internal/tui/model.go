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
)

type entityKind uint8

const (
	entitySubjects entityKind = iota
	entityThoughts
)

type entityRow struct {
	kind        entityKind
	title       string
	description string
}

func (row entityRow) Title() string       { return row.title }
func (row entityRow) Description() string { return row.description }
func (row entityRow) FilterValue() string { return row.title }

type Model struct {
	ctx    context.Context
	user   *data.User
	screen screen
	logger logging.Logger

	entityList    list.Model
	entityFocused bool
	width         int
	events        events.Model
	subjects      subjectState
	thoughts      thoughts.Model
	metrics       MetricsService
}

func NewModel(ctx context.Context, user *data.User, subjects SubjectService, thoughtService thoughts.Service, metrics MetricsService, eventService events.Service, timelineView events.TimelineView, logger logging.Logger) Model {
	entities := list.New([]list.Item{
		entityRow{
			kind:        entitySubjects,
			title:       "Subjects",
			description: "Browse and organize subjects",
		},
		entityRow{kind: entityThoughts, title: "Thoughts", description: "Browse all thoughts, newest observations first"},
	}, list.NewDefaultDelegate(), defaultWidth, defaultHeight)
	entities.Title = "Entities"
	entities.SetFilteringEnabled(false)
	entities.SetShowStatusBar(false)

	return Model{
		ctx:        ctx,
		user:       user,
		screen:     screenEvents,
		logger:     logger,
		entityList: entities,
		width:      defaultWidth,
		events:     events.New(ctx, user.UserID, eventService, timelineView, thoughtService, logger),
		subjects:   newSubjectState(subjects),
		thoughts:   thoughts.New(ctx, user.UserID, thoughtService, logger),
		metrics:    metrics,
	}
}

type homeOpened struct{}

func (Model) Init() tea.Cmd { return func() tea.Msg { return homeOpened{} } }

func (m Model) openHome() (tea.Model, tea.Cmd) {
	m.screen = screenEvents
	cmd := m.events.Open()
	return m, cmd
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case homeOpened:
		if m.screen != screenEvents {
			return m, nil
		}
		return m.openHome()
	case events.Tick, events.Listed, events.Counts, events.Latest, events.Opened, events.Saved:
		var cmd tea.Cmd
		m.events, cmd = m.events.Update(message)
		return m, cmd
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
		m.events.Resize(message.Width, max(1, message.Height-4))
		m.entityList.SetSize(message.Width, max(0, message.Height-3))
		m.resizeSubjects(message.Width, message.Height)
		m.thoughts.Resize(message.Width, max(1, message.Height-8))
		return m, nil
	case thoughts.Result, thoughts.BrowseThoughtsResult, thoughts.ThoughtCountResult:
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
		return m.handleSubjectsListed(message)
	case subjectCreatedMsg:
		return m.handleSubjectCreated(message)
	case subjectUpdatedMsg:
		return m.handleSubjectUpdated(message)
	case subjectDeletedMsg:
		return m.handleSubjectDeleted(message)
	case subjectFoundMsg:
		return m.handleSubjectFound(message)
	case tea.KeyPressMsg:
		if message.String() == "ctrl+c" {
			return m, tea.Quit
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
	case screenBrowseThoughts:
		if key, ok := message.(tea.KeyPressMsg); ok && m.thoughts.Browsing() {
			switch key.String() {
			case "q":
				return m, tea.Quit
			case "esc":
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
	// Keyboard input may arrive before Init's asynchronous message. Quit is
	// available immediately, but remains authored text inside an event form.
	if key, ok := message.(tea.KeyPressMsg); ok && key.String() == "q" && !m.events.FormOpen() {
		return m, tea.Quit
	}
	if key, ok := message.(tea.KeyPressMsg); ok && m.events.CanLeave() {
		if key.String() == "tab" {
			m.entityFocused = !m.entityFocused
			m.events.Pause()
			return m, nil
		}
		if m.entityFocused {
			switch key.String() {
			case "q":
				return m, tea.Quit
			case "left", "right":
				m.entityList.Select((m.entityList.Index() + 1) % len(m.entityList.Items()))
				return m, nil
			case "enter":
				row, ok := m.entityList.SelectedItem().(entityRow)
				if !ok {
					return m, nil
				}
				m.events.Close()
				if row.kind == entitySubjects {
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
	selected := m.entityList.Index()
	parts := []string{}
	for _, index := range []int{1, 0} {
		label := m.entityList.Items()[index].(entityRow).title
		if selected == index && m.entityFocused {
			label = m.subjects.list.Styles.Title.Render(label)
		}
		parts = append(parts, label)
	}
	line := strings.Join(parts, "   ")
	if ansi.StringWidth(line) > m.width {
		// Keep the selected entry visible instead of clipping it off-screen.
		line = m.entityList.Items()[selected].(entityRow).title
		if m.entityFocused {
			line = m.subjects.list.Styles.Title.Render(line)
		}
	}
	return ansi.Truncate(line, max(1, m.width), "…") + "\n" + ansi.Truncate("Tab: panel • ←/→: entity • Enter: open", max(1, m.width), "…")
}

func (m Model) View() tea.View {
	switch m.screen {
	case screenSubjectDetail, screenMiscThoughts, screenBrowseThoughts:
		if m.thoughts.ShowingDetail() {
			name := m.thoughts.SelectedSubjectName()
			if m.screen == screenSubjectDetail && m.subjects.selected != nil {
				name = m.subjects.selected.SubjectName
			}
			heading := m.subjects.list.Styles.Title.Render(name)
			return tea.NewView(heading + "\n\n" + m.thoughts.View())
		}
	}
	var content string
	switch m.screen {
	case screenEvents:
		content = m.events.View()
		if m.events.CanLeave() {
			content = ansi.Truncate("Local user: "+localUserLabel(m.user), max(1, m.width), "…") + "\n" + content
			content += "\n" + m.entityStrip()
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
	case screenMiscThoughts:
		content = "Misc thoughts\n\n" + m.thoughts.View()
		if m.thoughts.Browsing() {
			content += "\nEsc: subjects • q: quit"
		}
	case screenBrowseThoughts:
		heading := m.subjects.list.Styles.TitleBar.Render(m.subjects.list.Styles.Title.Render("Thoughts"))
		content = heading + "\n" + m.thoughts.View()
		if m.thoughts.Browsing() {
			content += "\nEsc: events • q: quit"
		}
	}
	return tea.NewView(content)
}

func localUserLabel(user *data.User) string {
	if user.Handle != nil {
		return fmt.Sprintf("%s (%s)", *user.Handle, user.UserID)
	}
	return user.UserID
}
