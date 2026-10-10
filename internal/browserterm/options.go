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
			data, _ := json.Marshal(state)
			_ = m.writer.writeFrame(append([]byte{'o'}, data...))
		}
	}
	return m, cmd
}
