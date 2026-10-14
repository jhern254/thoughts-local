package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/modelassets"
	"github.com/jhern254/go-thoughts/internal/speech"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
)

// ModelSetup is provided only to the browser root. The process owns the loaded
// engine; its session owns setup commands. No native handles cross this seam.
type ModelSetup interface {
	Description() modelassets.ModelDescription
	Directory() string
	LoadInstalledModel(context.Context) (ModelSetupStatus, error)
	InstallAndEnableModel(context.Context, func(modelassets.DownloadProgress)) error
}
type ModelSetupStatus struct {
	Ready       bool
	CanDownload bool
	Message     string
}

type modelSetupState struct {
	service     ModelSetup
	description modelassets.ModelDescription
	open        bool
	offered     bool
	focus       int
	checking    bool
	ready       bool
	canDownload bool
	loadFailed  bool
	status      string
	notice      string

	attempt         uint64
	busy            bool
	cancelling      bool
	cancel          context.CancelFunc
	progress        modelassets.DownloadProgress
	pendingProgress chan modelassets.DownloadProgress
}
type modelSetupChecked struct {
	status ModelSetupStatus
	err    error
}
type modelSetupProgress struct {
	attempt  uint64
	progress modelassets.DownloadProgress
}
type modelSetupFinished struct {
	attempt uint64
	err     error
}

// ConfigureModelSetup is called by browser composition before Init. Terminal
// models and transport tests retain their existing constructor and behavior.
func (m *Model) ConfigureModelSetup(service ModelSetup) {
	m.models = modelSetupState{
		service:     service,
		description: service.Description(),
		focus:       0,
		checking:    true,
		status:      "Checking local speech…",
	}
}
func (m Model) checkInstalledModel() tea.Cmd {
	return func() tea.Msg {
		status, err := m.models.service.LoadInstalledModel(m.ctx)
		return modelSetupChecked{status: status, err: err}
	}
}
func (m *Model) updateSpeechAvailability() {
	m.thoughts, _ = m.thoughts.Update(thoughts.SpeechAvailabilityMsg{Available: m.models.ready})
}
func (m *Model) offerModelSetup() {
	if m.ctx.Err() == nil && m.models.service != nil && !m.models.checking && !m.models.ready && !m.models.offered && m.screen == screenEvents && !m.options.open && !m.exitPromptOpen {
		m.models.open = true
		m.models.offered = true
	}
}
func (m Model) updateModelSetup(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case modelSetupChecked:
		m.models.checking = false
		m.models.ready = message.status.Ready
		m.models.canDownload = message.status.CanDownload
		m.models.loadFailed = message.err != nil
		m.models.status = message.status.Message
		if message.err != nil {
			m.models.status = modelSetupErrorMessage(message.err)
		}
		m.updateSpeechAvailability()
	case modelSetupProgress:
		if message.attempt != m.models.attempt || !m.models.busy || m.models.cancelling {
			return m, nil
		}
		m.models.progress = message.progress
		if message.progress.DownloadedBytes == message.progress.TotalBytes {
			m.models.status = "Finishing installation and loading speech…"
		}
		return m, m.waitForModelProgress(m.models.attempt, m.models.pendingProgress)
	case modelSetupFinished:
		if message.attempt != m.models.attempt || !m.models.busy {
			return m, nil
		}
		wasCancelled := m.models.cancelling || errors.Is(message.err, context.Canceled)
		m.models.busy = false
		m.models.cancelling = false
		m.models.cancel = nil
		m.models.pendingProgress = nil
		m.models.focus = 0
		if message.err == nil && !wasCancelled {
			m.models.ready = true
			m.models.canDownload = false
			m.models.status = "Speech is ready. Use F8 in a Thought to start and stop dictation."
			m.models.notice = "Speech is ready · F8 to dictate"
		} else if wasCancelled {
			m.models.status = "Download cancelled. You can retry when you're ready."
		} else {
			m.models.status = modelSetupErrorMessage(message.err)
			if errors.Is(message.err, modelassets.ErrIntegrity) || errors.Is(message.err, modelassets.ErrNotInstalled) {
				m.models.canDownload = false
				m.models.loadFailed = true
			}
		}
		m.updateSpeechAvailability()
	}
	return m, nil
}
func modelSetupErrorMessage(err error) string {
	switch {
	case errors.Is(err, modelassets.ErrBusy):
		return "Another Thoughts instance is setting up this model. Try again shortly."
	case errors.Is(err, modelassets.ErrIntegrity), errors.Is(err, modelassets.ErrNotInstalled):
		return "Saved model files need attention. Close Thoughts and remove this model's folder before retrying."
	case errors.Is(err, modelassets.ErrFilesystem):
		return "Could not access the models folder. Check its permissions and retry."
	case errors.Is(err, modelassets.ErrDownload):
		return "The download failed. Check your connection and retry."
	case errors.Is(err, speech.ErrRuntime):
		return "Local speech isn't available in this build. You can still type and save."
	default:
		return "Could not prepare local speech. Try again."
	}
}
func (m Model) waitForModelProgress(attempt uint64, pendingProgress <-chan modelassets.DownloadProgress) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
			return nil
		case progress, open := <-pendingProgress:
			if !open {
				return nil
			}
			return modelSetupProgress{attempt: attempt, progress: progress}
		}
	}
}
func (m *Model) startModelDownload() tea.Cmd {
	downloadCtx, cancel := context.WithCancel(m.ctx)
	m.models.attempt++
	attempt := m.models.attempt
	m.models.busy = true
	m.models.cancelling = false
	m.models.cancel = cancel
	m.models.focus = 0
	m.models.status = "Downloading speech model…"
	m.models.progress = modelassets.DownloadProgress{TotalBytes: m.models.description.ExpectedSizeBytes}
	pendingProgress := make(chan modelassets.DownloadProgress, 1)
	m.models.pendingProgress = pendingProgress
	service := m.models.service
	download := func() tea.Msg {
		defer cancel()
		defer close(pendingProgress)
		// Only the installer produces/closes this mailbox. Replacing a numeric
		// snapshot bounds UI buffering without dropping any downloaded file bytes.
		report := func(progress modelassets.DownloadProgress) {
			select {
			case pendingProgress <- progress:
				return
			default:
			}
			select {
			case <-pendingProgress:
			default:
			}
			select {
			case pendingProgress <- progress:
			default:
			}
		}
		err := service.InstallAndEnableModel(downloadCtx, report)
		if downloadCtx.Err() != nil {
			err = errors.Join(downloadCtx.Err(), err)
		}
		return modelSetupFinished{attempt: attempt, err: err}
	}
	return tea.Batch(download, m.waitForModelProgress(attempt, pendingProgress))
}

