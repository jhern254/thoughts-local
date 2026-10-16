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

// SpeechModelSetup is provided only to the browser root. The process owns the loaded
// engine; its session owns setup commands. No native handles cross this seam.
type SpeechModelSetup interface {
	Description() modelassets.ModelDescription
	Directory() string
	LoadInstalledSpeechModel(context.Context) (SpeechModelSetupStatus, error)
	InstallAndEnableSpeechModel(context.Context, func(modelassets.DownloadProgress)) error
}
type SpeechModelSetupStatus struct {
	Ready       bool
	CanDownload bool
	Message     string
}

type speechSetupState struct {
	service     SpeechModelSetup
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
type speechSetupChecked struct {
	status SpeechModelSetupStatus
	err    error
}
type speechSetupProgress struct {
	attempt  uint64
	progress modelassets.DownloadProgress
}
type speechSetupFinished struct {
	attempt uint64
	err     error
}

// ConfigureSpeechModelSetup is called by browser composition before Init. Terminal
// models and transport tests retain their existing constructor and behavior.
func (m *Model) ConfigureSpeechModelSetup(service SpeechModelSetup) {
	m.speechSetup = speechSetupState{
		service:     service,
		description: service.Description(),
		focus:       0,
		checking:    true,
		status:      "Checking local speech…",
	}
}
func (m Model) checkInstalledSpeechModel() tea.Cmd {
	return func() tea.Msg {
		status, err := m.speechSetup.service.LoadInstalledSpeechModel(m.ctx)
		return speechSetupChecked{status: status, err: err}
	}
}
func (m *Model) updateSpeechAvailability() {
	m.thoughts, _ = m.thoughts.Update(thoughts.SpeechAvailabilityMsg{Available: m.speechSetup.ready})
}
func (m *Model) offerSpeechSetup() {
	if m.ctx.Err() == nil && m.speechSetup.service != nil && !m.speechSetup.checking && !m.speechSetup.ready && !m.speechSetup.offered && m.screen == screenEvents && !m.options.open && !m.exitPromptOpen {
		m.speechSetup.open = true
		m.speechSetup.offered = true
	}
}
func (m Model) updateSpeechSetup(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case speechSetupChecked:
		m.speechSetup.checking = false
		m.speechSetup.ready = message.status.Ready
		m.speechSetup.canDownload = message.status.CanDownload
		m.speechSetup.loadFailed = message.err != nil
		m.speechSetup.status = message.status.Message
		if message.err != nil {
			m.speechSetup.status = speechSetupErrorMessage(message.err)
		}
		m.updateSpeechAvailability()
	case speechSetupProgress:
		if message.attempt != m.speechSetup.attempt || !m.speechSetup.busy || m.speechSetup.cancelling {
			return m, nil
		}
		m.speechSetup.progress = message.progress
		if message.progress.DownloadedBytes == message.progress.TotalBytes {
			m.speechSetup.status = "Finishing installation and loading speech…"
		}
		return m, m.waitForSpeechModelProgress(m.speechSetup.attempt, m.speechSetup.pendingProgress)
	case speechSetupFinished:
		if message.attempt != m.speechSetup.attempt || !m.speechSetup.busy {
			return m, nil
		}
		wasCancelled := m.speechSetup.cancelling || errors.Is(message.err, context.Canceled)
		m.speechSetup.busy = false
		m.speechSetup.cancelling = false
		m.speechSetup.cancel = nil
		m.speechSetup.pendingProgress = nil
		m.speechSetup.focus = 0
		if message.err == nil && !wasCancelled {
			m.speechSetup.ready = true
			m.speechSetup.canDownload = false
			m.speechSetup.status = "Speech is ready. Use F8 in a Thought to start and stop dictation."
			m.speechSetup.notice = "Speech is ready · F8 to dictate"
		} else if wasCancelled {
			m.speechSetup.status = "Download cancelled. You can retry when you're ready."
		} else {
			m.speechSetup.status = speechSetupErrorMessage(message.err)
			if errors.Is(message.err, modelassets.ErrIntegrity) || errors.Is(message.err, modelassets.ErrNotInstalled) {
				m.speechSetup.canDownload = false
				m.speechSetup.loadFailed = true
			}
		}
		m.updateSpeechAvailability()
	}
	return m, nil
}
func speechSetupErrorMessage(err error) string {
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
func (m Model) waitForSpeechModelProgress(attempt uint64, pendingProgress <-chan modelassets.DownloadProgress) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
			return nil
		case progress, open := <-pendingProgress:
			if !open {
				return nil
			}
			return speechSetupProgress{attempt: attempt, progress: progress}
		}
	}
}
func (m *Model) startSpeechModelDownload() tea.Cmd {
	downloadCtx, cancel := context.WithCancel(m.ctx)
	m.speechSetup.attempt++
	attempt := m.speechSetup.attempt
	m.speechSetup.busy = true
	m.speechSetup.cancelling = false
	m.speechSetup.cancel = cancel
	m.speechSetup.focus = 0
	m.speechSetup.status = "Downloading speech model…"
	m.speechSetup.progress = modelassets.DownloadProgress{TotalBytes: m.speechSetup.description.ExpectedSizeBytes}
	pendingProgress := make(chan modelassets.DownloadProgress, 1)
	m.speechSetup.pendingProgress = pendingProgress
	service := m.speechSetup.service
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
		err := service.InstallAndEnableSpeechModel(downloadCtx, report)
		if downloadCtx.Err() != nil {
			err = errors.Join(downloadCtx.Err(), err)
		}
		return speechSetupFinished{attempt: attempt, err: err}
	}
	return tea.Batch(download, m.waitForSpeechModelProgress(attempt, pendingProgress))
}

