package browserterm

import (
	"encoding/json"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/tui"
)

type optionsBridgeModel struct {
	tea.Model
	writer      *terminalWriter
	lastOptions tui.BrowserOptionsState
	videoEditor *videoEditor
	enabled     bool
}

func (m optionsBridgeModel) Init() tea.Cmd {
	return m.Model.Init()
}
func (m optionsBridgeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var enable tea.Cmd
	// Startup settings can arrive before commands returned by Init complete.
	if !m.enabled {
		m.enabled = true
		m.Model, enable = m.Model.Update(tui.BrowserOptionsEnabledMsg{})
	}
	var cmd tea.Cmd
	m.Model, cmd = m.Model.Update(msg)
	cmd = tea.Batch(enable, cmd)
	if model, ok := m.Model.(interface {
		OptionsState() tui.BrowserOptionsState
	}); ok {
		state := model.OptionsState()
		if state != m.lastOptions {
			m.lastOptions = state
			editorID := ""
			if m.videoEditor != nil {
				var cleanup func() error
				editorID, cleanup = m.videoEditor.setOpen(state.Open && !state.Accessibility)
				if cleanup != nil {
					cmd = tea.Batch(cmd, func() tea.Msg { _ = cleanup(); return nil })
				}
			}
			data, _ := json.Marshal(struct {
				tui.BrowserOptionsState
				EditorID string `json:"editorID"`
			}{state, editorID})
			_ = m.writer.writeFrame(append([]byte{'o'}, data...))
		}
	}
	return m, cmd
}
