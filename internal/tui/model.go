package tui

import (
	"context"
	"fmt"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
)

const (
	defaultWidth  = 80
	defaultHeight = 24
)

type screen uint8

const (
	screenEntities screen = iota
	screenSubjectList
	screenSubjectCreate
	screenSubjectDetail
)

type entityKind uint8

const entitySubjects entityKind = iota

type entityRow struct {
	kind        entityKind
	title       string
	description string
}

func (row entityRow) Title() string       { return row.title }
func (row entityRow) Description() string { return row.description }
func (row entityRow) FilterValue() string { return row.title }

type Model struct {
	ctx      context.Context
	user     *data.User
	subjects SubjectService
	screen   screen

	entityList       list.Model
	subjectList      list.Model
	subjectInput     textinput.Model
	selectedSubject  *data.Subject
	subjectError     error
	subjectLoading   bool
	subjectListStale bool
}

func NewModel(ctx context.Context, user *data.User, subjects SubjectService) Model {
	entities := list.New([]list.Item{
		entityRow{
			kind:        entitySubjects,
			title:       "Subjects",
			description: "Browse and organize subjects",
		},
	}, list.NewDefaultDelegate(), defaultWidth, defaultHeight)
	entities.Title = "Entities"
	entities.SetFilteringEnabled(false)
	entities.SetShowStatusBar(false)

	subjectList := list.New(subjectRows(nil), list.NewDefaultDelegate(), defaultWidth, defaultHeight)
	subjectList.Title = "Subjects"
	subjectList.SetStatusBarItemName("subject", "subjects")

	input := textinput.New()
	input.Prompt = "Subject name: "
	input.Placeholder = "What is this about?"

	return Model{
		ctx:          ctx,
		user:         user,
		subjects:     subjects,
		screen:       screenEntities,
		entityList:   entities,
		subjectList:  subjectList,
		subjectInput: input,
	}
}

func (Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.entityList.SetSize(message.Width, max(0, message.Height-3))
		m.subjectList.SetSize(message.Width, max(0, message.Height))
		m.subjectInput.SetWidth(max(0, message.Width-2))
		return m, nil
	case subjectsListedMsg:
		return m.handleSubjectsListed(message)
	case subjectCreatedMsg:
		return m.handleSubjectCreated(message)
	case subjectFoundMsg:
		return m.handleSubjectFound(message)
	case tea.KeyPressMsg:
		if message.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}

	switch m.screen {
	case screenEntities:
		return m.updateEntities(message)
	case screenSubjectList:
		return m.updateSubjectList(message)
	case screenSubjectCreate:
		return m.updateSubjectCreate(message)
	case screenSubjectDetail:
		return m.updateSubjectDetail(message)
	default:
		return m, nil
	}
}

func (m Model) updateEntities(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "q":
			return m, tea.Quit
		case "enter":
			row, ok := m.entityList.SelectedItem().(entityRow)
			if ok && row.kind == entitySubjects {
				m.screen = screenSubjectList
				m.subjectError = nil
				m.subjectLoading = true
				return m, m.listSubjects()
			}
		}
	}

	var command tea.Cmd
	m.entityList, command = m.entityList.Update(message)
	return m, command
}

func (m Model) View() tea.View {
	var content string
	switch m.screen {
	case screenEntities:
		content = fmt.Sprintf("Thoughts\n\nLocal user: %s\n\n%s", localUserLabel(m.user), m.entityList.View())
	case screenSubjectList:
		content = m.viewSubjectList()
	case screenSubjectCreate:
		content = m.viewSubjectCreate()
	case screenSubjectDetail:
		content = m.viewSubjectDetail()
	}
	return tea.NewView(content)
}

func localUserLabel(user *data.User) string {
	if user.Handle != nil {
		return fmt.Sprintf("%s (%s)", *user.Handle, user.UserID)
	}
	return user.UserID
}
