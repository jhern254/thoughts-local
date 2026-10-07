package browserterm

import (
	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/tui"
)

// Options remain browser presentation; the TUI only requests opening the page.
type optionsBridgeModel struct {
	tea.Model
	writer *terminalWriter
}

func (m optionsBridgeModel) Init() tea.Cmd {
	return tea.Batch(m.Model.Init(), func() tea.Msg { return tui.BrowserOptionsEnabledMsg{} })
}
func (m optionsBridgeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tui.OpenBrowserOptionsMsg); ok {
		_ = m.writer.writeFrame([]byte{'o'})
		return m, nil
	}
	var cmd tea.Cmd
	m.Model, cmd = m.Model.Update(msg)
	return m, cmd
}