type speechSetupAction uint8

const (
	speechSetupClose speechSetupAction = iota
	speechSetupDownload
	speechSetupHide
	speechSetupCancel
	speechSetupCheck
)

type speechSetupButton struct {
	action speechSetupAction
	label  string
}

func (m Model) speechSetupButtons() []speechSetupButton {
	switch {
	case m.speechSetup.busy:
		buttons := []speechSetupButton{{speechSetupHide, "Keep writing"}}
		if !m.speechSetup.cancelling {
			buttons = append(buttons, speechSetupButton{speechSetupCancel, "Cancel download"})
		}
		return buttons
	case m.speechSetup.checking:
		return []speechSetupButton{{speechSetupClose, "Back"}}
	case m.speechSetup.ready:
		return []speechSetupButton{{speechSetupClose, "Done"}}
	case m.speechSetup.canDownload:
		label := "Download and enable"
		if m.speechSetup.status != "" {
			label = "Retry download"
		}
		return []speechSetupButton{{speechSetupDownload, label}, {speechSetupClose, "Not now (recommended to download)"}}
	case m.speechSetup.loadFailed:
		return []speechSetupButton{{speechSetupCheck, "Retry check"}, {speechSetupClose, "Not now"}}
	default:
		return []speechSetupButton{{speechSetupClose, "Not now"}}
	}
}
func (m Model) activateSpeechSetupButton(button speechSetupButton) (tea.Model, tea.Cmd) {
	switch button.action {
	case speechSetupClose:
		m.speechSetup.open = false
		m.speechSetup.offered = true
	case speechSetupHide:
		m.speechSetup.open = false
		m.options.open = false
		m.requestOptions("")
	case speechSetupDownload:
		return m, m.startSpeechModelDownload()
	case speechSetupCancel:
		m.speechSetup.cancelling = true
		m.speechSetup.status = "Cancelling download…"
		m.speechSetup.cancel()
		m.speechSetup.focus = 0
	case speechSetupCheck:
		m.speechSetup.checking = true
		m.speechSetup.status = "Checking local speech…"
		return m, m.checkInstalledSpeechModel()
	}
	return m, nil
}
func (m Model) updateSpeechSetupInput(message tea.Msg) (tea.Model, tea.Cmd) {
	buttons := m.speechSetupButtons()
	m.speechSetup.focus = min(m.speechSetup.focus, len(buttons)-1)
	switch message := message.(type) {
	case tea.KeyPressMsg:
		switch message.String() {
		case "esc", "q":
			return m.activateSpeechSetupButton(speechSetupButton{action: speechSetupClose})
		case "ctrl+c":
			m.exitPromptOpen = true
			m.exitYes = false
		case "up", "left", "shift+tab":
			m.speechSetup.focus = (m.speechSetup.focus + len(buttons) - 1) % len(buttons)
		case "down", "right", "tab":
			m.speechSetup.focus = (m.speechSetup.focus + 1) % len(buttons)
		case "enter", "space":
			return m.activateSpeechSetupButton(buttons[m.speechSetup.focus])
		}
	case tea.MouseClickMsg:
		if message.Button == tea.MouseLeft {
			_, left, top, buttonRows := m.speechSetupPopupLayout()
			for index, row := range buttonRows {
				if message.Y == top+row && message.X >= left+2 && message.X < left+m.speechSetupPopupWidth()-2 {
					m.speechSetup.focus = index
					return m.activateSpeechSetupButton(buttons[index])
				}
			}
		}
	}
	return m, nil
}
func (m Model) speechSetupPopupWidth() int { return max(20, min(58, m.width-2)) }
func (m Model) speechSetupPopupLayout() (string, int, int, []int) {
	width := m.speechSetupPopupWidth()
	innerWidth := width - 4
	titleStyle := m.subjects.list.Styles.Title
	heading := "Enable local speech-to-text"
	if m.speechSetup.ready {
		heading = "Local speech is enabled"
	}
	rows := []string{titleStyle.Render("Models"), heading, "", m.speechSetup.description.DisplayName, fmt.Sprintf("Model size: %.1f MB", float64(m.speechSetup.description.ExpectedSizeBytes)/1e6), "Audio stays local; never saved.", ""}
	status := m.speechSetup.status
	if m.speechSetup.ready && status == "" {
		status = "Speech is ready. F8 starts/stops dictation."
	}
	if status != "" {
		rows = append(rows, strings.Split(ansi.Wrap(status, innerWidth, ""), "\n")...)
	}
	if m.speechSetup.busy {
		fraction := float64(0)
		if m.speechSetup.progress.TotalBytes > 0 {
			fraction = float64(m.speechSetup.progress.DownloadedBytes) / float64(m.speechSetup.progress.TotalBytes)
		}
		bar := progress.New(progress.WithWidth(innerWidth))
		rows = append(rows, bar.ViewAs(fraction), fmt.Sprintf("%.1f / %.1f MB", float64(m.speechSetup.progress.DownloadedBytes)/1e6, float64(m.speechSetup.progress.TotalBytes)/1e6))
	}
	if m.height >= 24 {
		directory := strings.Map(func(character rune) rune {
			if unicode.IsControl(character) {
				return ' '
			}
			return character
		}, m.speechSetup.service.Directory())
		rows = append(rows, "", ansi.Truncate("Folder: "+directory, innerWidth, "…"))
	}
	rows = append(rows, "")
	buttons := m.speechSetupButtons()
	buttonRows := make([]int, len(buttons))
	for index, button := range buttons {
		buttonRows[index] = len(rows) + 1
		label := button.label
		if index == min(m.speechSetup.focus, len(buttons)-1) {
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
func (m Model) overlaySpeechSetupPopup(view tea.View) tea.View {
	popup, left, top, _ := m.speechSetupPopupLayout()
	view.Content = lipgloss.NewCompositor(lipgloss.NewLayer(view.Content), lipgloss.NewLayer(popup).X(left).Y(top).Z(1)).Render()
	view.MouseMode = tea.MouseModeCellMotion
	return view
}
