package browserterm

import (
	"encoding/json"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/tui"
)

type optionsBridgeModel struct {
	tea.Model
	writer                *terminalWriter
	lastOptionsState      tui.BrowserOptionsState
	videoEditor           *videoEditor
	browserOptionsEnabled bool
}

func (m optionsBridgeModel) Init() tea.Cmd {
	return m.Model.Init()
}
func (m optionsBridgeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var enableBrowserOptionsCmd tea.Cmd
	// Startup settings can arrive before commands returned by Init complete.
	if !m.browserOptionsEnabled {
		m.browserOptionsEnabled = true
		m.Model, enableBrowserOptionsCmd = m.Model.Update(tui.BrowserOptionsEnabledMsg{})
	}
	var cmd tea.Cmd
	m.Model, cmd = m.Model.Update(msg)
	cmd = tea.Batch(enableBrowserOptionsCmd, cmd)
	if optionsModel, ok := m.Model.(interface {
		OptionsState() tui.BrowserOptionsState
	}); ok {
		optionsState := optionsModel.OptionsState()
		if optionsState != m.lastOptionsState {
			m.lastOptionsState = optionsState
			editorID := ""
			if m.videoEditor != nil {
				var cleanupRetiredVideoEditor func() error
				editorID, cleanupRetiredVideoEditor = m.videoEditor.setOpen(optionsState.Open && !optionsState.Accessibility)
				m.videoEditor.setPreviewOperation(optionsState.ID, optionsState.Request == "prepareLoop")
				if cleanupRetiredVideoEditor != nil {
					cmd = tea.Batch(cmd, func() tea.Msg { _ = cleanupRetiredVideoEditor(); return nil })
				}
			}
			optionsJSON, _ := json.Marshal(struct {
				tui.BrowserOptionsState
				EditorID string `json:"editorID"`
			}{optionsState, editorID})
			_ = m.writer.writeFrame(append([]byte{'o'}, optionsJSON...))
		}
	}
	return m, cmd
}
