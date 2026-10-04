package browserterm

import (
	"encoding/json"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
)

// The adapter observes authoritative state after Update, so queued commands
// cannot announce an obsolete draft. It carries no audio or authored text.
type voiceBridgeModel struct {
	tea.Model
	writer *terminalWriter
	state  thoughts.VoiceState
}

func (m voiceBridgeModel) Init() tea.Cmd {
	return tea.Batch(m.Model.Init(), func() tea.Msg { return thoughts.VoiceAvailable{} })
}
func (m voiceBridgeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.Model, cmd = m.Model.Update(msg)
	if model, ok := m.Model.(interface{ VoiceState() thoughts.VoiceState }); ok {
		state := model.VoiceState()
		if state != m.state {
			m.state = state
			data, _ := json.Marshal(state)
			_ = m.writer.writeFrame(append([]byte{'v'}, data...))
		}
	}
	return m, cmd
}