type modelsAction uint8

const (
	modelsClose modelsAction = iota
	modelsDownload
	modelsHide
	modelsCancel
	modelsCheck
)

type modelsButton struct {
	action modelsAction
	label  string
}

func (m Model) modelsButtons() []modelsButton {
	switch {
	case m.models.busy:
		buttons := []modelsButton{{modelsHide, "Keep writing"}}
		if !m.models.cancelling {
			buttons = append(buttons, modelsButton{modelsCancel, "Cancel download"})
		}
		return buttons
	case m.models.checking:
		return []modelsButton{{modelsClose, "Back"}}
	case m.models.ready:
		return []modelsButton{{modelsClose, "Done"}}
	case m.models.canDownload:
		label := "Download and enable"
		if m.models.status != "" {
			label = "Retry download"
		}
		return []modelsButton{{modelsDownload, label}, {modelsClose, "Not now (recommended to download)"}}
	case m.models.loadFailed:
		return []modelsButton{{modelsCheck, "Retry check"}, {modelsClose, "Not now"}}
	default:
		return []modelsButton{{modelsClose, "Not now"}}
	}
}
func (m Model) activateModelsButton(button modelsButton) (tea.Model, tea.Cmd) {
	switch button.action {
	case modelsClose:
		m.models.open = false
		m.models.offered = true
	case modelsHide:
		m.models.open = false
		m.options.open = false
		m.requestOptions("")
	case modelsDownload:
		return m, m.startModelDownload()
	case modelsCancel:
		m.models.cancelling = true
		m.models.status = "Cancelling download…"
		m.models.cancel()
		m.models.focus = 0
	case modelsCheck:
		m.models.checking = true
		m.models.status = "Checking local speech…"
		return m, m.checkInstalledModel()
	}
	return m, nil
}
func (m Model) updateModelsInput(message tea.Msg) (tea.Model, tea.Cmd) {
	buttons := m.modelsButtons()
	m.models.focus = min(m.models.focus, len(buttons)-1)
	switch message := message.(type) {
	case tea.KeyPressMsg:
		switch message.String() {
		case "esc", "q":
			return m.activateModelsButton(modelsButton{action: modelsClose})
		case "ctrl+c":
			m.exitPromptOpen = true
			m.exitYes = false
		case "up", "left", "shift+tab":
			m.models.focus = (m.models.focus + len(buttons) - 1) % len(buttons)
		case "down", "right", "tab":
			m.models.focus = (m.models.focus + 1) % len(buttons)
		case "enter", "space":
			return m.activateModelsButton(buttons[m.models.focus])
		}
	case tea.MouseClickMsg:
		if message.Button == tea.MouseLeft {
			_, left, top, buttonRows := m.modelsPopupLayout()
			for index, row := range buttonRows {
				if message.Y == top+row && message.X >= left+2 && message.X < left+m.modelsPopupWidth()-2 {
					m.models.focus = index
					return m.activateModelsButton(buttons[index])
				}
			}
		}
	}
	return m, nil
}
func (m Model) modelsPopupWidth() int { return max(20, min(58, m.width-2)) }
func (m Model) modelsPopupLayout() (string, int, int, []int) {
	width := m.modelsPopupWidth()
	innerWidth := width - 4
	titleStyle := m.subjects.list.Styles.Title
	heading := "Enable local speech-to-text"
	if m.models.ready {
		heading = "Local speech is enabled"
	}
	rows := []string{titleStyle.Render("Models"), heading, "", m.models.description.DisplayName, fmt.Sprintf("Model size: %.1f MB", float64(m.models.description.ExpectedSizeBytes)/1e6), "Audio stays local; never saved.", ""}
	status := m.models.status
	if m.models.ready && status == "" {
		status = "Speech is ready. F8 starts/stops dictation."
	}
	if status != "" {
		rows = append(rows, strings.Split(ansi.Wrap(status, innerWidth, ""), "\n")...)
	}
	if m.models.busy {
		fraction := float64(0)
		if m.models.progress.TotalBytes > 0 {
			fraction = float64(m.models.progress.DownloadedBytes) / float64(m.models.progress.TotalBytes)
		}
		bar := progress.New(progress.WithWidth(innerWidth))
		rows = append(rows, bar.ViewAs(fraction), fmt.Sprintf("%.1f / %.1f MB", float64(m.models.progress.DownloadedBytes)/1e6, float64(m.models.progress.TotalBytes)/1e6))
	}
	if m.height >= 24 {
		directory := strings.Map(func(character rune) rune {
			if unicode.IsControl(character) {
				return ' '
			}
			return character
		}, m.models.service.Directory())
		rows = append(rows, "", ansi.Truncate("Folder: "+directory, innerWidth, "…"))
	}
	rows = append(rows, "")
	buttons := m.modelsButtons()
	buttonRows := make([]int, len(buttons))
	for index, button := range buttons {
		buttonRows[index] = len(rows) + 1
		label := button.label
		if index == min(m.models.focus, len(buttons)-1) {
			label = titleStyle.Render(label)
		}
		rows = append(rows, label)
	}
	for index, row := range rows {
		rows[index] = ansi.Truncate(row, innerWidth, "…")
	}
	popup := lipgloss.NewStyle().Width(width).Padding(0, 1).Border(lipgloss.RoundedBorder()).Background(lipgloss.Color("#202020")).Render(strings.Join(rows, "\n"))
	return popup, max(0, (m.width-width)/2), max(0, (m.height-lipgloss.Height(popup))/2), buttonRows
}
func (m Model) overlayModelsPopup(view tea.View) tea.View {
	popup, left, top, _ := m.modelsPopupLayout()
	view.Content = lipgloss.NewCompositor(lipgloss.NewLayer(view.Content), lipgloss.NewLayer(popup).X(left).Y(top).Z(1)).Render()
	view.MouseMode = tea.MouseModeCellMotion
	return view
}
