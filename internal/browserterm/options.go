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
}

func (m optionsBridgeModel) Init() tea.Cmd {
	return tea.Batch(m.Model.Init(), func() tea.Msg { return tui.BrowserOptionsEnabledMsg{} })
}
func (m optionsBridgeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.Model, cmd = m.Model.Update(msg)
	if model, ok := m.Model.(interface {
		OptionsState() tui.BrowserOptionsState
	}); ok {
		state := model.OptionsState()
		if state != m.lastOptions {
			m.lastOptions = state
			editorID := ""
			if m.videoEditor != nil {
				var cleanup func() error
				editorID, cleanup = m.videoEditor.setOpen(state.Open)
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
